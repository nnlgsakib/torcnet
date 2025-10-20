package db

import (
	"github.com/cockroachdb/pebble"
)

// PebbleDBDatabase is a PebbleDB implementation of the Database interface
type PebbleDBDatabase struct {
	db *pebble.DB
}

// NewPebbleDB creates a new PebbleDB database
func NewPebbleDB() Database {
	return &PebbleDBDatabase{}
}

// Open opens the database
func (pdb *PebbleDBDatabase) Open(path string) error {
	db, err := pebble.Open(path, &pebble.Options{})
	if err != nil {
		return err
	}
	pdb.db = db
	return nil
}

// Close closes the database
func (pdb *PebbleDBDatabase) Close() error {
	return pdb.db.Close()
}

// Put stores a key-value pair
func (pdb *PebbleDBDatabase) Put(key, value []byte) error {
	return pdb.db.Set(key, value, pebble.Sync)
}

// Get retrieves a value by key
func (pdb *PebbleDBDatabase) Get(key []byte) ([]byte, error) {
	value, closer, err := pdb.db.Get(key)
	if err != nil {
		return nil, err
	}
	defer closer.Close()
	
	// Copy the value since it's only valid until closer.Close()
	result := make([]byte, len(value))
	copy(result, value)
	return result, nil
}

// Delete removes a key-value pair
func (pdb *PebbleDBDatabase) Delete(key []byte) error {
	return pdb.db.Delete(key, pebble.Sync)
}

// Has checks if a key exists
func (pdb *PebbleDBDatabase) Has(key []byte) (bool, error) {
	_, closer, err := pdb.db.Get(key)
	if err == pebble.ErrNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	closer.Close()
	return true, nil
}

// Iterator returns an iterator over a key range
func (pdb *PebbleDBDatabase) Iterator(start, end []byte) (Iterator, error) {
	iter, err := pdb.db.NewIter(&pebble.IterOptions{
		LowerBound: start,
		UpperBound: end,
	})
	if err != nil {
		return nil, err
	}
	return &PebbleDBIterator{iter: iter}, nil
}

// Batch returns a batch for atomic updates
func (pdb *PebbleDBDatabase) Batch() Batch {
	return &PebbleDBBatch{
		batch: pdb.db.NewBatch(),
	}
}

// PebbleDBIterator is a PebbleDB implementation of the Iterator interface
type PebbleDBIterator struct {
	iter   *pebble.Iterator
}

// Next moves the iterator to the next key
func (it *PebbleDBIterator) Next() bool {
	return it.iter.Next()
}

// Key returns the current key
func (it *PebbleDBIterator) Key() []byte {
	return it.iter.Key()
}

// Value returns the current value
func (it *PebbleDBIterator) Value() []byte {
	return it.iter.Value()
}

// Error returns any accumulated error
func (it *PebbleDBIterator) Error() error {
	return it.iter.Error()
}

// Close releases associated resources
func (it *PebbleDBIterator) Close() error {
	return it.iter.Close()
}

// PebbleDBBatch is a PebbleDB implementation of the Batch interface
type PebbleDBBatch struct {
	batch *pebble.Batch
}

// Put adds a key-value pair to the batch
func (b *PebbleDBBatch) Put(key, value []byte) error {
	return b.batch.Set(key, value, nil)
}

// Delete adds a key deletion to the batch
func (b *PebbleDBBatch) Delete(key []byte) error {
	return b.batch.Delete(key, nil)
}

// Write writes the batch to the database
func (b *PebbleDBBatch) Write() error {
	return b.batch.Commit(pebble.Sync)
}

// Reset resets the batch
func (b *PebbleDBBatch) Reset() {
	b.batch.Reset()
}