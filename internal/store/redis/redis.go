package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/knadh/otpgateway/v3/internal/store"
	"github.com/knadh/otpgateway/v3/pkg/models"
	"github.com/redis/go-redis/v9"
)

// Redis implements a Redis Store.
type Redis struct {
	client *redis.Client
	conf   Conf
}

var (
	ctx = context.Background()
)

// Conf contains Redis configuration fields.
type Conf struct {
	Host      string        `json:"host"`
	Port      int           `json:"port"`
	Username  string        `json:"username"`
	Password  string        `json:"password"`
	DB        int           `json:"db"`
	MaxActive int           `json:"max_active"`
	MaxIdle   int           `json:"max_idle"`
	Timeout   time.Duration `json:"timeout"`
	KeyPrefix string        `json:"key_prefix"`
	// If this is set, 'check' and 'close' events will be PUBLISHed to
	// to this Redis key (Redis PubSub).
	PublishKey string `json:"publish_key"`
}

type event struct {
	Type      string          `json:"type"`
	Namespace string          `json:"namespace"`
	ID        string          `json:"id"`
	Data      json.RawMessage `json:"data"`
}

// New returns a Redis implementation of store.
func New(c Conf) *Redis {
	if c.KeyPrefix == "" {
		c.KeyPrefix = "OTP"
	}

	client := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%d", c.Host, c.Port),
		Username:     c.Username,
		Password:     c.Password,
		DB:           c.DB,
		DialTimeout:  c.Timeout,
		WriteTimeout: c.Timeout,
		ReadTimeout:  c.Timeout,
	})

	return &Redis{
		conf:   c,
		client: client,
	}
}

// Ping checks if Redis server is reachable
func (r *Redis) Ping() error {
	return r.client.Ping(ctx).Err()
}

// Check checks the attempt count and TTL duration against an ID.
// Passing count=true increments the attempt counter.
func (r *Redis) Check(namespace, id string, counter bool) (models.OTP, error) {
	// Retrieve the OTP information.
	out, err := r.get(namespace, id)
	if err != nil {
		return out, err
	}
	if !counter {
		return out, err
	}

	// Define the key....
	key := r.makeKey(namespace, id)

	// Increment attempts and get TTL.
	pipe := r.client.TxPipeline()
	attempts := pipe.HIncrBy(ctx, key, "attempts", 1)
	ttl := pipe.TTL(ctx, key)
	_, err = pipe.Exec(ctx)
	if err != nil {
		return out, err
	}

	out.Attempts = int(attempts.Val())
	out.TTL = ttl.Val()

	// If there's a configured PublishKey, publish the event.
	if r.conf.PublishKey != "" {
		b, _ := json.Marshal(out)
		e, _ := json.Marshal(event{
			Type:      "check",
			Namespace: namespace,
			ID:        id,
			Data:      json.RawMessage(b),
		})
		err := r.client.Publish(ctx, r.conf.PublishKey, e).Err()
		if err != nil {
			return out, err
		}
	}

	return out, nil
}

func (r *Redis) Set(namespace, id string, otp models.OTP) (models.OTP, error) {
	// Set the OTP value.
	key := r.makeKey(namespace, id)
	exp := otp.TTL.Milliseconds()

	// Create a transaction to execute commands atomically.
	txf := func(tx *redis.Tx) error {
		_, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.HMSet(ctx, key,
				"otp", otp.OTP,
				"to", otp.To,
				"channel_description", otp.ChannelDesc,
				"address_description", otp.AddressDesc,
				"extra", string(otp.Extra),
				"provider", otp.Provider,
				"closed", false,
				"max_attempts", otp.MaxAttempts)

			pipe.HIncrBy(ctx, key, "attempts", 1)
			pipe.PExpire(ctx, key, time.Duration(exp)*time.Millisecond)
			return nil
		})
		return err
	}

	// Watch the key for changes. If the key is modified externally between
	// the time of watch and the transaction execution, the transaction will be aborted.
	err := r.client.Watch(ctx, txf, key)
	if err != nil {
		return otp, err
	}

	// Retrieve the updated attempts count to update the OTP struct.
	attempts, err := r.client.HGet(ctx, key, "attempts").Int()
	if err != nil {
		return otp, err
	}

	otp.Attempts = attempts
	otp.TTLSeconds = otp.TTL.Seconds()
	otp.Namespace = namespace
	otp.ID = id

	return otp, nil
}

// SetAddress sets (updates) the address on an existing OTP.
func (r *Redis) SetAddress(namespace, id, address string) error {
	// Set the OTP value.
	key := r.makeKey(namespace, id)

	if err := r.client.HSet(ctx, key, "to", address).Err(); err != nil {
		return err
	}

	return nil
}

// Close closes an OTP and marks it as done (verified).
// After this, the OTP has to expire after a TTL or be deleted.
func (r *Redis) Close(namespace, id string) error {
	// Set the OTP as closed.
	if err := r.client.HSet(ctx, r.makeKey(namespace, id), "closed", true).Err(); err != nil {
		return err
	}

	// Publish?
	if r.conf.PublishKey != "" {
		e, _ := json.Marshal(event{
			Type:      "close",
			Namespace: namespace,
			ID:        id,
			Data:      json.RawMessage([]byte(`null`)),
		})
		if err := r.client.Publish(ctx, r.conf.PublishKey, e).Err(); err != nil {
			return err
		}
	}

	return nil
}

// Delete deletes the OTP saved against a given ID.
func (r *Redis) Delete(namespace, id string) error {
	if err := r.client.Del(ctx, r.makeKey(namespace, id)).Err(); err != nil {
		return err
	}
	return nil
}

// makeKey makes the Redis key for the OTP.
func (r *Redis) makeKey(namespace, id string) string {
	return fmt.Sprintf("%s:%s:%s", r.conf.KeyPrefix, namespace, id)
}

// get retrieves the OTP information from Redis based on the namespace and ID.
func (r *Redis) get(namespace, id string) (models.OTP, error) {
	key := r.makeKey(namespace, id)
	out := models.OTP{
		Namespace: namespace,
		ID:        id,
	}

	// Retrieve all fields of the hash.
	if err := r.client.HGetAll(ctx, key).Scan(&out); err != nil {
		return out, err
	}

	// Doesn't exist?
	if out.OTP == "" {
		return out, store.ErrNotExist
	}

	// Retrieve TTL.
	ttl, err := r.client.TTL(ctx, key).Result()
	if err != nil {
		return out, err
	}

	out.TTL = ttl
	out.TTLSeconds = ttl.Seconds()
	return out, nil
}
