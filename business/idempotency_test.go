package business

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Канонічна regexp для UUID v4: 8-4-4-4-12 hex-цифр, з версією 4 і
// variant у [8,9,a,b].
var uuidV4Re = regexp.MustCompile(
	`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`,
)

func TestNewIdempotencyKey_format(t *testing.T) {
	k, err := NewIdempotencyKey()
	require.NoError(t, err)
	require.Len(t, k, 36, "UUID v4 — це 36 символів з тире")
	assert.Regexp(t, uuidV4Re, k)
	_ = err
}

func TestNewIdempotencyKey_versionAndVariantBits(t *testing.T) {
	// Версія — 13-й символ (індекс 14, після третього "-") має бути '4'.
	// Variant — індекс 19 має бути одним з 8/9/a/b.
	for i := 0; i < 32; i++ {
		k, err := NewIdempotencyKey()
		require.NoError(t, err)
		assert.Equalf(t, byte('4'), k[14], "version nibble for key %q", k)
		v := k[19]
		assert.Containsf(t, "89ab", string(v), "variant nibble for key %q", k)
	}
}

func TestNewIdempotencyKey_unique(t *testing.T) {
	const n = 1000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		k, err := NewIdempotencyKey()
		require.NoError(t, err)
		_, dup := seen[k]
		require.False(t, dup, "collision after %d keys: %q", i, k)
		seen[k] = struct{}{}
	}
}
