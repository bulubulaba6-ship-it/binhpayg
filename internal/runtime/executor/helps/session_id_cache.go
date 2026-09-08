package helps

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

type sessionIDCacheEntry struct {
	value  string
	expire time.Time
}

var (
	sessionIDCache            = make(map[string]sessionIDCacheEntry)
	sessionIDCacheMu          sync.RWMutex
	sessionIDCacheCleanupOnce sync.Once
)

const (
	sessionIDTTL                = time.Hour
	sessionIDCacheCleanupPeriod = 15 * time.Minute
)

func startSessionIDCacheCleanup() {
	go func() {
		ticker := time.NewTicker(sessionIDCacheCleanupPeriod)
		defer ticker.Stop()
		for range ticker.C {
			purgeExpiredSessionIDs()
		}
	}()
}

func purgeExpiredSessionIDs() {
	now := time.Now()
	sessionIDCacheMu.Lock()
	for key, entry := range sessionIDCache {
		if !entry.expire.After(now) {
			delete(sessionIDCache, key)
		}
	}
	sessionIDCacheMu.Unlock()
}

func sessionIDCacheKey(apiKey string) string {
	sum := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(sum[:])
}

// CachedSessionID returns a stable session UUID per apiKey, refreshing the TTL on each access.
func CachedSessionID(apiKey string) string {
	if apiKey == "" {
		return uuid.New().String()
	}

	sessionIDCacheCleanupOnce.Do(startSessionIDCacheCleanup)

	key := sessionIDCacheKey(apiKey)
	now := time.Now()

	sessionIDCacheMu.RLock()
	entry, ok := sessionIDCache[key]
	valid := ok && entry.value != "" && entry.expire.After(now)
	sessionIDCacheMu.RUnlock()
	if valid {
		sessionIDCacheMu.Lock()
		entry = sessionIDCache[key]
		if entry.value != "" && entry.expire.After(now) {
			entry.expire = now.Add(sessionIDTTL)
			sessionIDCache[key] = entry
			sessionIDCacheMu.Unlock()
			return entry.value
		}
		sessionIDCacheMu.Unlock()
	}

	newID := uuid.New().String()

	sessionIDCacheMu.Lock()
	entry, ok = sessionIDCache[key]
	if !ok || entry.value == "" || !entry.expire.After(now) {
		entry.value = newID
	}
	entry.expire = now.Add(sessionIDTTL)
	sessionIDCache[key] = entry
	sessionIDCacheMu.Unlock()
	return entry.value
}

// EnsureSessionHeader ensures that an outbound request to an upstream provider has a valid session header
// (e.g. x-opencode-session for OpenCode Go, or session-id).
func EnsureSessionHeader(req *http.Request, apiKey string) {
	if req == nil {
		return
	}
	sessID := req.Header.Get("x-opencode-session")
	if sessID == "" {
		sessID = req.Header.Get("session-id")
	}
	if sessID == "" {
		sessID = req.Header.Get("x-session-id")
	}
	if sessID == "" {
		sessID = CachedSessionID(apiKey)
	}
	req.Header.Set("x-opencode-session", sessID)
}
