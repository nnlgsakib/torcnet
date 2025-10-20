package vm

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"sync"
	
	"github.com/torcnet/torcnet/pkg/db"
)

// StateDB implementation for EVM compatibility
type EVMStateDB struct {
	db       db.Database
	accounts map[string]*Account
	logs     []Log
	snapshots []StateSnapshot
	mutex    sync.RWMutex
}

// Account represents an Ethereum account
type Account struct {
	Address  []byte
	Nonce    uint64
	Balance  *big.Int
	Code     []byte
	CodeHash []byte
	Storage  map[string][]byte
	Suicided bool
	Dirty    bool
}

// StateSnapshot represents a state snapshot for rollback
type StateSnapshot struct {
	ID       int
	Accounts map[string]*Account
	Logs     []Log
}

// NewEVMStateDB creates a new EVM state database
func NewEVMStateDB(database db.Database) *EVMStateDB {
	return &EVMStateDB{
		db:        database,
		accounts:  make(map[string]*Account),
		logs:      make([]Log, 0),
		snapshots: make([]StateSnapshot, 0),
	}
}

// GetBalance returns the balance of an account
func (s *EVMStateDB) GetBalance(addr []byte) *big.Int {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	account := s.getAccount(addr)
	if account == nil {
		return big.NewInt(0)
	}
	return new(big.Int).Set(account.Balance)
}

// SetBalance sets the balance of an account
func (s *EVMStateDB) SetBalance(addr []byte, amount *big.Int) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	
	account := s.getOrCreateAccount(addr)
	account.Balance = new(big.Int).Set(amount)
	account.Dirty = true
}

// GetNonce returns the nonce of an account
func (s *EVMStateDB) GetNonce(addr []byte) uint64 {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	account := s.getAccount(addr)
	if account == nil {
		return 0
	}
	return account.Nonce
}

// SetNonce sets the nonce of an account
func (s *EVMStateDB) SetNonce(addr []byte, nonce uint64) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	
	account := s.getOrCreateAccount(addr)
	account.Nonce = nonce
	account.Dirty = true
}

// GetCode returns the code of an account
func (s *EVMStateDB) GetCode(addr []byte) []byte {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	account := s.getAccount(addr)
	if account == nil {
		return nil
	}
	return account.Code
}

// SetCode sets the code of an account
func (s *EVMStateDB) SetCode(addr []byte, code []byte) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	
	account := s.getOrCreateAccount(addr)
	account.Code = make([]byte, len(code))
	copy(account.Code, code)
	
	// Calculate code hash
	hash := sha256.Sum256(code)
	account.CodeHash = hash[:]
	account.Dirty = true
}

// GetCodeHash returns the code hash of an account
func (s *EVMStateDB) GetCodeHash(addr []byte) []byte {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	account := s.getAccount(addr)
	if account == nil {
		return nil
	}
	return account.CodeHash
}

// GetState returns the storage value for a key
func (s *EVMStateDB) GetState(addr []byte, key []byte) []byte {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	account := s.getAccount(addr)
	if account == nil {
		return make([]byte, 32) // Return zero value
	}
	
	keyStr := hex.EncodeToString(key)
	if value, exists := account.Storage[keyStr]; exists {
		return value
	}
	
	// Try to load from database
	dbKey := append(addr, key...)
	value, err := s.db.Get(dbKey)
	if err != nil {
		return make([]byte, 32) // Return zero value on error
	}
	
	// Cache the value
	if account.Storage == nil {
		account.Storage = make(map[string][]byte)
	}
	account.Storage[keyStr] = value
	
	return value
}

// SetState sets the storage value for a key
func (s *EVMStateDB) SetState(addr []byte, key []byte, value []byte) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	
	account := s.getOrCreateAccount(addr)
	if account.Storage == nil {
		account.Storage = make(map[string][]byte)
	}
	
	keyStr := hex.EncodeToString(key)
	
	// Ensure value is 32 bytes
	storageValue := make([]byte, 32)
	copy(storageValue[32-len(value):], value)
	
	account.Storage[keyStr] = storageValue
	account.Dirty = true
}

// Exist checks if an account exists
func (s *EVMStateDB) Exist(addr []byte) bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	return s.getAccount(addr) != nil
}

// Empty checks if an account is empty (no nonce, balance, or code)
func (s *EVMStateDB) Empty(addr []byte) bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	account := s.getAccount(addr)
	if account == nil {
		return true
	}
	
	return account.Nonce == 0 && 
		   account.Balance.Sign() == 0 && 
		   len(account.Code) == 0
}

// CreateAccount creates a new account
func (s *EVMStateDB) CreateAccount(addr []byte) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	
	s.getOrCreateAccount(addr)
}

// Suicide marks an account for deletion
func (s *EVMStateDB) Suicide(addr []byte) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	
	account := s.getAccount(addr)
	if account == nil {
		return false
	}
	
	account.Suicided = true
	account.Dirty = true
	return true
}

// HasSuicided checks if an account has been marked for deletion
func (s *EVMStateDB) HasSuicided(addr []byte) bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	account := s.getAccount(addr)
	if account == nil {
		return false
	}
	
	return account.Suicided
}

// AddLog adds a log entry
func (s *EVMStateDB) AddLog(addr []byte, topics [][]byte, data []byte) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	
	log := Log{
		Address: make([]byte, len(addr)),
		Topics:  make([][]byte, len(topics)),
		Data:    make([]byte, len(data)),
	}
	
	copy(log.Address, addr)
	copy(log.Data, data)
	
	for i, topic := range topics {
		log.Topics[i] = make([]byte, len(topic))
		copy(log.Topics[i], topic)
	}
	
	s.logs = append(s.logs, log)
}

// GetLogs returns all logs
func (s *EVMStateDB) GetLogs() []Log {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	logs := make([]Log, len(s.logs))
	copy(logs, s.logs)
	return logs
}

// Snapshot creates a state snapshot
func (s *EVMStateDB) Snapshot() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	
	id := len(s.snapshots)
	
	// Deep copy accounts
	accountsCopy := make(map[string]*Account)
	for addr, account := range s.accounts {
		accountCopy := &Account{
			Address:  make([]byte, len(account.Address)),
			Nonce:    account.Nonce,
			Balance:  new(big.Int).Set(account.Balance),
			Code:     make([]byte, len(account.Code)),
			CodeHash: make([]byte, len(account.CodeHash)),
			Storage:  make(map[string][]byte),
			Suicided: account.Suicided,
			Dirty:    account.Dirty,
		}
		
		copy(accountCopy.Address, account.Address)
		copy(accountCopy.Code, account.Code)
		copy(accountCopy.CodeHash, account.CodeHash)
		
		for key, value := range account.Storage {
			accountCopy.Storage[key] = make([]byte, len(value))
			copy(accountCopy.Storage[key], value)
		}
		
		accountsCopy[addr] = accountCopy
	}
	
	// Deep copy logs
	logsCopy := make([]Log, len(s.logs))
	for i, log := range s.logs {
		logsCopy[i] = Log{
			Address: make([]byte, len(log.Address)),
			Topics:  make([][]byte, len(log.Topics)),
			Data:    make([]byte, len(log.Data)),
		}
		
		copy(logsCopy[i].Address, log.Address)
		copy(logsCopy[i].Data, log.Data)
		
		for j, topic := range log.Topics {
			logsCopy[i].Topics[j] = make([]byte, len(topic))
			copy(logsCopy[i].Topics[j], topic)
		}
	}
	
	snapshot := StateSnapshot{
		ID:       id,
		Accounts: accountsCopy,
		Logs:     logsCopy,
	}
	
	s.snapshots = append(s.snapshots, snapshot)
	return id
}

// RevertToSnapshot reverts state to a snapshot
func (s *EVMStateDB) RevertToSnapshot(id int) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	
	if id < 0 || id >= len(s.snapshots) {
		return
	}
	
	snapshot := s.snapshots[id]
	s.accounts = snapshot.Accounts
	s.logs = snapshot.Logs
	
	// Remove snapshots after the reverted one
	s.snapshots = s.snapshots[:id]
}

// Commit commits all changes to the database
func (s *EVMStateDB) Commit() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	
	batch := s.db.Batch()
	
	for addrStr, account := range s.accounts {
		if !account.Dirty {
			continue
		}
		
		addr, _ := hex.DecodeString(addrStr)
		
		// Save account data
		accountKey := append([]byte("account:"), addr...)
		accountData := s.encodeAccount(account)
		batch.Put(accountKey, accountData)
		
		// Save storage
		for keyStr, value := range account.Storage {
			key, _ := hex.DecodeString(keyStr)
			storageKey := append(addr, key...)
			batch.Put(storageKey, value)
		}
		
		// Mark as clean
		account.Dirty = false
	}
	
	return batch.Write()
}

// LoadFromDatabase loads state from database
func (s *EVMStateDB) LoadFromDatabase() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	
	// Implementation would iterate through database keys
	// and load account data. For simplicity, we'll leave this
	// as a placeholder that can be extended based on the
	// specific database schema used.
	
	return nil
}

// Helper methods

func (s *EVMStateDB) getAccount(addr []byte) *Account {
	addrStr := hex.EncodeToString(addr)
	if account, exists := s.accounts[addrStr]; exists {
		return account
	}
	
	// Try to load from database
	accountKey := append([]byte("account:"), addr...)
	data, err := s.db.Get(accountKey)
	if err != nil {
		return nil
	}
	
	account := s.decodeAccount(data)
	if account != nil {
		account.Address = make([]byte, len(addr))
		copy(account.Address, addr)
		s.accounts[addrStr] = account
	}
	
	return account
}

func (s *EVMStateDB) getOrCreateAccount(addr []byte) *Account {
	account := s.getAccount(addr)
	if account != nil {
		return account
	}
	
	// Create new account
	account = &Account{
		Address: make([]byte, len(addr)),
		Nonce:   0,
		Balance: big.NewInt(0),
		Code:    nil,
		Storage: make(map[string][]byte),
		Dirty:   true,
	}
	
	copy(account.Address, addr)
	
	addrStr := hex.EncodeToString(addr)
	s.accounts[addrStr] = account
	
	return account
}

func (s *EVMStateDB) encodeAccount(account *Account) []byte {
	// Simple encoding - in production, use proper serialization
	data := make([]byte, 0)
	
	// Nonce (8 bytes)
	nonceBytes := make([]byte, 8)
	for i := 0; i < 8; i++ {
		nonceBytes[i] = byte(account.Nonce >> (8 * (7 - i)))
	}
	data = append(data, nonceBytes...)
	
	// Balance (32 bytes)
	balanceBytes := make([]byte, 32)
	account.Balance.FillBytes(balanceBytes)
	data = append(data, balanceBytes...)
	
	// Code length (4 bytes) + code
	codeLen := make([]byte, 4)
	for i := 0; i < 4; i++ {
		codeLen[i] = byte(len(account.Code) >> (8 * (3 - i)))
	}
	data = append(data, codeLen...)
	data = append(data, account.Code...)
	
	// Code hash (32 bytes)
	if len(account.CodeHash) == 32 {
		data = append(data, account.CodeHash...)
	} else {
		data = append(data, make([]byte, 32)...)
	}
	
	// Suicided flag (1 byte)
	if account.Suicided {
		data = append(data, 1)
	} else {
		data = append(data, 0)
	}
	
	return data
}

func (s *EVMStateDB) decodeAccount(data []byte) *Account {
	if len(data) < 77 { // Minimum size: 8 + 32 + 4 + 0 + 32 + 1
		return nil
	}
	
	account := &Account{
		Storage: make(map[string][]byte),
	}
	
	offset := 0
	
	// Nonce (8 bytes)
	nonce := uint64(0)
	for i := 0; i < 8; i++ {
		nonce = (nonce << 8) | uint64(data[offset+i])
	}
	account.Nonce = nonce
	offset += 8
	
	// Balance (32 bytes)
	account.Balance = new(big.Int).SetBytes(data[offset : offset+32])
	offset += 32
	
	// Code length (4 bytes)
	codeLen := 0
	for i := 0; i < 4; i++ {
		codeLen = (codeLen << 8) | int(data[offset+i])
	}
	offset += 4
	
	// Code
	if codeLen > 0 && offset+codeLen <= len(data) {
		account.Code = make([]byte, codeLen)
		copy(account.Code, data[offset:offset+codeLen])
	}
	offset += codeLen
	
	// Code hash (32 bytes)
	if offset+32 <= len(data) {
		account.CodeHash = make([]byte, 32)
		copy(account.CodeHash, data[offset:offset+32])
	}
	offset += 32
	
	// Suicided flag (1 byte)
	if offset < len(data) {
		account.Suicided = data[offset] == 1
	}
	
	return account
}

// AddressFromBytes converts bytes to a 20-byte address
func AddressFromBytes(b []byte) []byte {
	addr := make([]byte, 20)
	if len(b) > 20 {
		copy(addr, b[len(b)-20:])
	} else {
		copy(addr[20-len(b):], b)
	}
	return addr
}

// BytesToHash converts bytes to a 32-byte hash
func BytesToHash(b []byte) []byte {
	hash := make([]byte, 32)
	if len(b) > 32 {
		copy(hash, b[len(b)-32:])
	} else {
		copy(hash[32-len(b):], b)
	}
	return hash
}

// IsZeroAddress checks if an address is zero
func IsZeroAddress(addr []byte) bool {
	return bytes.Equal(addr, make([]byte, 20))
}

// IsZeroHash checks if a hash is zero
func IsZeroHash(hash []byte) bool {
	return bytes.Equal(hash, make([]byte, 32))
}