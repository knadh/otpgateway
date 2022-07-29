package redis

import (
	"encoding/json"
	"fmt"
	"time"

	redigo "github.com/gomodule/redigo/redis"
	"github.com/knadh/otpgateway/v3/internal/store"
	"github.com/knadh/otpgateway/v3/pkg/models"
)

// Redis implements a  Redis Store.
type Redis struct {
	pool *redigo.Pool
	conf Conf
}

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
	pool := &redigo.Pool{
		Wait:      true,
		MaxActive: c.MaxActive,
		MaxIdle:   c.MaxIdle,
		Dial: func() (redigo.Conn, error) {
			c, err := redigo.Dial(
				"tcp",
				fmt.Sprintf("%s:%d", c.Host, c.Port),
				redigo.DialPassword(c.Password),
				redigo.DialConnectTimeout(c.Timeout),
				redigo.DialReadTimeout(c.Timeout),
				redigo.DialWriteTimeout(c.Timeout),
				redigo.DialDatabase(c.DB),
			)

			return c, err
		},
	}
	return &Redis{
		conf: c,
		pool: pool,
	}
}

// Ping checks if Redis server is reachable
func (r *Redis) Ping() error {
	c := r.pool.Get()
	defer c.Close()
	_, err := c.Do("PING") // Test redis connection
	return err
}

// Check checks the attempt count and TTL duration against an ID.
// Passing count=true increments the attempt counter.
func (r *Redis) Check(namespace, id string, counter bool) (models.OTP, error) {
	c := r.pool.Get()
	defer c.Close()

	out, err := r.get(namespace, id, c)
	if err != nil {
		return out, err
	}
	if !counter {
		return out, err
	}

	// Increment attempts.
	key := r.makeKey(namespace, id)
	r.begin(c)
	c.Send("HINCRBY", key, "attempts", 1)
	c.Send("TTL", key)
	resp, err := r.end(c)
	if err != nil {
		return out, err
	}

	attempts, _ := redigo.Int(resp[0], nil)
	out.Attempts = attempts

	ttl, _ := redigo.Int64(resp[1], nil)
	out.TTL = time.Duration(ttl) * time.Second

	// Publish?
	if r.conf.PublishKey != "" {
		b, _ := json.Marshal(out)
		e, _ := json.Marshal(event{
			Type:      "check",
			Namespace: namespace,
			ID:        id,
			Data:      json.RawMessage(b),
		})
		_, _ = c.Do("PUBLISH", r.conf.PublishKey, e)
	}

	return out, err
}

// Set sets an OTP in the store.
func (r *Redis) Set(namespace, id string, otp models.OTP) (models.OTP, error) {
	c := r.pool.Get()
	defer c.Close()

	// Set the OTP value.
	var (
		key = r.makeKey(namespace, id)
		exp = otp.TTL.Nanoseconds() / int64(time.Millisecond)
	)

	r.begin(c)
	c.Send("HMSET", key,
		"otp", otp.OTP,
		"to", otp.To,
		"channel_description", otp.ChannelDesc,
		"address_description", otp.AddressDesc,
		"extra", string(otp.Extra),
		"provider", otp.Provider,
		"closed", false,
		"max_attempts", otp.MaxAttempts)
	c.Send("HINCRBY", key, "attempts", 1)
	c.Send("PEXPIRE", key, exp)

	// Flush the commands and get their responses.
	// [1] is the number of attempts.
	// [3] is the TTL.
	resp, err := r.end(c)
	if err != nil {
		return otp, err
	}
	attempts, err := redigo.Int(resp[1], nil)
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
	c := r.pool.Get()
	defer c.Close()

	// Set the OTP value.
	var key = r.makeKey(namespace, id)

	if _, err := c.Do("HSET", key, "to", address); err != nil {
		return err
	}

	return nil
}

// Close closes an OTP and marks it as done (verified).
// After this, the OTP has to expire after a TTL or be deleted.
func (r *Redis) Close(namespace, id string) error {
	c := r.pool.Get()
	defer c.Close()

	_, err := c.Do("HSET", r.makeKey(namespace, id), "closed", true)

	// Publish?
	if r.conf.PublishKey != "" {
		e, _ := json.Marshal(event{
			Type:      "close",
			Namespace: namespace,
			ID:        id,
			Data:      json.RawMessage([]byte(`null`)),
		})
		_, _ = c.Do("PUBLISH", r.conf.PublishKey, e)
	}

	return err
}

// Delete deletes the OTP saved against a given ID.
func (r *Redis) Delete(namespace, id string) error {
	c := r.pool.Get()
	defer c.Close()

	_, err := c.Do("DEL", r.makeKey(namespace, id))
	return err
}

// get begins a transaction.
func (r *Redis) get(namespace, id string, c redigo.Conn) (models.OTP, error) {
	var (
		key = r.makeKey(namespace, id)
		out = models.OTP{
			Namespace: namespace,
			ID:        id,
		}
	)

	resp, err := redigo.Values(c.Do("HGETALL", key))
	if err != nil {
		return out, err
	}
	if err := redigo.ScanStruct(resp, &out); err != nil {
		return out, err
	}

	// Doesn't exist?
	if out.OTP == "" {
		return out, store.ErrNotExist
	}

	ttl, err := redigo.Int64(c.Do("TTL", key))
	if err != nil {
		return out, err
	}

	out.TTL = time.Duration(ttl) * time.Second
	out.TTLSeconds = out.TTL.Seconds()
	return out, nil
}

// begin begins a transaction.
func (r *Redis) begin(c redigo.Conn) error {
	return c.Send("MULTI")
}

// end begins a transaction.
func (r *Redis) end(c redigo.Conn) ([]interface{}, error) {
	rep, err := redigo.Values(c.Do("EXEC"))

	// Check if there are any errors.
	for _, r := range rep {
		if v, ok := r.(redigo.Error); ok {
			return rep, v
		}
	}
	return rep, err
}

// makeKey makes the Redis key for the OTP.
func (r *Redis) makeKey(namespace, id string) string {
	return fmt.Sprintf("%s:%s:%s", r.conf.KeyPrefix, namespace, id)
}
