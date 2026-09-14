package auth

import (
	"context"
	"sync"
	"time"

	"thinkboard-backend/internal/shared/clock"
	"thinkboard-backend/internal/shared/kernel"
)

type cacheEntry struct {
	value     bool
	expiresAt time.Time
}

// ttlCache is a tiny TTL-bounded cache, generic over the key shape (§5.6 cache.go: "TTL <= 60s").
type ttlCache[K comparable] struct {
	mu      sync.Mutex
	ttl     time.Duration
	clock   clock.Clock
	entries map[K]cacheEntry
}

func newTTLCache[K comparable](ttl time.Duration, c clock.Clock) *ttlCache[K] {
	return &ttlCache[K]{ttl: ttl, clock: c, entries: make(map[K]cacheEntry)}
}

func (c *ttlCache[K]) get(key K) (bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || c.clock.Now().After(e.expiresAt) {
		return false, false
	}
	return e.value, true
}

func (c *ttlCache[K]) set(key K, value bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = cacheEntry{value: value, expiresAt: c.clock.Now().Add(c.ttl)}
}

type membershipKey struct {
	profile kernel.ProfileID
	team    kernel.TeamID
}

type sessionKey struct {
	profile kernel.ProfileID
	session kernel.SessionID
}

// CachedAuthorizer wraps an Authorizer with a short TTL cache on the three membership checks that
// sit on every RLS-equivalent authorization path. ResolveProfileID passes through uncached — it's
// a single indexed lookup, not the repeated per-request check the cache exists for.
type CachedAuthorizer struct {
	*Authorizer
	members  *ttlCache[membershipKey]
	leaders  *ttlCache[membershipKey]
	sessions *ttlCache[sessionKey]
}

// NewCachedAuthorizer wraps inner with a cache of the given TTL (§5.6 default: 60s).
func NewCachedAuthorizer(inner *Authorizer, ttl time.Duration, c clock.Clock) *CachedAuthorizer {
	return &CachedAuthorizer{
		Authorizer: inner,
		members:    newTTLCache[membershipKey](ttl, c),
		leaders:    newTTLCache[membershipKey](ttl, c),
		sessions:   newTTLCache[sessionKey](ttl, c),
	}
}

func (c *CachedAuthorizer) IsTeamMember(ctx context.Context, profileID kernel.ProfileID, teamID kernel.TeamID) (bool, error) {
	key := membershipKey{profileID, teamID}
	if v, ok := c.members.get(key); ok {
		return v, nil
	}
	ok, err := c.Authorizer.IsTeamMember(ctx, profileID, teamID)
	if err != nil {
		return false, err
	}
	c.members.set(key, ok)
	return ok, nil
}

func (c *CachedAuthorizer) IsTeamLeader(ctx context.Context, profileID kernel.ProfileID, teamID kernel.TeamID) (bool, error) {
	key := membershipKey{profileID, teamID}
	if v, ok := c.leaders.get(key); ok {
		return v, nil
	}
	ok, err := c.Authorizer.IsTeamLeader(ctx, profileID, teamID)
	if err != nil {
		return false, err
	}
	c.leaders.set(key, ok)
	return ok, nil
}

func (c *CachedAuthorizer) CanAccessSession(ctx context.Context, profileID kernel.ProfileID, sessionID kernel.SessionID) (bool, error) {
	key := sessionKey{profileID, sessionID}
	if v, ok := c.sessions.get(key); ok {
		return v, nil
	}
	ok, err := c.Authorizer.CanAccessSession(ctx, profileID, sessionID)
	if err != nil {
		return false, err
	}
	c.sessions.set(key, ok)
	return ok, nil
}
