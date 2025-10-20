package state

import (
	"bytes"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/torcnet/torcnet/pkg/crypto"
	"github.com/torcnet/torcnet/pkg/db"
)

// NodeType represents the type of trie node
type NodeType uint8

const (
	NodeTypeEmpty NodeType = iota
	NodeTypeLeaf
	NodeTypeBranch
	NodeTypeExtension
)

// TrieNode represents a node in the Merkle Patricia Trie
type TrieNode struct {
	NodeType NodeType
	Key      []byte
	Value    []byte
	Hash     []byte
	Children [16]*TrieNode // For branch nodes
	Dirty    bool
	RefCount int64
	mu       sync.RWMutex
}

// MerklePatriciaTrie represents an advanced Merkle Patricia Trie with parallel execution support
type MerklePatriciaTrie struct {
	root    *TrieNode
	db      db.Database
	cache   *TrieCache
	version uint64
	mu      sync.RWMutex

	// Parallel execution support
	workers    int
	jobQueue   chan *TrieJob
	resultChan chan *TrieResult
	stopChan   chan struct{}
	wg         sync.WaitGroup

	// Performance metrics
	stats *TrieStats
}

// TrieCache implements a high-performance LRU cache for trie nodes
type TrieCache struct {
	nodes   map[string]*TrieNode
	lru     *LRUList
	maxSize int
	size    int64
	mu      sync.RWMutex
}

// LRUList implements a doubly-linked list for LRU cache
type LRUList struct {
	head *LRUNode
	tail *LRUNode
	size int
}

// LRUNode represents a node in the LRU list
type LRUNode struct {
	key  string
	node *TrieNode
	prev *LRUNode
	next *LRUNode
}

// TrieJob represents a job for parallel trie operations
type TrieJob struct {
	Operation string
	Key       []byte
	Value     []byte
	ResultCh  chan *TrieResult
}

// TrieResult represents the result of a trie operation
type TrieResult struct {
	Value []byte
	Hash  []byte
	Error error
}

// TrieStats tracks performance metrics
type TrieStats struct {
	Reads       int64
	Writes      int64
	CacheHits   int64
	CacheMisses int64
	Commits     int64
}

// NewMerklePatriciaTrie creates a new advanced Merkle Patricia Trie
func NewMerklePatriciaTrie(database db.Database, workers int, cacheSize int) *MerklePatriciaTrie {
	trie := &MerklePatriciaTrie{
		root:       &TrieNode{NodeType: NodeTypeEmpty},
		db:         database,
		cache:      NewTrieCache(cacheSize),
		version:    0,
		workers:    workers,
		jobQueue:   make(chan *TrieJob, workers*2),
		resultChan: make(chan *TrieResult, workers*2),
		stopChan:   make(chan struct{}),
		stats:      &TrieStats{},
	}

	// Start worker goroutines for parallel execution
	for i := 0; i < workers; i++ {
		trie.wg.Add(1)
		go trie.worker(i)
	}

	return trie
}

// NewTrieCache creates a new trie cache
func NewTrieCache(maxSize int) *TrieCache {
	return &TrieCache{
		nodes:   make(map[string]*TrieNode),
		lru:     NewLRUList(),
		maxSize: maxSize,
	}
}

// NewLRUList creates a new LRU list
func NewLRUList() *LRUList {
	head := &LRUNode{}
	tail := &LRUNode{}
	head.next = tail
	tail.prev = head

	return &LRUList{
		head: head,
		tail: tail,
	}
}

// Get retrieves a value from the trie with parallel execution support
func (t *MerklePatriciaTrie) Get(key []byte) ([]byte, error) {
	atomic.AddInt64(&t.stats.Reads, 1)

	// Check cache first
	if value := t.cache.Get(string(key)); value != nil {
		atomic.AddInt64(&t.stats.CacheHits, 1)
		return value.Value, nil
	}

	atomic.AddInt64(&t.stats.CacheMisses, 1)

	// Perform trie traversal
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.get(t.root, key, 0)
}

// Put stores a value in the trie with parallel execution support
func (t *MerklePatriciaTrie) Put(key, value []byte) error {
	atomic.AddInt64(&t.stats.Writes, 1)

	t.mu.Lock()
	defer t.mu.Unlock()

	newRoot, err := t.put(t.root, key, value, 0)
	if err != nil {
		return err
	}

	t.root = newRoot
	t.version++

	// Update cache
	t.cache.Put(string(key), &TrieNode{Value: value})

	return nil
}

// PutBatch performs batch operations with parallel execution
func (t *MerklePatriciaTrie) PutBatch(operations map[string][]byte) error {
	if len(operations) == 0 {
		return nil
	}

	// Split operations into chunks for parallel processing
	chunkSize := len(operations) / t.workers
	if chunkSize == 0 {
		chunkSize = 1
	}

	var wg sync.WaitGroup
	errChan := make(chan error, t.workers)

	i := 0
	chunk := make(map[string][]byte)

	for key, value := range operations {
		chunk[key] = value
		i++

		if i%chunkSize == 0 || i == len(operations) {
			wg.Add(1)
			go func(ops map[string][]byte) {
				defer wg.Done()
				for k, v := range ops {
					if err := t.Put([]byte(k), v); err != nil {
						errChan <- err
						return
					}
				}
			}(chunk)
			chunk = make(map[string][]byte)
		}
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		if err != nil {
			return err
		}
	}

	return nil
}

// Delete removes a key from the trie
func (t *MerklePatriciaTrie) Delete(key []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	newRoot, err := t.delete(t.root, key, 0)
	if err != nil {
		return err
	}

	t.root = newRoot
	t.version++

	// Remove from cache
	t.cache.Delete(string(key))

	return nil
}

// Hash returns the root hash of the trie
func (t *MerklePatriciaTrie) Hash() []byte {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.hash(t.root)
}

// Commit commits the trie to the database
func (t *MerklePatriciaTrie) Commit() error {
	atomic.AddInt64(&t.stats.Commits, 1)

	t.mu.Lock()
	defer t.mu.Unlock()

	return t.commit(t.root)
}

// CreateSnapshot creates a snapshot of the current trie state
func (t *MerklePatriciaTrie) CreateSnapshot() *TrieSnapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return &TrieSnapshot{
		Root:    t.copyNode(t.root),
		Version: t.version,
		Hash:    t.Hash(),
	}
}

// RestoreSnapshot restores the trie from a snapshot
func (t *MerklePatriciaTrie) RestoreSnapshot(snapshot *TrieSnapshot) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.root = snapshot.Root
	t.version = snapshot.Version
}

// worker processes trie operations in parallel
func (t *MerklePatriciaTrie) worker(id int) {
	defer t.wg.Done()

	for {
		select {
		case job := <-t.jobQueue:
			result := &TrieResult{}

			switch job.Operation {
			case "get":
				value, err := t.Get(job.Key)
				result.Value = value
				result.Error = err
			case "put":
				err := t.Put(job.Key, job.Value)
				result.Error = err
			case "delete":
				err := t.Delete(job.Key)
				result.Error = err
			}

			job.ResultCh <- result

		case <-t.stopChan:
			return
		}
	}
}

// get performs trie traversal to retrieve a value
func (t *MerklePatriciaTrie) get(node *TrieNode, key []byte, depth int) ([]byte, error) {
	if node == nil || node.NodeType == NodeTypeEmpty {
		return nil, fmt.Errorf("key not found")
	}

	switch node.NodeType {
	case NodeTypeLeaf:
		if bytes.Equal(node.Key, key[depth:]) {
			return node.Value, nil
		}
		return nil, fmt.Errorf("key not found")

	case NodeTypeExtension:
		if bytes.HasPrefix(key[depth:], node.Key) {
			return t.get(node.Children[0], key, depth+len(node.Key))
		}
		return nil, fmt.Errorf("key not found")

	case NodeTypeBranch:
		if depth >= len(key) {
			return node.Value, nil
		}

		nibble := key[depth]
		if nibble < 16 {
			return t.get(node.Children[nibble], key, depth+1)
		}
		return nil, fmt.Errorf("invalid key")
	}

	return nil, fmt.Errorf("unknown node type")
}

// put inserts or updates a value in the trie
func (t *MerklePatriciaTrie) put(node *TrieNode, key, value []byte, depth int) (*TrieNode, error) {
	if node == nil || node.NodeType == NodeTypeEmpty {
		// Create new leaf node
		return &TrieNode{
			NodeType: NodeTypeLeaf,
			Key:      key[depth:],
			Value:    value,
			Dirty:    true,
		}, nil
	}

	switch node.NodeType {
	case NodeTypeLeaf:
		return t.putLeaf(node, key, value, depth)

	case NodeTypeExtension:
		return t.putExtension(node, key, value, depth)

	case NodeTypeBranch:
		return t.putBranch(node, key, value, depth)
	}

	return nil, fmt.Errorf("unknown node type")
}

// putLeaf handles insertion into a leaf node
func (t *MerklePatriciaTrie) putLeaf(node *TrieNode, key, value []byte, depth int) (*TrieNode, error) {
	existingKey := key[depth:]

	if bytes.Equal(node.Key, existingKey) {
		// Update existing leaf
		newNode := t.copyNode(node)
		newNode.Value = value
		newNode.Dirty = true
		return newNode, nil
	}

	// Split the leaf node
	commonPrefix := t.commonPrefix(node.Key, existingKey)

	if len(commonPrefix) == 0 {
		// No common prefix, create branch node
		branch := &TrieNode{
			NodeType: NodeTypeBranch,
			Dirty:    true,
		}

		// Add existing leaf
		if len(node.Key) > 0 {
			branch.Children[node.Key[0]] = &TrieNode{
				NodeType: NodeTypeLeaf,
				Key:      node.Key[1:],
				Value:    node.Value,
				Dirty:    true,
			}
		} else {
			branch.Value = node.Value
		}

		// Add new leaf
		if len(existingKey) > 0 {
			branch.Children[existingKey[0]] = &TrieNode{
				NodeType: NodeTypeLeaf,
				Key:      existingKey[1:],
				Value:    value,
				Dirty:    true,
			}
		} else {
			branch.Value = value
		}

		return branch, nil
	}

	// Create extension node with common prefix
	extension := &TrieNode{
		NodeType: NodeTypeExtension,
		Key:      commonPrefix,
		Dirty:    true,
	}

	// Create branch node for the remaining parts
	branch := &TrieNode{
		NodeType: NodeTypeBranch,
		Dirty:    true,
	}

	// Add existing leaf remainder
	oldRemainder := node.Key[len(commonPrefix):]
	if len(oldRemainder) > 0 {
		branch.Children[oldRemainder[0]] = &TrieNode{
			NodeType: NodeTypeLeaf,
			Key:      oldRemainder[1:],
			Value:    node.Value,
			Dirty:    true,
		}
	} else {
		branch.Value = node.Value
	}

	// Add new leaf remainder
	newRemainder := existingKey[len(commonPrefix):]
	if len(newRemainder) > 0 {
		branch.Children[newRemainder[0]] = &TrieNode{
			NodeType: NodeTypeLeaf,
			Key:      newRemainder[1:],
			Value:    value,
			Dirty:    true,
		}
	} else {
		branch.Value = value
	}

	extension.Children[0] = branch
	return extension, nil
}

// putExtension handles insertion into an extension node
func (t *MerklePatriciaTrie) putExtension(node *TrieNode, key, value []byte, depth int) (*TrieNode, error) {
	keyRemainder := key[depth:]
	commonPrefix := t.commonPrefix(node.Key, keyRemainder)

	if len(commonPrefix) == len(node.Key) {
		// Full match, continue with child
		newChild, err := t.put(node.Children[0], key, value, depth+len(node.Key))
		if err != nil {
			return nil, err
		}

		newNode := t.copyNode(node)
		newNode.Children[0] = newChild
		newNode.Dirty = true
		return newNode, nil
	}

	// Partial match, need to split
	newExtension := &TrieNode{
		NodeType: NodeTypeExtension,
		Key:      commonPrefix,
		Dirty:    true,
	}

	branch := &TrieNode{
		NodeType: NodeTypeBranch,
		Dirty:    true,
	}

	// Add old extension remainder
	oldRemainder := node.Key[len(commonPrefix):]
	if len(oldRemainder) == 1 {
		branch.Children[oldRemainder[0]] = node.Children[0]
	} else {
		branch.Children[oldRemainder[0]] = &TrieNode{
			NodeType: NodeTypeExtension,
			Key:      oldRemainder[1:],
			Children: [16]*TrieNode{node.Children[0]},
			Dirty:    true,
		}
	}

	// Add new key remainder
	newRemainder := keyRemainder[len(commonPrefix):]
	if len(newRemainder) > 0 {
		branch.Children[newRemainder[0]] = &TrieNode{
			NodeType: NodeTypeLeaf,
			Key:      newRemainder[1:],
			Value:    value,
			Dirty:    true,
		}
	} else {
		branch.Value = value
	}

	newExtension.Children[0] = branch
	return newExtension, nil
}

// putBranch handles insertion into a branch node
func (t *MerklePatriciaTrie) putBranch(node *TrieNode, key, value []byte, depth int) (*TrieNode, error) {
	newNode := t.copyNode(node)

	if depth >= len(key) {
		// Set value at this branch
		newNode.Value = value
		newNode.Dirty = true
		return newNode, nil
	}

	nibble := key[depth]
	if nibble >= 16 {
		return nil, fmt.Errorf("invalid key")
	}

	child, err := t.put(node.Children[nibble], key, value, depth+1)
	if err != nil {
		return nil, err
	}

	newNode.Children[nibble] = child
	newNode.Dirty = true
	return newNode, nil
}

// delete removes a key from the trie
func (t *MerklePatriciaTrie) delete(node *TrieNode, key []byte, depth int) (*TrieNode, error) {
	if node == nil || node.NodeType == NodeTypeEmpty {
		return node, nil // Key not found, nothing to delete
	}

	switch node.NodeType {
	case NodeTypeLeaf:
		if bytes.Equal(node.Key, key[depth:]) {
			return &TrieNode{NodeType: NodeTypeEmpty}, nil
		}
		return node, nil

	case NodeTypeExtension:
		if bytes.HasPrefix(key[depth:], node.Key) {
			newChild, err := t.delete(node.Children[0], key, depth+len(node.Key))
			if err != nil {
				return nil, err
			}

			if newChild.NodeType == NodeTypeEmpty {
				return &TrieNode{NodeType: NodeTypeEmpty}, nil
			}

			newNode := t.copyNode(node)
			newNode.Children[0] = newChild
			newNode.Dirty = true
			return newNode, nil
		}
		return node, nil

	case NodeTypeBranch:
		if depth >= len(key) {
			// Delete value at this branch
			newNode := t.copyNode(node)
			newNode.Value = nil
			newNode.Dirty = true
			return t.normalizeBranch(newNode), nil
		}

		nibble := key[depth]
		if nibble >= 16 {
			return node, nil
		}

		newChild, err := t.delete(node.Children[nibble], key, depth+1)
		if err != nil {
			return nil, err
		}

		newNode := t.copyNode(node)
		newNode.Children[nibble] = newChild
		newNode.Dirty = true
		return t.normalizeBranch(newNode), nil
	}

	return node, nil
}

// normalizeBranch optimizes branch nodes after deletion
func (t *MerklePatriciaTrie) normalizeBranch(node *TrieNode) *TrieNode {
	if node.NodeType != NodeTypeBranch {
		return node
	}

	childCount := 0
	var lastChild *TrieNode
	var lastIndex int

	for i, child := range node.Children {
		if child != nil && child.NodeType != NodeTypeEmpty {
			childCount++
			lastChild = child
			lastIndex = i
		}
	}

	// If only one child and no value, convert to extension or return child
	if childCount == 1 && node.Value == nil {
		if lastChild.NodeType == NodeTypeLeaf {
			return &TrieNode{
				NodeType: NodeTypeLeaf,
				Key:      append([]byte{byte(lastIndex)}, lastChild.Key...),
				Value:    lastChild.Value,
				Dirty:    true,
			}
		} else if lastChild.NodeType == NodeTypeExtension {
			return &TrieNode{
				NodeType: NodeTypeExtension,
				Key:      append([]byte{byte(lastIndex)}, lastChild.Key...),
				Children: lastChild.Children,
				Dirty:    true,
			}
		}
	}

	// If no children and no value, return empty node
	if childCount == 0 && node.Value == nil {
		return &TrieNode{NodeType: NodeTypeEmpty}
	}

	return node
}

// hash calculates the hash of a node
func (t *MerklePatriciaTrie) hash(node *TrieNode) []byte {
	if node == nil || node.NodeType == NodeTypeEmpty {
		return crypto.Hash([]byte{})
	}

	if node.Hash != nil && !node.Dirty {
		return node.Hash
	}

	var data []byte

	switch node.NodeType {
	case NodeTypeLeaf:
		data = append(data, byte(NodeTypeLeaf))
		data = append(data, node.Key...)
		data = append(data, node.Value...)

	case NodeTypeExtension:
		data = append(data, byte(NodeTypeExtension))
		data = append(data, node.Key...)
		data = append(data, t.hash(node.Children[0])...)

	case NodeTypeBranch:
		data = append(data, byte(NodeTypeBranch))
		for _, child := range node.Children {
			data = append(data, t.hash(child)...)
		}
		if node.Value != nil {
			data = append(data, node.Value...)
		}
	}

	hash := crypto.Hash(data)
	node.Hash = hash
	node.Dirty = false

	return hash
}

// commit saves the trie to the database
func (t *MerklePatriciaTrie) commit(node *TrieNode) error {
	if node == nil || node.NodeType == NodeTypeEmpty || !node.Dirty {
		return nil
	}

	// Commit children first
	for _, child := range node.Children {
		if err := t.commit(child); err != nil {
			return err
		}
	}

	// Serialize and save node
	data := t.serializeNode(node)
	hash := t.hash(node)

	if err := t.db.Put(hash, data); err != nil {
		return err
	}

	node.Dirty = false
	return nil
}

// serializeNode serializes a trie node for storage
func (t *MerklePatriciaTrie) serializeNode(node *TrieNode) []byte {
	// This is a simplified serialization
	// In production, use more efficient encoding like RLP
	var data []byte
	data = append(data, byte(node.NodeType))

	switch node.NodeType {
	case NodeTypeLeaf:
		data = append(data, byte(len(node.Key)))
		data = append(data, node.Key...)
		data = append(data, byte(len(node.Value)))
		data = append(data, node.Value...)

	case NodeTypeExtension:
		data = append(data, byte(len(node.Key)))
		data = append(data, node.Key...)
		data = append(data, t.hash(node.Children[0])...)

	case NodeTypeBranch:
		for _, child := range node.Children {
			if child != nil && child.NodeType != NodeTypeEmpty {
				data = append(data, t.hash(child)...)
			} else {
				data = append(data, make([]byte, 32)...) // Empty hash
			}
		}
		if node.Value != nil {
			data = append(data, byte(len(node.Value)))
			data = append(data, node.Value...)
		}
	}

	return data
}

// copyNode creates a deep copy of a trie node
func (t *MerklePatriciaTrie) copyNode(node *TrieNode) *TrieNode {
	if node == nil {
		return nil
	}

	newNode := &TrieNode{
		NodeType: node.NodeType,
		Key:      make([]byte, len(node.Key)),
		Value:    make([]byte, len(node.Value)),
		Hash:     make([]byte, len(node.Hash)),
		Dirty:    node.Dirty,
		RefCount: node.RefCount,
	}

	copy(newNode.Key, node.Key)
	copy(newNode.Value, node.Value)
	copy(newNode.Hash, node.Hash)

	for i, child := range node.Children {
		newNode.Children[i] = child
	}

	return newNode
}

// commonPrefix finds the common prefix between two byte slices
func (t *MerklePatriciaTrie) commonPrefix(a, b []byte) []byte {
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}

	for i := 0; i < minLen; i++ {
		if a[i] != b[i] {
			return a[:i]
		}
	}

	return a[:minLen]
}

// Cache operations
func (c *TrieCache) Get(key string) *TrieNode {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if node, exists := c.nodes[key]; exists {
		c.lru.MoveToFront(key)
		return node
	}

	return nil
}

func (c *TrieCache) Put(key string, node *TrieNode) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.nodes[key]; exists {
		c.nodes[key] = node
		c.lru.MoveToFront(key)
		return
	}

	if c.size >= int64(c.maxSize) {
		// Evict least recently used
		lru := c.lru.RemoveLast()
		if lru != nil {
			delete(c.nodes, lru.key)
			c.size--
		}
	}

	c.nodes[key] = node
	c.lru.AddToFront(key, node)
	c.size++
}

func (c *TrieCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.nodes[key]; exists {
		delete(c.nodes, key)
		c.lru.Remove(key)
		c.size--
	}
}

// LRU List operations
func (l *LRUList) AddToFront(key string, node *TrieNode) {
	newNode := &LRUNode{
		key:  key,
		node: node,
		next: l.head.next,
		prev: l.head,
	}

	l.head.next.prev = newNode
	l.head.next = newNode
	l.size++
}

func (l *LRUList) MoveToFront(key string) {
	// Find and move node to front
	current := l.head.next
	for current != l.tail {
		if current.key == key {
			// Remove from current position
			current.prev.next = current.next
			current.next.prev = current.prev

			// Add to front
			current.next = l.head.next
			current.prev = l.head
			l.head.next.prev = current
			l.head.next = current
			break
		}
		current = current.next
	}
}

func (l *LRUList) Remove(key string) {
	current := l.head.next
	for current != l.tail {
		if current.key == key {
			current.prev.next = current.next
			current.next.prev = current.prev
			l.size--
			break
		}
		current = current.next
	}
}

func (l *LRUList) RemoveLast() *LRUNode {
	if l.size == 0 {
		return nil
	}

	last := l.tail.prev
	last.prev.next = l.tail
	l.tail.prev = last.prev
	l.size--

	return last
}

// TrieSnapshot represents a snapshot of the trie state
type TrieSnapshot struct {
	Root    *TrieNode
	Version uint64
	Hash    []byte
}

// GetStats returns performance statistics
func (t *MerklePatriciaTrie) GetStats() *TrieStats {
	return &TrieStats{
		Reads:       atomic.LoadInt64(&t.stats.Reads),
		Writes:      atomic.LoadInt64(&t.stats.Writes),
		CacheHits:   atomic.LoadInt64(&t.stats.CacheHits),
		CacheMisses: atomic.LoadInt64(&t.stats.CacheMisses),
		Commits:     atomic.LoadInt64(&t.stats.Commits),
	}
}

// Stop gracefully shuts down the trie workers
func (t *MerklePatriciaTrie) Stop() {
	close(t.stopChan)
	t.wg.Wait()
}
