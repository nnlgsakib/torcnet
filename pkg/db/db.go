package db

import "errors"

// Database defines the interface for database operations
type Database interface {
	// Open opens the database
	Open(path string) error
	
	// Close closes the database
	Close() error
	
	// Put stores a key-value pair
	Put(key, value []byte) error
	
	// Get retrieves a value by key
	Get(key []byte) ([]byte, error)
	
	// Delete removes a key-value pair
	Delete(key []byte) error
	
	// Has checks if a key exists
	Has(key []byte) (bool, error)
	
	// Iterator returns an iterator over a key range
	Iterator(start, end []byte) (Iterator, error)
	
	// Batch returns a batch for atomic updates
	Batch() Batch
}

// Iterator defines the interface for database iterators
type Iterator interface {
	// Next moves the iterator to the next key
	Next() bool
	
	// Key returns the current key
	Key() []byte
	
	// Value returns the current value
	Value() []byte
	
	// Error returns any accumulated error
	Error() error
	
	// Close releases associated resources
	Close() error
}

// Batch defines the interface for batch operations
type Batch interface {
	// Put adds a key-value pair to the batch
	Put(key, value []byte) error
	
	// Delete adds a key deletion to the batch
	Delete(key []byte) error
	
	// Write writes the batch to the database
	Write() error
	
	// Reset resets the batch
	Reset()
}

// DBType represents the type of database
type DBType string

const (
	// LevelDB database type
	LevelDB DBType = "leveldb"
	
	// PebbleDB database type
	PebbleDB DBType = "pebble"
	
	// RocksDB database type
	RocksDB DBType = "rocksdb"
)

// NewDatabase creates a new database of the specified type
func NewDatabase(dbType DBType) (Database, error) {
	switch dbType {
	case LevelDB:
		return NewLevelDB(), nil
	case PebbleDB:
		return NewPebbleDB(), nil
	case RocksDB:
		return NewRocksDB(), nil
	default:
		return nil, errors.New("unsupported database type")
	}
}