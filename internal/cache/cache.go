// Package cache stores /extract results so a repeated URL costs neither a page
// fetch nor an LLM call. Values are opaque bytes (the JSON response), which
// keeps this package free of any dependency on the service layer.
package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// keyPrefix namespaces entries and versions the cached response shape. Bump the
// version to invalidate every entry at once after a response-format change.
const keyPrefix = "linkmeta:v1:"

// Store is a TTL key/value store. Implementations must be safe for concurrent
// use, and must never block a request longer than their configured timeout.
type Store interface {
	// Get returns the value and whether it was present. A miss is (nil, false, nil);
	// an error means the backend failed and the caller should treat it as a miss.
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, val []byte, ttl time.Duration) error
	Close() error
}

// Key builds the cache key. Every input that can change the response must be
// part of it: the URL, the effective category language, and a fingerprint of the
// settings that shape extraction (see service.CacheFingerprint). Redis entries
// outlive the process, so a config change must not serve results built under the
// previous configuration.
func Key(url, lang, configFingerprint string) string {
	sum := sha256.Sum256([]byte(url + "\x00" + lang + "\x00" + configFingerprint))
	return keyPrefix + hex.EncodeToString(sum[:])
}
