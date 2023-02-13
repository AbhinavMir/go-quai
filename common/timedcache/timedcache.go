package timedcache

import (
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru"
)

// timedEntry provides a wrapper to store an entry in an LRU cache, with a
// specified expiration time
type timedEntry struct {
	expiresAt int64
	value     interface{}
}

// expired returns whether or not the given entry has expired
func (te *timedEntry) expired() bool {
	return te.expiresAt < time.Now().Unix()
}

// TimedCache defines a new cache, where entries are removed after exceeding
// their ttl. The entry is not guaranteed to live this long (i.e. if it gets
// evicted when the cache fills up). Conversely, the entry also isn't guaranteed
// to expire at exactly the ttl time. The expiration mechanism is 'lazy', and
// will only remove expired objects at next access.
type TimedCache struct {
	ttl   int64     // Time (in seconds) each entry is allowed to live for
	cache lru.Cache // Underlying size-limited LRU cache
	lock  sync.RWMutex
}

// New creates a new cache with a given size and ttl. TTL defines the time in
// seconds an entry shall live, before being expired.
func New(size int, ttl int) (*TimedCache, error) {
	cache, err := lru.New(size)
	if err != nil {
		return &TimedCache{}, err
	}
	return &TimedCache{ttl: int64(ttl), cache: *cache}, nil
}

// calcExpireTime calculates the expiration time given a TTL relative to now.
func calcExpireTime(ttl int64) int64 {
	t := time.Now().Unix() + ttl
	return t
}

// removeExpired removes any expired entries from the cache
func (tc *TimedCache) removeExpired() {
	for k := range tc.cache.Keys() {
		if val, ok := tc.cache.Peek(k); ok {
			if v := val.(timedEntry); v.expired() {
				tc.cache.Remove(k)
			}
		}
	}
}

// Purge is used to completely clear the cache.
func (tc *TimedCache) Purge() {
	tc.lock.Lock()
	defer tc.lock.Unlock()
	tc.cache.Purge()
}

// Add adds a value to the cache. Returns true if an eviction occurred.
func (tc *TimedCache) Add(key, value interface{}) (evicted bool) {
	tc.lock.Lock()
	defer tc.lock.Unlock()
	// First remove expired entries, so that LRU cache doesn't evict more than
	// necessary, if there is not enough room to add this entry.
	tc.removeExpired()
	// Wrap the entry and add it to the cache
	return tc.cache.Add(key, timedEntry{expiresAt: calcExpireTime(tc.ttl), value: value})
}

// Get looks up a key's value from the cache, removing it if it has expired.
func (tc *TimedCache) Get(key interface{}) (value interface{}, ok bool) {
	tc.lock.Lock()
	defer tc.lock.Unlock()
	val, ok := tc.cache.Get(key)
	if ok {
		v := val.(timedEntry)
		if v.expired() {
			tc.cache.Remove(key)
			return nil, false
		} else {
			return v.value, true
		}
	} else {
		return nil, false
	}
}

// Contains checks if a key is in the cache, without updating the
// recent-ness or deleting it for being stale.
func (tc *TimedCache) Contains(key interface{}) bool {
	_, ok := tc.Peek(key)
	return ok
}

// Peek returns the key value (or undefined if not found) without updating
// the "recently used"-ness or ttl of the key.
func (tc *TimedCache) Peek(key interface{}) (value interface{}, ok bool) {
	tc.lock.Lock()
	defer tc.lock.Unlock()
	val, ok := tc.cache.Peek(key)
	if ok {
		v := val.(timedEntry)
		if v.expired() {
			tc.cache.Remove(key)
			return nil, false
		} else {
			return v.value, ok
		}
	} else {
		return nil, false
	}
}

// ContainsOrAdd checks if a key is in the cache without updating the
// recent-ness, ttl, or deleting it for being stale, and if not, adds the value.
// Returns whether found and whether an eviction occurred.
func (tc *TimedCache) ContainsOrAdd(key, value interface{}) (ok, evicted bool) {
	tc.lock.Lock()
	defer tc.lock.Unlock()
	// First remove expired entries, so that LRU cache doesn't evict more than
	// necessary, if there is not enough room to add this entry.
	tc.removeExpired()
	// Wrap the entry and add it to the cache
	return tc.cache.ContainsOrAdd(key, timedEntry{expiresAt: calcExpireTime(tc.ttl), value: value})
}

// PeekOrAdd checks if a key is in the cache without updating the
// recent-ness, ttl, or deleting it for being stale, and if not, adds the value.
// Returns whether found and whether an eviction occurred.
func (tc *TimedCache) PeekOrAdd(key, value interface{}) (previous interface{}, ok, evicted bool) {
	tc.lock.Lock()
	defer tc.lock.Unlock()
	// First remove expired entries, so that LRU cache doesn't evict more than
	// necessary, if there is not enough room to add this entry.
	tc.removeExpired()
	// Wrap the entry and add it to the cache
	return tc.cache.PeekOrAdd(key, timedEntry{expiresAt: calcExpireTime(tc.ttl), value: value})
}

// Remove removes the provided key from the cache.
func (tc *TimedCache) Remove(key interface{}) (present bool) {
	tc.lock.Lock()
	defer tc.lock.Unlock()
	tc.removeExpired()
	return tc.cache.Remove(key)
}

// Resize changes the cache size.
func (tc *TimedCache) Resize(size int) (evicted int) {
	tc.lock.Lock()
	defer tc.lock.Unlock()
	tc.removeExpired()
	return tc.cache.Resize(size)
}

// RemoveOldest removes the oldest item from the cache.
func (tc *TimedCache) RemoveOldest() (key, value interface{}, ok bool) {
	tc.lock.Lock()
	defer tc.lock.Unlock()
	tc.removeExpired()
	k, v, ok := tc.cache.RemoveOldest()
	if ok {
		v = v.(timedEntry).value
	}
	return k, v, ok
}

// GetOldest returns the oldest entry
func (tc *TimedCache) GetOldest() (key, value interface{}, ok bool) {
	tc.lock.Lock()
	defer tc.lock.Unlock()
	tc.removeExpired()
	k, v, ok := tc.cache.GetOldest()
	if ok {
		v = v.(timedEntry).value
	}
	return k, v, ok
}

// Keys returns a slice of the keys in the cache, from oldest to newest.
func (tc *TimedCache) Keys() []interface{} {
	tc.lock.Lock()
	defer tc.lock.Unlock()
	tc.removeExpired()
	return tc.cache.Keys()
}

// Len returns the number of items in the cache.
func (tc *TimedCache) Len() int {
	tc.lock.Lock()
	defer tc.lock.Unlock()
	tc.removeExpired()
	return tc.cache.Len()
}

// Ttl returns the number of seconds each item is allowed to live (except if
// evicted to free up space)
func (tc *TimedCache) Ttl() int64 {
	tc.lock.RLock()
	defer tc.lock.RUnlock()
	return tc.ttl
}
