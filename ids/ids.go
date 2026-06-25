package ids

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// NewUUIDV7 returns a time-sortable UUID version 7 string.
func NewUUIDV7() (string, error) {
	var b [16]byte

	// UUIDv7 stores the Unix timestamp in milliseconds in the first 48 bits.
	unixMillis := uint64(time.Now().UnixMilli())
	b[0] = byte(unixMillis >> 40)
	b[1] = byte(unixMillis >> 32)
	b[2] = byte(unixMillis >> 24)
	b[3] = byte(unixMillis >> 16)
	b[4] = byte(unixMillis >> 8)
	b[5] = byte(unixMillis)

	// Fill the remaining bytes with randomness before setting version and variant bits.
	if _, err := rand.Read(b[6:]); err != nil {
		return "", fmt.Errorf("generate uuidv7 random bytes: %w", err)
	}

	// Set version 7 in the high nibble of byte 6.
	b[6] = (b[6] & 0x0f) | 0x70

	// Set the RFC 4122 variant bits: 10xxxxxx.
	b[8] = (b[8] & 0x3f) | 0x80

	return formatUUID(b), nil
}

// MustNewUUIDV7 returns a time-sortable UUID version 7 string or panics.
func MustNewUUIDV7() string {
	id, err := NewUUIDV7()
	if err != nil {
		panic(err)
	}
	return id
}

// formatUUID renders raw UUID bytes in the canonical 8-4-4-4-12 form.
func formatUUID(b [16]byte) string {
	var dst [36]byte

	// Encode the five UUID groups and insert separators manually to avoid fmt allocation overhead.
	hex.Encode(dst[0:8], b[0:4])
	dst[8] = '-'
	hex.Encode(dst[9:13], b[4:6])
	dst[13] = '-'
	hex.Encode(dst[14:18], b[6:8])
	dst[18] = '-'
	hex.Encode(dst[19:23], b[8:10])
	dst[23] = '-'
	hex.Encode(dst[24:36], b[10:16])

	return string(dst[:])
}
