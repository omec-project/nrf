// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"sync"
	"time"

	"github.com/omec-project/openapi/v2/models"
)

type profileCacheEntry struct {
	profile   models.NFProfileDiscovery
	expiresAt time.Time
}

// nfProfileCache is a TTL cache of decoded NF profiles keyed by nfInstanceId.
// It eliminates redundant MongoDB fetches and JSON decodes for stable profiles.
type nfProfileCache struct {
	mu      sync.RWMutex
	entries map[string]profileCacheEntry
}

var profileCache = &nfProfileCache{
	entries: make(map[string]profileCacheEntry),
}

func (c *nfProfileCache) get(nfInstanceID string) (models.NFProfileDiscovery, bool) {
	c.mu.RLock()
	entry, ok := c.entries[nfInstanceID]
	c.mu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		return models.NFProfileDiscovery{}, false
	}
	return entry.profile, true
}

func (c *nfProfileCache) set(profile models.NFProfileDiscovery, expiresAt time.Time) {
	id := profile.GetNfInstanceId()
	if id == "" || expiresAt.IsZero() || !expiresAt.After(time.Now()) {
		return
	}
	c.mu.Lock()
	c.entries[id] = profileCacheEntry{profile: profile, expiresAt: expiresAt}
	c.mu.Unlock()
}

func (c *nfProfileCache) evict(nfInstanceID string) {
	c.mu.Lock()
	delete(c.entries, nfInstanceID)
	c.mu.Unlock()
}

func (c *nfProfileCache) evictByNfType(nfType string) {
	c.mu.Lock()
	for id, entry := range c.entries {
		if string(entry.profile.GetNfType()) == nfType {
			delete(c.entries, id)
		}
	}
	c.mu.Unlock()
}
