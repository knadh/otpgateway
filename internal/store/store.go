package store

import (
	"errors"

	"github.com/knadh/otpgateway/v3/pkg/models"
)

// ErrNotExist is thrown when an OTP (requested by namespace / ID)
// does not exist.
var ErrNotExist = errors.New("the OTP does not exist")

// Store represents a storage backend where OTP data is stored.
type Store interface {
	// Set sets an OTP against an ID. Every Set() increments the attempts
	// count against the ID that was initially set.
	Set(namespace, id string, otp models.OTP) (models.OTP, error)

	// SetAddress sets (updates) the address on an existing OTP.
	SetAddress(namespace, id, address string) error

	// Check checks the attempt count and TTL duration against an ID.
	// Passing counter=true increments the attempt counter.
	Check(namespace, id string, counter bool) (models.OTP, error)

	// Close closes an OTP and marks it as done (verified).
	// After this, the OTP has to expire after a TTL or be deleted.
	Close(namespace, id string) error

	// Delete deletes the OTP saved against a given ID.
	Delete(namespace, id string) error

	// Ping checks if store is reachable
	Ping() error
}
