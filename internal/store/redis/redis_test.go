package redis

import (
	"log"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis"
	"github.com/knadh/otpgateway/v3/internal/store"
	"github.com/knadh/otpgateway/v3/pkg/models"
	"github.com/stretchr/testify/assert"
)

var (
	rStore  *Redis
	rdis    *miniredis.Miniredis
	mockOTP = models.OTP{
		Namespace:   "mynamespace",
		ID:          "myotpid",
		OTP:         "myotp",
		MaxAttempts: 3,
		ChannelDesc: "channeldesc",
		AddressDesc: "addressdesc",
		Provider:    "smtp",
		Extra:       []byte(`{"some": "json", "extra": true}`),
		TTL:         2 * time.Second,
		TTLSeconds:  2,
	}
)

func init() {
	rd, err := miniredis.Run()
	if err != nil {
		log.Println(err)
	}
	rdis = rd

	port, _ := strconv.Atoi(rd.Port())
	rStore = New(Conf{
		Host: rd.Host(),
		Port: port,
	})
}

func reset(t *testing.T) {
	rdis.FlushDB()
	_, err := rStore.Set(mockOTP.Namespace, mockOTP.ID, mockOTP)
	assert.Equal(t, nil, err, "error setting OTP")
}

func TestStoreSet(t *testing.T) {
	rdis.FlushDB()
	resp, err := rStore.Set(mockOTP.Namespace, mockOTP.ID, mockOTP)
	assert.Equal(t, nil, err, "error setting OTP")

	cmp := mockOTP
	// Override dynamic values.
	cmp.Attempts = resp.Attempts
	cmp.TTL = resp.TTL
	cmp.TTLSeconds = resp.TTLSeconds
	assert.Equal(t, cmp, resp, "OTP doesn't match")
}

func TestStoreCheck(t *testing.T) {
	reset(t)

	// Don't increment.
	o, _ := rStore.Check(mockOTP.Namespace, mockOTP.ID, false)
	assert.Equal(t, 1, o.Attempts, "attempts incorrectly incremented")

	// Increment.
	o, _ = rStore.Check(mockOTP.Namespace, mockOTP.ID, true)
	assert.Equal(t, 2, o.Attempts, "attempts didn't increment")

	o, _ = rStore.Check(mockOTP.Namespace, mockOTP.ID, true)
	assert.Equal(t, 3, o.Attempts, "attempts didn't increment")
}

func TestStoreTTL(t *testing.T) {
	reset(t)

	// Check if the OTP has expired.
	o, err := rStore.Check(mockOTP.Namespace, mockOTP.ID, false)
	assert.Equal(t, nil, err, "error checking OTP")
	assert.Equal(t, mockOTP.TTL, o.TTL, "TTL doesn't match")
}
func TestStoreClose(t *testing.T) {
	reset(t)

	err := rStore.Close(mockOTP.Namespace, mockOTP.ID)
	assert.Equal(t, nil, err, "error closing OTP")

	o, err := rStore.Check(mockOTP.Namespace, mockOTP.ID, false)
	assert.Equal(t, true, o.Closed, "OTP didn't close")
}
func TestStoreDelete(t *testing.T) {
	reset(t)

	err := rStore.Delete(mockOTP.Namespace, mockOTP.ID)
	assert.Equal(t, nil, err, "error deleting OTP")

	_, err = rStore.Check(mockOTP.Namespace, mockOTP.ID, false)
	assert.Equal(t, store.ErrNotExist, err, "OTP wasn't deleted")
}
