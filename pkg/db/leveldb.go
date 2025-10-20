package db

import (
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/iterator"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
)

// LevelDBDatabase is a LevelDB implementation of the Database interface
type LevelDBDatabase struct {
	db *leveldb.DB
}

// NewLevelDB creates a new LevelDB database
func NewLevelDB() Database {
	return &LevelDBDatabase{}
}

// Open opens the database
func (ldb *LevelDBDatabase) Open(path string) error {
	db, err := leveldb.OpenFile(path, nil)
	if err != nil {
		return err
	}
	ldb.db = db
	return nil
}

// Close closes the database
func (ldb *LevelDBDatabase) Close() error {
	return ldb.db.Close()
}

// Put stores a key-value pair
func (ldb *LevelDBDatabase) Put(key, value []byte) error {
	return ldb.db.Put(key, value, nil)
}

// Get retrieves a value by key
func (ldb *LevelDBDatabase) Get(key []byte) ([]byte, error) {
	return ldb.db.Get(key, nil)
}

// Delete removes a key-value pair
func (ldb *LevelDBDatabase) Delete(key []byte) error {
	return ldb.db.Delete(key, nil)
}

// Has checks if a key exists
func (ldb *LevelDBDatabase) Has(key []byte) (bool, error) {
	return ldb.db.Has(key, nil)
}

// Iterator returns an iterator over a key range
func (ldb *LevelDBDatabase) Iterator(start, end []byte) (Iterator, error) {
	return &LevelDBIterator{
		iter: ldb.db.NewIterator(&util.Range{Start: start, Limit: end}, nil),
	}, nil
}

// Batch returns a batch for atomic updates
func (ldb *LevelDBDatabase) Batch() Batch {
	return &LevelDBBatch{
		batch: new(leveldb.Batch),
		db:    ldb.db,
	}
}

// LevelDBIterator is a LevelDB implementation of the Iterator interface
type LevelDBIterator struct {
	iter iterator.Iterator
}

// Next moves the iterator to the next key
func (it *LevelDBIterator) Next() bool {
	return it.iter.Next()
}

// Key returns the current key
func (it *LevelDBIterator) Key() []byte {
	return it.iter.Key()
}

// Value returns the current value
func (it *LevelDBIterator) Value() []byte {
	return it.iter.Value()
}

// Error returns any accumulated error
func (it *LevelDBIterator) Error() error {
	return it.iter.Error()
}

// Close releases associated resources
func (it *LevelDBIterator) Close() error {
	it.iter.Release()
	return nil
}

// LevelDBBatch is a LevelDB implementation of the Batch interface
type LevelDBBatch struct {
	batch *leveldb.Batch
	db    *leveldb.DB
}

// Put adds a key-value pair to the batch
func (b *LevelDBBatch) Put(key, value []byte) error {
	b.batch.Put(key, value)
	return nil
}

// Delete adds a key deletion to the batch
func (b *LevelDBBatch) Delete(key []byte) error {
	b.batch.Delete(key)
	return nil
}

// Write writes the batch to the database
func (b *LevelDBBatch) Write() error {
	return b.db.Write(b.batch, &opt.WriteOptions{Sync: true})
}

// Reset resets the batch
func (b *LevelDBBatch) Reset() {
	b.batch.Reset()
}