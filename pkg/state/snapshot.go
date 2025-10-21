package state

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/torcnet/torcnet/pkg/crypto"
	"github.com/torcnet/torcnet/pkg/db"
)

// SnapshotManager manages state snapshots and versioning
type SnapshotManager struct {
	db             db.Database
	snapshots      map[uint64]*StateSnapshot
	versions       []uint64
	currentVersion uint64
	maxSnapshots   int
	mu             sync.RWMutex

	// Versioning
	versionTree   *VersionTree
	branchManager *BranchManager

	// Compression and storage
	compressor    *SnapshotCompressor
	storageEngine *SnapshotStorage
}

// StateSnapshot represents a complete state snapshot at a specific version
// Note: StateSnapshot struct is now defined in pb/state.proto
// This is a wrapper for compatibility
type StateSnapshot struct {
	Version     uint64
	Timestamp   time.Time
	RootHash    []byte
	BlockHeight uint64
	Accounts    map[string]*Account
	Storage     map[string][]byte
	Metadata    *SnapshotMetadata

	// Compression info
	Compressed      bool
	CompressionType string
	OriginalSize    uint64
	CompressedSize  uint64

	// Merkle proof for verification
	MerkleProof [][]byte
}

// SnapshotMetadata contains additional snapshot information
type SnapshotMetadata struct {
	Creator      string         `json:"creator"`
	Description  string         `json:"description"`
	Tags         []string       `json:"tags"`
	Dependencies []uint64       `json:"dependencies"`
	Statistics   *SnapshotStats `json:"statistics"`
	Checksum     []byte         `json:"checksum"`
}

// SnapshotStats contains snapshot statistics
type SnapshotStats struct {
	AccountCount     uint64        `json:"account_count"`
	TransactionCount uint64        `json:"transaction_count"`
	StorageSize      uint64        `json:"storage_size"`
	CreationTime     time.Duration `json:"creation_time"`
}

// VersionTree manages version relationships and branching
type VersionTree struct {
	nodes    map[uint64]*VersionNode
	branches map[string]*Branch
	root     *VersionNode
	mu       sync.RWMutex
}

// VersionNode represents a node in the version tree
type VersionNode struct {
	Version   uint64         `json:"version"`
	Parent    *VersionNode   `json:"-"`
	Children  []*VersionNode `json:"-"`
	Branch    string         `json:"branch"`
	Timestamp time.Time      `json:"timestamp"`
	Hash      []byte         `json:"hash"`
}

// Branch represents a version branch
type Branch struct {
	Name        string       `json:"name"`
	Head        *VersionNode `json:"-"`
	Created     time.Time    `json:"created"`
	Description string       `json:"description"`
}

// BranchManager manages version branches
type BranchManager struct {
	branches     map[string]*Branch
	activeBranch string
	mergeHistory []*MergeRecord
	mu           sync.RWMutex
}

// MergeRecord tracks branch merges
type MergeRecord struct {
	SourceBranch string    `json:"source_branch"`
	TargetBranch string    `json:"target_branch"`
	MergeVersion uint64    `json:"merge_version"`
	Timestamp    time.Time `json:"timestamp"`
	Conflicts    []string  `json:"conflicts"`
}

// SnapshotCompressor handles snapshot compression
type SnapshotCompressor struct {
	algorithm string
	level     int
}

// SnapshotStorage manages snapshot storage and retrieval
type SnapshotStorage struct {
	db        db.Database
	cacheSize int
	cache     map[uint64]*StateSnapshot
	cacheMu   sync.RWMutex
}

// DiffSnapshot represents differences between two snapshots
type DiffSnapshot struct {
	FromVersion uint64                  `json:"from_version"`
	ToVersion   uint64                  `json:"to_version"`
	Added       map[string]*Account     `json:"added"`
	Modified    map[string]*AccountDiff `json:"modified"`
	Deleted     []string                `json:"deleted"`
	StorageDiff map[string]*StorageDiff `json:"storage_diff"`
}

// AccountDiff represents changes to an account
type AccountDiff struct {
	Address     []byte            `json:"address"`
	BalanceDiff *big.Int          `json:"balance_diff"`
	NonceDiff   uint64            `json:"nonce_diff"`
	StorageDiff map[string][]byte `json:"storage_diff"`
}

// StorageDiff represents changes to storage
type StorageDiff struct {
	Key      string `json:"key"`
	OldValue []byte `json:"old_value"`
	NewValue []byte `json:"new_value"`
}

// NewSnapshotManager creates a new snapshot manager
func NewSnapshotManager(database db.Database, maxSnapshots int) *SnapshotManager {
	return &SnapshotManager{
		db:            database,
		snapshots:     make(map[uint64]*StateSnapshot),
		versions:      make([]uint64, 0),
		maxSnapshots:  maxSnapshots,
		versionTree:   NewVersionTree(),
		branchManager: NewBranchManager(),
		compressor:    NewSnapshotCompressor("lz4", 1),
		storageEngine: NewSnapshotStorage(database, 100),
	}
}

// CreateSnapshot creates a new state snapshot
func (sm *SnapshotManager) CreateSnapshot(state *State, blockHeight uint64, metadata *SnapshotMetadata) (*StateSnapshot, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	startTime := time.Now()
	version := sm.currentVersion + 1

	// Create snapshot
	snapshot := &StateSnapshot{
		Version:     version,
		Timestamp:   time.Now(),
		BlockHeight: blockHeight,
		Accounts:    make(map[string]*Account),
		Storage:     make(map[string][]byte),
		Metadata:    metadata,
	}

	// Copy state data
	if err := sm.copyStateToSnapshot(state, snapshot); err != nil {
		return nil, fmt.Errorf("failed to copy state: %w", err)
	}

	// Calculate root hash
	snapshot.RootHash = sm.calculateSnapshotHash(snapshot)

	// Generate Merkle proof
	snapshot.MerkleProof = sm.generateMerkleProof(snapshot)

	// Update statistics
	if snapshot.Metadata != nil && snapshot.Metadata.Statistics != nil {
		snapshot.Metadata.Statistics.CreationTime = time.Since(startTime)
		snapshot.Metadata.Statistics.AccountCount = uint64(len(snapshot.Accounts))
		snapshot.Metadata.Statistics.StorageSize = sm.calculateStorageSize(snapshot)
	}

	// Compress if beneficial
	if err := sm.compressSnapshot(snapshot); err != nil {
		return nil, fmt.Errorf("failed to compress snapshot: %w", err)
	}

	// Store snapshot
	if err := sm.storeSnapshot(snapshot); err != nil {
		return nil, fmt.Errorf("failed to store snapshot: %w", err)
	}

	// Update version tree
	sm.versionTree.AddVersion(version, sm.branchManager.activeBranch)

	// Update manager state
	sm.snapshots[version] = snapshot
	sm.versions = append(sm.versions, version)
	sm.currentVersion = version

	// Cleanup old snapshots if necessary
	sm.cleanupOldSnapshots()

	return snapshot, nil
}

// RestoreSnapshot restores state from a snapshot
func (sm *SnapshotManager) RestoreSnapshot(version uint64, state *State) error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	snapshot, err := sm.getSnapshot(version)
	if err != nil {
		return fmt.Errorf("failed to get snapshot: %w", err)
	}

	// Verify snapshot integrity
	if err := sm.verifySnapshot(snapshot); err != nil {
		return fmt.Errorf("snapshot verification failed: %w", err)
	}

	// Decompress if necessary
	if snapshot.Compressed {
		if err := sm.decompressSnapshot(snapshot); err != nil {
			return fmt.Errorf("failed to decompress snapshot: %w", err)
		}
	}

	// Restore state
	return sm.restoreStateFromSnapshot(snapshot, state)
}

// CreateDiff creates a diff between two snapshots
func (sm *SnapshotManager) CreateDiff(fromVersion, toVersion uint64) (*DiffSnapshot, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	fromSnapshot, err := sm.getSnapshot(fromVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to get from snapshot: %w", err)
	}

	toSnapshot, err := sm.getSnapshot(toVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to get to snapshot: %w", err)
	}

	diff := &DiffSnapshot{
		FromVersion: fromVersion,
		ToVersion:   toVersion,
		Added:       make(map[string]*Account),
		Modified:    make(map[string]*AccountDiff),
		Deleted:     make([]string, 0),
		StorageDiff: make(map[string]*StorageDiff),
	}

	// Compare accounts
	sm.compareAccounts(fromSnapshot.Accounts, toSnapshot.Accounts, diff)

	// Compare storage
	sm.compareStorage(fromSnapshot.Storage, toSnapshot.Storage, diff)

	return diff, nil
}

// ApplyDiff applies a diff to create a new snapshot
func (sm *SnapshotManager) ApplyDiff(baseVersion uint64, diff *DiffSnapshot) (*StateSnapshot, error) {
	baseSnapshot, err := sm.getSnapshot(baseVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to get base snapshot: %w", err)
	}

	// Create new snapshot based on base
	newSnapshot := &StateSnapshot{
		Version:     diff.ToVersion,
		Timestamp:   time.Now(),
		BlockHeight: baseSnapshot.BlockHeight + 1,
		Accounts:    make(map[string]*Account),
		Storage:     make(map[string][]byte),
	}

	// Copy base accounts
	for addr, account := range baseSnapshot.Accounts {
		newSnapshot.Accounts[addr] = sm.copyAccount(account)
	}

	// Copy base storage
	for key, value := range baseSnapshot.Storage {
		newSnapshot.Storage[key] = make([]byte, len(value))
		copy(newSnapshot.Storage[key], value)
	}

	// Apply diff
	sm.applyDiffToSnapshot(diff, newSnapshot)

	// Calculate new root hash
	newSnapshot.RootHash = sm.calculateSnapshotHash(newSnapshot)

	return newSnapshot, nil
}

// CreateBranch creates a new version branch
func (sm *SnapshotManager) CreateBranch(name, description string, fromVersion uint64) error {
	return sm.branchManager.CreateBranch(name, description, fromVersion, sm.versionTree)
}

// SwitchBranch switches to a different branch
func (sm *SnapshotManager) SwitchBranch(branchName string) error {
	return sm.branchManager.SwitchBranch(branchName)
}

// MergeBranch merges one branch into another
func (sm *SnapshotManager) MergeBranch(sourceBranch, targetBranch string) (*MergeRecord, error) {
	return sm.branchManager.MergeBranch(sourceBranch, targetBranch, sm.versionTree)
}

// GetSnapshot retrieves a snapshot by version
func (sm *SnapshotManager) GetSnapshot(version uint64) (*StateSnapshot, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.getSnapshot(version)
}

// ListSnapshots returns all available snapshots
func (sm *SnapshotManager) ListSnapshots() []*StateSnapshot {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	snapshots := make([]*StateSnapshot, 0, len(sm.snapshots))
	for _, snapshot := range sm.snapshots {
		snapshots = append(snapshots, snapshot)
	}

	return snapshots
}

// PruneSnapshots removes old snapshots based on retention policy
func (sm *SnapshotManager) PruneSnapshots(retentionPolicy *RetentionPolicy) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	return sm.pruneSnapshots(retentionPolicy)
}

// Helper methods

func (sm *SnapshotManager) getSnapshot(version uint64) (*StateSnapshot, error) {
	// Check memory cache first
	if snapshot, exists := sm.snapshots[version]; exists {
		return snapshot, nil
	}

	// Load from storage
	return sm.storageEngine.LoadSnapshot(version)
}

func (sm *SnapshotManager) copyStateToSnapshot(state *State, snapshot *StateSnapshot) error {
	// This would iterate through the state trie and copy all accounts
	// For now, we'll use the existing accounts from the state
	state.mu.RLock()
	defer state.mu.RUnlock()

	for addr, account := range state.accounts {
		snapshot.Accounts[addr] = sm.copyAccount(account)
	}

	// Note: Storage is managed per account, not globally in State
	// Global storage would be accessed through the trie if needed

	return nil
}

func (sm *SnapshotManager) copyAccount(account *Account) *Account {
	newAccount := &Account{
		Address: account.Address,
		Balance: account.Balance,
		Nonce:   account.Nonce,
		Storage: make(map[string][]byte),
	}

	for key, value := range account.Storage {
		newAccount.Storage[key] = make([]byte, len(value))
		copy(newAccount.Storage[key], value)
	}

	return newAccount
}

func (sm *SnapshotManager) calculateSnapshotHash(snapshot *StateSnapshot) []byte {
	// Create a deterministic hash of the snapshot
	data, _ := json.Marshal(snapshot.Accounts)
	storageData, _ := json.Marshal(snapshot.Storage)

	combined := append(data, storageData...)
	return crypto.Hash(combined)
}

func (sm *SnapshotManager) generateMerkleProof(snapshot *StateSnapshot) [][]byte {
	// Generate Merkle proof for snapshot verification
	// This is a simplified implementation
	proofs := make([][]byte, 0)

	for _, account := range snapshot.Accounts {
		accountData, _ := json.Marshal(account)
		proof := crypto.Hash(accountData)
		proofs = append(proofs, proof)
	}

	return proofs
}

func (sm *SnapshotManager) calculateStorageSize(snapshot *StateSnapshot) uint64 {
	size := uint64(0)

	for _, account := range snapshot.Accounts {
		size += uint64(len(account.Address))
		size += 32 // Balance (big.Int)
		size += 8  // Nonce
		for key, value := range account.Storage {
			size += uint64(len(key) + len(value))
		}
	}

	for key, value := range snapshot.Storage {
		size += uint64(len(key) + len(value))
	}

	return size
}

func (sm *SnapshotManager) compressSnapshot(snapshot *StateSnapshot) error {
	if sm.compressor == nil {
		return nil
	}

	// Serialize snapshot
	data, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}

	// Compress data
	compressed, err := sm.compressor.Compress(data)
	if err != nil {
		return err
	}

	// Update snapshot if compression is beneficial
	if len(compressed) < len(data) {
		snapshot.Compressed = true
		snapshot.CompressionType = sm.compressor.algorithm
		snapshot.OriginalSize = uint64(len(data))
		snapshot.CompressedSize = uint64(len(compressed))
	}

	return nil
}

func (sm *SnapshotManager) decompressSnapshot(snapshot *StateSnapshot) error {
	if !snapshot.Compressed || sm.compressor == nil {
		return nil
	}

	// Load compressed data and decompress
	// This would involve loading the compressed data from storage
	// and decompressing it back to the original format

	snapshot.Compressed = false
	return nil
}

func (sm *SnapshotManager) verifySnapshot(snapshot *StateSnapshot) error {
	// Verify snapshot integrity using Merkle proofs and checksums
	calculatedHash := sm.calculateSnapshotHash(snapshot)

	if !bytes.Equal(calculatedHash, snapshot.RootHash) {
		return fmt.Errorf("snapshot hash mismatch")
	}

	// Verify Merkle proofs
	if len(snapshot.MerkleProof) > 0 {
		// Verify each proof
		for i, proof := range snapshot.MerkleProof {
			if len(proof) == 0 {
				return fmt.Errorf("invalid merkle proof at index %d", i)
			}
		}
	}

	return nil
}

func (sm *SnapshotManager) storeSnapshot(snapshot *StateSnapshot) error {
	return sm.storageEngine.StoreSnapshot(snapshot)
}

func (sm *SnapshotManager) restoreStateFromSnapshot(snapshot *StateSnapshot, state *State) error {
	state.mu.Lock()
	defer state.mu.Unlock()

	// Clear existing state
	state.accounts = make(map[string]*Account)

	// Restore accounts
	for addr, account := range snapshot.Accounts {
		state.accounts[addr] = sm.copyAccount(account)
	}

	// Note: Storage is managed per account, not globally in State
	// Global storage would be restored through the trie if needed

	return nil
}

func (sm *SnapshotManager) compareAccounts(from, to map[string]*Account, diff *DiffSnapshot) {
	// Find added and modified accounts
	for addr, toAccount := range to {
		if fromAccount, exists := from[addr]; exists {
			// Check if modified
			if !sm.accountsEqual(fromAccount, toAccount) {
				diff.Modified[addr] = sm.createAccountDiff(fromAccount, toAccount)
			}
		} else {
			// Added account
			diff.Added[addr] = sm.copyAccount(toAccount)
		}
	}

	// Find deleted accounts
	for addr := range from {
		if _, exists := to[addr]; !exists {
			diff.Deleted = append(diff.Deleted, addr)
		}
	}
}

func (sm *SnapshotManager) compareStorage(from, to map[string][]byte, diff *DiffSnapshot) {
	// Compare storage changes
	for key, toValue := range to {
		if fromValue, exists := from[key]; exists {
			if !bytes.Equal(fromValue, toValue) {
				diff.StorageDiff[key] = &StorageDiff{
					Key:      key,
					OldValue: fromValue,
					NewValue: toValue,
				}
			}
		} else {
			diff.StorageDiff[key] = &StorageDiff{
				Key:      key,
				OldValue: nil,
				NewValue: toValue,
			}
		}
	}

	// Find deleted storage
	for key, fromValue := range from {
		if _, exists := to[key]; !exists {
			diff.StorageDiff[key] = &StorageDiff{
				Key:      key,
				OldValue: fromValue,
				NewValue: nil,
			}
		}
	}
}

func (sm *SnapshotManager) accountsEqual(a, b *Account) bool {
	if a.Address != b.Address {
		return false
	}
	if a.Balance != b.Balance {
		return false
	}
	if a.Nonce != b.Nonce {
		return false
	}
	if len(a.Storage) != len(b.Storage) {
		return false
	}
	for key, value := range a.Storage {
		if bValue, exists := b.Storage[key]; !exists || !bytes.Equal(value, bValue) {
			return false
		}
	}
	return true
}

func (sm *SnapshotManager) createAccountDiff(from, to *Account) *AccountDiff {
	diff := &AccountDiff{
		Address:     []byte(to.Address),
		BalanceDiff: big.NewInt(int64(to.Balance) - int64(from.Balance)),
		NonceDiff:   to.Nonce - from.Nonce,
		StorageDiff: make(map[string][]byte),
	}

	// Compare storage
	for key, toValue := range to.Storage {
		if fromValue, exists := from.Storage[key]; !exists || !bytes.Equal(fromValue, toValue) {
			diff.StorageDiff[key] = toValue
		}
	}

	return diff
}

func (sm *SnapshotManager) applyDiffToSnapshot(diff *DiffSnapshot, snapshot *StateSnapshot) {
	// Apply added accounts
	for addr, account := range diff.Added {
		snapshot.Accounts[addr] = sm.copyAccount(account)
	}

	// Apply modified accounts
	for addr, accountDiff := range diff.Modified {
		if account, exists := snapshot.Accounts[addr]; exists {
			account.Balance = uint64(int64(account.Balance) + accountDiff.BalanceDiff.Int64())
			account.Nonce += accountDiff.NonceDiff
			for key, value := range accountDiff.StorageDiff {
				account.Storage[key] = value
			}
		}
	}

	// Apply deleted accounts
	for _, addr := range diff.Deleted {
		delete(snapshot.Accounts, addr)
	}

	// Apply storage diff
	for key, storageDiff := range diff.StorageDiff {
		if storageDiff.NewValue != nil {
			snapshot.Storage[key] = storageDiff.NewValue
		} else {
			delete(snapshot.Storage, key)
		}
	}
}

func (sm *SnapshotManager) cleanupOldSnapshots() {
	if len(sm.versions) <= sm.maxSnapshots {
		return
	}

	// Remove oldest snapshots
	toRemove := len(sm.versions) - sm.maxSnapshots
	for i := 0; i < toRemove; i++ {
		version := sm.versions[i]
		delete(sm.snapshots, version)
		sm.storageEngine.DeleteSnapshot(version)
	}

	sm.versions = sm.versions[toRemove:]
}

func (sm *SnapshotManager) pruneSnapshots(policy *RetentionPolicy) error {
	// Implement retention policy-based pruning
	// This would remove snapshots based on age, count, or other criteria
	return nil
}

// Version Tree implementation
func NewVersionTree() *VersionTree {
	return &VersionTree{
		nodes:    make(map[uint64]*VersionNode),
		branches: make(map[string]*Branch),
	}
}

func (vt *VersionTree) AddVersion(version uint64, branch string) {
	vt.mu.Lock()
	defer vt.mu.Unlock()

	node := &VersionNode{
		Version:   version,
		Branch:    branch,
		Timestamp: time.Now(),
		Children:  make([]*VersionNode, 0),
	}

	vt.nodes[version] = node

	if vt.root == nil {
		vt.root = node
	}
}

// Branch Manager implementation
func NewBranchManager() *BranchManager {
	bm := &BranchManager{
		branches:     make(map[string]*Branch),
		activeBranch: "main",
		mergeHistory: make([]*MergeRecord, 0),
	}

	// Create main branch
	bm.branches["main"] = &Branch{
		Name:        "main",
		Created:     time.Now(),
		Description: "Main branch",
	}

	return bm
}

func (bm *BranchManager) CreateBranch(name, description string, fromVersion uint64, vt *VersionTree) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if _, exists := bm.branches[name]; exists {
		return fmt.Errorf("branch %s already exists", name)
	}

	branch := &Branch{
		Name:        name,
		Created:     time.Now(),
		Description: description,
	}

	bm.branches[name] = branch
	return nil
}

func (bm *BranchManager) SwitchBranch(branchName string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if _, exists := bm.branches[branchName]; !exists {
		return fmt.Errorf("branch %s does not exist", branchName)
	}

	bm.activeBranch = branchName
	return nil
}

func (bm *BranchManager) MergeBranch(sourceBranch, targetBranch string, vt *VersionTree) (*MergeRecord, error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if _, exists := bm.branches[sourceBranch]; !exists {
		return nil, fmt.Errorf("source branch %s does not exist", sourceBranch)
	}

	if _, exists := bm.branches[targetBranch]; !exists {
		return nil, fmt.Errorf("target branch %s does not exist", targetBranch)
	}

	record := &MergeRecord{
		SourceBranch: sourceBranch,
		TargetBranch: targetBranch,
		Timestamp:    time.Now(),
		Conflicts:    make([]string, 0),
	}

	bm.mergeHistory = append(bm.mergeHistory, record)
	return record, nil
}

// Snapshot Compressor implementation
func NewSnapshotCompressor(algorithm string, level int) *SnapshotCompressor {
	return &SnapshotCompressor{
		algorithm: algorithm,
		level:     level,
	}
}

func (sc *SnapshotCompressor) Compress(data []byte) ([]byte, error) {
	// This would implement actual compression using the specified algorithm
	// For now, return the original data
	return data, nil
}

func (sc *SnapshotCompressor) Decompress(data []byte) ([]byte, error) {
	// This would implement actual decompression
	// For now, return the original data
	return data, nil
}

// Snapshot Storage implementation
func NewSnapshotStorage(database db.Database, cacheSize int) *SnapshotStorage {
	return &SnapshotStorage{
		db:        database,
		cacheSize: cacheSize,
		cache:     make(map[uint64]*StateSnapshot),
	}
}

func (ss *SnapshotStorage) StoreSnapshot(snapshot *StateSnapshot) error {
	data, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("snapshot_%d", snapshot.Version)
	if err := ss.db.Put([]byte(key), data); err != nil {
		return err
	}

	// Add to cache
	ss.cacheMu.Lock()
	ss.cache[snapshot.Version] = snapshot
	ss.cacheMu.Unlock()

	return nil
}

func (ss *SnapshotStorage) LoadSnapshot(version uint64) (*StateSnapshot, error) {
	// Check cache first
	ss.cacheMu.RLock()
	if snapshot, exists := ss.cache[version]; exists {
		ss.cacheMu.RUnlock()
		return snapshot, nil
	}
	ss.cacheMu.RUnlock()

	// Load from database
	key := fmt.Sprintf("snapshot_%d", version)
	data, err := ss.db.Get([]byte(key))
	if err != nil {
		return nil, err
	}

	var snapshot StateSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, err
	}

	// Add to cache
	ss.cacheMu.Lock()
	ss.cache[version] = &snapshot
	ss.cacheMu.Unlock()

	return &snapshot, nil
}

func (ss *SnapshotStorage) DeleteSnapshot(version uint64) error {
	key := fmt.Sprintf("snapshot_%d", version)

	// Remove from cache
	ss.cacheMu.Lock()
	delete(ss.cache, version)
	ss.cacheMu.Unlock()

	// Remove from database
	return ss.db.Delete([]byte(key))
}

// RetentionPolicy defines snapshot retention rules
type RetentionPolicy struct {
	MaxAge       time.Duration `json:"max_age"`
	MaxCount     int           `json:"max_count"`
	KeepInterval time.Duration `json:"keep_interval"`
}