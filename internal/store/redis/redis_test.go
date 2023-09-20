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
	"github.com/stretchr/testify/require"
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

func setup(t *testing.T) *Redis {
	rdis.FlushDB()
	_, err := rStore.Set(mockOTP.Namespace, mockOTP.ID, mockOTP)
	require.NoError(t, err, "Failed to set up test OTP")

	t.Cleanup(func() {
		rdis.FlushDB()
	})

	return rStore
}

func TestStoreSet(t *testing.T) {
	rStore := setup(t)

	resp, err := rStore.Set(mockOTP.Namespace, mockOTP.ID, mockOTP)
	assert.NoError(t, err, "Error setting OTP")

	cmp := mockOTP
	// Override dynamic values.
	cmp.Attempts = resp.Attempts
	cmp.TTL = resp.TTL
	cmp.TTLSeconds = resp.TTLSeconds
	assert.Equal(t, cmp, resp, "Returned OTP doesn't match expected OTP")
}

func TestStoreCheck(t *testing.T) {
	rStore := setup(t)

	t.Run("no increment", func(t *testing.T) {
		o, err := rStore.Check(mockOTP.Namespace, mockOTP.ID, false)
		assert.NoError(t, err, "Error checking OTP without increment")
		assert.Equal(t, 1, o.Attempts, "Unexpected attempt count")
	})

	t.Run("with increment", func(t *testing.T) {
		o, err := rStore.Check(mockOTP.Namespace, mockOTP.ID, true)
		assert.NoError(t, err, "Error checking OTP with increment")
		assert.Equal(t, 2, o.Attempts, "Unexpected attempt count after first increment")

		o, err = rStore.Check(mockOTP.Namespace, mockOTP.ID, true)
		assert.NoError(t, err, "Error checking OTP with second increment")
		assert.Equal(t, 3, o.Attempts, "Unexpected attempt count after second increment")
	})
}

func TestStoreTTL(t *testing.T) {
	rStore := setup(t)

	o, err := rStore.Check(mockOTP.Namespace, mockOTP.ID, false)
	assert.NoError(t, err, "Error checking OTP")
	assert.Equal(t, mockOTP.TTL, o.TTL, "Returned OTP TTL doesn't match expected TTL")
}

func TestStoreClose(t *testing.T) {
	rStore := setup(t)

	err := rStore.Close(mockOTP.Namespace, mockOTP.ID)
	assert.NoError(t, err, "Error closing OTP")

	o, err := rStore.Check(mockOTP.Namespace, mockOTP.ID, false)
	assert.NoError(t, err, "Error checking closed OTP")
	assert.True(t, o.Closed, "OTP should be closed but isn't")
}

func TestStoreDelete(t *testing.T) {
	rStore := setup(t)

	err := rStore.Delete(mockOTP.Namespace, mockOTP.ID)
	assert.NoError(t, err, "Error deleting OTP")

	_, err = rStore.Check(mockOTP.Namespace, mockOTP.ID, false)
	assert.Equal(t, store.ErrNotExist, err, "OTP should not exist but it does")
}
