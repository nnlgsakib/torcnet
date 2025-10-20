package db

import (
	"bytes"
	"errors"
	"sort"
	"sync"
)

// MemoryDatabase is an in-memory implementation of the Database interface
type MemoryDatabase struct {
	data map[string][]byte
	mu   sync.RWMutex
}

// NewMemoryDB creates a new in-memory database
func NewMemoryDB() Database {
	return &MemoryDatabase{
		data: make(map[string][]byte),
	}
}

// Open opens the database (no-op for memory database)
func (mdb *MemoryDatabase) Open(path string) error {
	return nil
}

// Close closes the database (no-op for memory database)
func (mdb *MemoryDatabase) Close() error {
	mdb.mu.Lock()
	defer mdb.mu.Unlock()
	mdb.data = nil
	return nil
}

// Put stores a key-value pair
func (mdb *MemoryDatabase) Put(key, value []byte) error {
	mdb.mu.Lock()
	defer mdb.mu.Unlock()
	
	mdb.data[string(key)] = make([]byte, len(value))
	copy(mdb.data[string(key)], value)
	return nil
}

// Get retrieves a value by key
func (mdb *MemoryDatabase) Get(key []byte) ([]byte, error) {
	mdb.mu.RLock()
	defer mdb.mu.RUnlock()
	
	value, exists := mdb.data[string(key)]
	if !exists {
		return nil, errors.New("key not found")
	}
	
	result := make([]byte, len(value))
	copy(result, value)
	return result, nil
}

// Delete removes a key-value pair
func (mdb *MemoryDatabase) Delete(key []byte) error {
	mdb.mu.Lock()
	defer mdb.mu.Unlock()
	
	delete(mdb.data, string(key))
	return nil
}

// Has checks if a key exists
func (mdb *MemoryDatabase) Has(key []byte) (bool, error) {
	mdb.mu.RLock()
	defer mdb.mu.RUnlock()
	
	_, exists := mdb.data[string(key)]
	return exists, nil
}

// Iterator returns an iterator over a key range
func (mdb *MemoryDatabase) Iterator(start, end []byte) (Iterator, error) {
	mdb.mu.RLock()
	defer mdb.mu.RUnlock()
	
	var keys []string
	for key := range mdb.data {
		keyBytes := []byte(key)
		if (start == nil || bytes.Compare(keyBytes, start) >= 0) &&
		   (end == nil || bytes.Compare(keyBytes, end) < 0) {
			keys = append(keys, key)
		}
	}
	
	sort.Strings(keys)
	
	return &MemoryIterator{
		db:   mdb,
		keys: keys,
		pos:  -1,
	}, nil
}

// Batch returns a batch for atomic updates
func (mdb *MemoryDatabase) Batch() Batch {
	return &MemoryBatch{
		db:      mdb,
		puts:    make(map[string][]byte),
		deletes: make(map[string]bool),
	}
}

// MemoryIterator implements Iterator for memory database
type MemoryIterator struct {
	db   *MemoryDatabase
	keys []string
	pos  int
}

// Next moves the iterator to the next key
func (mi *MemoryIterator) Next() bool {
	mi.pos++
	return mi.pos < len(mi.keys)
}

// Key returns the current key
func (mi *MemoryIterator) Key() []byte {
	if mi.pos < 0 || mi.pos >= len(mi.keys) {
		return nil
	}
	return []byte(mi.keys[mi.pos])
}

// Value returns the current value
func (mi *MemoryIterator) Value() []byte {
	if mi.pos < 0 || mi.pos >= len(mi.keys) {
		return nil
	}
	
	mi.db.mu.RLock()
	defer mi.db.mu.RUnlock()
	
	value := mi.db.data[mi.keys[mi.pos]]
	result := make([]byte, len(value))
	copy(result, value)
	return result
}

// Error returns any accumulated error
func (mi *MemoryIterator) Error() error {
	return nil
}

// Close releases associated resources
func (mi *MemoryIterator) Close() error {
	return nil
}

// MemoryBatch implements Batch for memory database
type MemoryBatch struct {
	db      *MemoryDatabase
	puts    map[string][]byte
	deletes map[string]bool
}

// Put adds a put operation to the batch
func (mb *MemoryBatch) Put(key, value []byte) error {
	mb.puts[string(key)] = make([]byte, len(value))
	copy(mb.puts[string(key)], value)
	delete(mb.deletes, string(key))
	return nil
}

// Delete adds a delete operation to the batch
func (mb *MemoryBatch) Delete(key []byte) error {
	mb.deletes[string(key)] = true
	delete(mb.puts, string(key))
	return nil
}

// Write executes all operations in the batch
func (mb *MemoryBatch) Write() error {
	mb.db.mu.Lock()
	defer mb.db.mu.Unlock()
	
	for key := range mb.deletes {
		delete(mb.db.data, key)
	}
	
	for key, value := range mb.puts {
		mb.db.data[key] = make([]byte, len(value))
		copy(mb.db.data[key], value)
	}
	
	return nil
}

// Reset clears all operations in the batch
func (mb *MemoryBatch) Reset() {
	mb.puts = make(map[string][]byte)
	mb.deletes = make(map[string]bool)
}