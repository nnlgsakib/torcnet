package state

import (
	"fmt"
	"runtime"
	"sync"

	"github.com/torcnet/torcnet/pb"
	"github.com/torcnet/torcnet/pkg/db"
	"google.golang.org/protobuf/proto"
)

// State represents the global state of the blockchain
type State struct {
	db        db.Database
	accounts  map[string]*Account
	mu        sync.RWMutex
	cacheSize int
	trie      *MerklePatriciaTrie
}

// Account represents an account in the state
// Note: Account struct is now defined in pb/state.proto as StateAccount
// This is a wrapper for compatibility
type Account struct {
	Address  string
	Balance  uint64
	Nonce    uint64
	CodeHash []byte
	Storage  map[string][]byte
}

// Serialize serializes the account to protobuf bytes
func (a *Account) Serialize() []byte {
	pbAccount := &pb.StateAccount{
		Address: a.Address,
		Balance: a.Balance,
		Nonce:   a.Nonce,
		Code:    a.CodeHash, // Note: protobuf uses 'Code' field instead of 'CodeHash'
		Storage: a.Storage,
	}
	data, _ := proto.Marshal(pbAccount)
	return data
}

// NewState creates a new state backed by Merkle Patricia Trie
func NewState(database db.Database, cacheSize int) *State {
	workers := runtime.NumCPU()
	trie := NewMerklePatriciaTrie(database, workers, cacheSize)
	return &State{
		db:        database,
		accounts:  make(map[string]*Account),
		cacheSize: cacheSize,
		trie:      trie,
	}
}

// GetAccount gets an account from the state
func (s *State) GetAccount(address string) (*Account, error) {
	s.mu.RLock()
	if account, ok := s.accounts[address]; ok {
		s.mu.RUnlock()
		return account, nil
	}
	s.mu.RUnlock()

	// Not in cache, get from trie
	key := []byte("account:" + address)
	data, err := s.trie.Get(key)
	if err != nil || data == nil {
		return nil, fmt.Errorf("account not found: %s", address)
	}

	var account Account
	var pbAccount pb.StateAccount
	if err := proto.Unmarshal(data, &pbAccount); err != nil {
		return nil, fmt.Errorf("failed to unmarshal account: %v", err)
	}
	
	// Convert from protobuf to local struct
	account = Account{
		Address:  pbAccount.Address,
		Balance:  pbAccount.Balance,
		Nonce:    pbAccount.Nonce,
		CodeHash: pbAccount.Code, // Note: protobuf uses 'Code' field instead of 'CodeHash'
		Storage:  pbAccount.Storage,
	}

	// Add to cache
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accounts[address] = &account
	
	// Prune cache if needed
	if len(s.accounts) > s.cacheSize {
		s.pruneCache()
	}
	
	return &account, nil
}

// UpdateAccount updates an account in the state
func (s *State) UpdateAccount(account *Account) error {
	// Convert to protobuf
	pbAccount := &pb.StateAccount{
		Address: account.Address,
		Balance: account.Balance,
		Nonce:   account.Nonce,
		Code:    account.CodeHash, // Note: protobuf uses 'Code' field instead of 'CodeHash'
		Storage: account.Storage,
	}
	
	data, err := proto.Marshal(pbAccount)
	if err != nil {
		return fmt.Errorf("failed to marshal account: %v", err)
	}

	// Update trie
	key := []byte("account:" + account.Address)
	if err := s.trie.Put(key, data); err != nil {
		return fmt.Errorf("failed to update account in trie: %v", err)
	}

	// Update cache
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accounts[account.Address] = account
	
	return nil
}

// CreateAccount creates a new account in the state
func (s *State) CreateAccount(address string, initialBalance uint64) (*Account, error) {
	// Check if account already exists
	s.mu.RLock()
	_, exists := s.accounts[address]
	s.mu.RUnlock()
	
	if exists {
		return nil, fmt.Errorf("account already exists: %s", address)
	}
	
	// Create new account
	account := &Account{
		Address: address,
		Balance: initialBalance,
		Nonce:   0,
		Storage: make(map[string][]byte),
	}
	
	// Update trie
	// Convert to protobuf
	pbAccount := &pb.StateAccount{
		Address: account.Address,
		Balance: account.Balance,
		Nonce:   account.Nonce,
		Code:    account.CodeHash, // Note: protobuf uses 'Code' field instead of 'CodeHash'
		Storage: account.Storage,
	}
	
	data, err := proto.Marshal(pbAccount)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal account: %v", err)
	}
	key := []byte("account:" + address)
	if err := s.trie.Put(key, data); err != nil {
		return nil, fmt.Errorf("failed to store account in trie: %v", err)
	}
	
	// Update cache
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accounts[address] = account
	
	return account, nil
}

// Transfer transfers funds between accounts
func (s *State) Transfer(from, to string, amount uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	// Get sender account
	fromAccount, err := s.GetAccount(from)
	if err != nil {
		return fmt.Errorf("sender account not found: %s", from)
	}
	
	// Check balance
	if fromAccount.Balance < amount {
		return fmt.Errorf("insufficient balance: %d < %d", fromAccount.Balance, amount)
	}
	
	// Get receiver account
	toAccount, err := s.GetAccount(to)
	if err != nil {
		// Create receiver account if it doesn't exist
		toAccount, err = s.CreateAccount(to, 0)
		if err != nil {
			return fmt.Errorf("failed to create receiver account: %v", err)
		}
	}
	
	// Update balances
	fromAccount.Balance -= amount
	toAccount.Balance += amount
	fromAccount.Nonce++
	
	// Update accounts in trie (write-through)
	if err := s.UpdateAccount(fromAccount); err != nil {
		return fmt.Errorf("failed to update sender account: %v", err)
	}
	
	if err := s.UpdateAccount(toAccount); err != nil {
		return fmt.Errorf("failed to update receiver account: %v", err)
	}
	
	return nil
}

// GetStorage gets a storage value from an account
func (s *State) GetStorage(address string, key string) ([]byte, error) {
	account, err := s.GetAccount(address)
	if err != nil {
		return nil, err
	}
	
	value, ok := account.Storage[key]
	if !ok {
		return nil, fmt.Errorf("storage key not found: %s", key)
	}
	
	return value, nil
}

// SetStorage sets a storage value in an account
func (s *State) SetStorage(address string, key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	account, err := s.GetAccount(address)
	if err != nil {
		return err
	}
	
	account.Storage[key] = value
	
	return s.UpdateAccount(account)
}

// Commit commits all pending changes to the database (via trie)
func (s *State) Commit() error {
	return s.trie.Commit()
}

// RootHash returns the current state root hash
func (s *State) RootHash() []byte {
	return s.trie.Hash()
}

// pruneCache removes least recently used accounts from cache
func (s *State) pruneCache() {
	// Simple implementation: remove random accounts until cache size is reduced by half
	// In a production system, this would use an LRU cache
	targetSize := s.cacheSize / 2
	for addr := range s.accounts {
		delete(s.accounts, addr)
		if len(s.accounts) <= targetSize {
			break
		}
	}
}