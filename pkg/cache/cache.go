package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/oetiker/response-analyzer/pkg/logging"
)

// CacheEntry represents a cached item
type CacheEntry struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Cache provides caching functionality
type Cache struct {
	logger    *logging.Logger
	cacheDir  string
	entries   map[string]*CacheEntry
	mutex     sync.RWMutex
	ttl       time.Duration
	persisted bool
}

// NewCache creates a new Cache instance
func NewCache(logger *logging.Logger, cacheDir string, ttl time.Duration, persisted bool) (*Cache, error) {
	cache := &Cache{
		logger:    logger,
		cacheDir:  cacheDir,
		entries:   make(map[string]*CacheEntry),
		ttl:       ttl,
		persisted: persisted,
	}

	// Create cache directory if it doesn't exist
	if persisted {
		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create cache directory: %w", err)
		}
	}

	// Load cached entries if persisted
	if persisted {
		if err := cache.loadEntries(); err != nil {
			logger.Warn("Failed to load cached entries", "error", err)
		}
	}

	return cache, nil
}

// Get retrieves a value from the cache
func (c *Cache) Get(key string) (string, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	// Generate hash key
	hashedKey := hashKey(key)

	// Check if entry exists
	entry, ok := c.entries[hashedKey]
	if !ok {
		return "", false
	}

	// Check if entry has expired
	if time.Now().After(entry.ExpiresAt) {
		c.logger.Debug("Cache entry expired", "key", key)
		delete(c.entries, hashedKey)
		if c.persisted {
			// Remove the file asynchronously
			go func() {
				filePath := filepath.Join(c.cacheDir, hashedKey+".json")
				if err := os.Remove(filePath); err != nil {
					c.logger.Warn("Failed to remove expired cache file", "path", filePath, "error", err)
				}
			}()
		}
		return "", false
	}

	c.logger.Debug("Cache hit", "key", key)
	return entry.Value, true
}

// Set stores a value in the cache
func (c *Cache) Set(key, value string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Generate hash key
	hashedKey := hashKey(key)

	// Create entry
	now := time.Now()
	entry := &CacheEntry{
		Key:       key,
		Value:     value,
		CreatedAt: now,
		ExpiresAt: now.Add(c.ttl),
	}

	// Store in memory
	c.entries[hashedKey] = entry

	// Persist to disk if enabled
	if c.persisted {
		if err := c.persistEntry(hashedKey, entry); err != nil {
			return fmt.Errorf("failed to persist cache entry: %w", err)
		}
	}

	c.logger.Debug("Cache set", "key", key)
	return nil
}

// Clear removes all entries from the cache
func (c *Cache) Clear() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Clear memory cache
	c.entries = make(map[string]*CacheEntry)

	// Clear persisted cache if enabled
	if c.persisted {
		files, err := filepath.Glob(filepath.Join(c.cacheDir, "*.json"))
		if err != nil {
			return fmt.Errorf("failed to list cache files: %w", err)
		}

		for _, file := range files {
			if err := os.Remove(file); err != nil {
				c.logger.Warn("Failed to remove cache file", "path", file, "error", err)
			}
		}
	}

	c.logger.Info("Cache cleared")
	return nil
}

// persistEntry saves a cache entry to disk
func (c *Cache) persistEntry(hashedKey string, entry *CacheEntry) error {
	// Marshal entry to JSON
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal cache entry: %w", err)
	}

	// Write to file
	filePath := filepath.Join(c.cacheDir, hashedKey+".json")
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	return nil
}

// loadEntries loads all cached entries from disk
func (c *Cache) loadEntries() error {
	c.logger.Info("Loading cached entries", "dir", c.cacheDir)

	// Find all cache files
	files, err := filepath.Glob(filepath.Join(c.cacheDir, "*.json"))
	if err != nil {
		return fmt.Errorf("failed to list cache files: %w", err)
	}

	// Load each file
	for _, file := range files {
		// Read file
		data, err := os.ReadFile(file)
		if err != nil {
			c.logger.Warn("Failed to read cache file", "path", file, "error", err)
			continue
		}

		// Unmarshal entry
		var entry CacheEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			c.logger.Warn("Failed to unmarshal cache entry", "path", file, "error", err)
			continue
		}

		// Check if entry has expired
		if time.Now().After(entry.ExpiresAt) {
			c.logger.Debug("Skipping expired cache entry", "key", entry.Key)
			if err := os.Remove(file); err != nil {
				c.logger.Warn("Failed to remove expired cache file", "path", file, "error", err)
			}
			continue
		}

		// Store in memory
		hashedKey := hashKey(entry.Key)
		c.entries[hashedKey] = &entry
	}

	c.logger.Info("Loaded cached entries", "count", len(c.entries))
	return nil
}

// hashKey creates a hash of the key for file naming
func hashKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}
