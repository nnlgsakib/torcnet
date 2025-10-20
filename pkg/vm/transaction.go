package vm

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"time"
)

// EVMTransaction represents an EVM-compatible transaction
type EVMTransaction struct {
	Hash      []byte    `json:"hash"`
	Nonce     uint64    `json:"nonce"`
	From      []byte    `json:"from"`
	To        []byte    `json:"to"`        // nil for contract creation
	Value     *big.Int  `json:"value"`
	Gas       uint64    `json:"gas"`
	GasPrice  *big.Int  `json:"gasPrice"`
	Data      []byte    `json:"data"`
	V         *big.Int  `json:"v"`
	R         *big.Int  `json:"r"`
	S         *big.Int  `json:"s"`
	Timestamp time.Time `json:"timestamp"`
}

// TransactionReceipt represents the result of transaction execution
type TransactionReceipt struct {
	TransactionHash   []byte  `json:"transactionHash"`
	TransactionIndex  uint64  `json:"transactionIndex"`
	BlockHash         []byte  `json:"blockHash"`
	BlockNumber       uint64  `json:"blockNumber"`
	From              []byte  `json:"from"`
	To                []byte  `json:"to"`
	CumulativeGasUsed uint64  `json:"cumulativeGasUsed"`
	GasUsed           uint64  `json:"gasUsed"`
	ContractAddress   []byte  `json:"contractAddress"` // For contract creation
	Logs              []Log   `json:"logs"`
	Status            uint64  `json:"status"` // 1 for success, 0 for failure
	EffectiveGasPrice *big.Int `json:"effectiveGasPrice"`
}

// Log represents an EVM log entry
type Log struct {
	Address     []byte    `json:"address"`
	Topics      [][]byte  `json:"topics"`
	Data        []byte    `json:"data"`
	BlockNumber uint64    `json:"blockNumber"`
	TxHash      []byte    `json:"transactionHash"`
	TxIndex     uint64    `json:"transactionIndex"`
	BlockHash   []byte    `json:"blockHash"`
	Index       uint64    `json:"logIndex"`
	Removed     bool      `json:"removed"`
}

// ExecutionResult represents the result of transaction execution
type ExecutionResult struct {
	ReturnData      []byte
	GasUsed         uint64
	GasRefund       uint64
	Err             error
	Logs            []*Log
	CreatedAddr     []byte // For contract creation
	ContractAddress []byte // Alternative name for contract address
	StateChanges    map[string]interface{}
}

// TransactionProcessor handles EVM transaction processing with gas management
type TransactionProcessor struct {
	stateDB       *EVMStateDB
	evm           *EVM
	gasCalculator *GasCalculator
	chainID       *big.Int
}

// NewTransactionProcessor creates a new transaction processor with gas management
func NewTransactionProcessor(stateDB *EVMStateDB, evm *EVM, chainID *big.Int, gasPrice *big.Int) *TransactionProcessor {
	gasCalculator := NewGasCalculator(gasPrice)
	return &TransactionProcessor{
		stateDB:       stateDB,
		evm:           evm,
		gasCalculator: gasCalculator,
		chainID:       chainID,
	}
}

// ProcessTransaction processes an EVM transaction with comprehensive gas management
func (tp *TransactionProcessor) ProcessTransaction(tx *EVMTransaction, blockHeight uint64, blockHash []byte, txIndex uint32) (*TransactionReceipt, error) {
	// Validate gas limit
	if err := tp.gasCalculator.ValidateGasLimit(tx.Gas); err != nil {
		return nil, fmt.Errorf("invalid gas limit: %v", err)
	}

	// Create gas tracker
	gasTracker := NewGasTracker(tx.Gas, tp.gasCalculator)

	// Calculate intrinsic gas
	isContractCreation := len(tx.To) == 0
	intrinsicGas := tp.gasCalculator.CalculateIntrinsicGas(tx.Data, isContractCreation)
	
	// Consume intrinsic gas
	if err := gasTracker.ConsumeGas(intrinsicGas); err != nil {
		return &TransactionReceipt{
			TransactionHash:  tx.Hash,
			TransactionIndex: uint64(txIndex),
			BlockHash:        blockHash,
			BlockNumber:      blockHeight,
			From:             tx.From,
			To:               tx.To,
			GasUsed:          tx.Gas, // All gas consumed on failure
			Status:           0,      // Failed
			Logs:             []Log{},
		}, nil
	}

	// Create snapshot for rollback
	snapshot := tp.stateDB.Snapshot()

	// Check sender balance for gas payment
	senderAddr := []byte(tx.From)
	senderBalance := tp.stateDB.GetBalance(senderAddr)
	maxGasCost := new(big.Int).Mul(new(big.Int).SetUint64(tx.Gas), tx.GasPrice)
	totalCost := new(big.Int).Add(tx.Value, maxGasCost)

	if senderBalance.Cmp(totalCost) < 0 {
		return nil, fmt.Errorf("insufficient balance for gas and value")
	}

	// Deduct gas cost upfront
	newBalance := new(big.Int).Sub(senderBalance, maxGasCost)
	tp.stateDB.SetBalance(senderAddr, newBalance)

	// Increment sender nonce
	senderNonce := tp.stateDB.GetNonce(senderAddr)
	tp.stateDB.SetNonce(senderAddr, senderNonce+1)

	var result *ExecutionResult
	var err error

	if isContractCreation {
		result, err = tp.processContractCreation(tx, gasTracker)
	} else {
		result, err = tp.processContractCall(tx, gasTracker)
	}

	// Calculate final gas usage
	finalGasUsed := gasTracker.FinalizeGas()
	
	// Calculate gas refund
	gasRefund := tx.Gas - finalGasUsed
	refundAmount := new(big.Int).Mul(new(big.Int).SetUint64(gasRefund), tx.GasPrice)
	
	// Refund unused gas to sender
	currentBalance := tp.stateDB.GetBalance(senderAddr)
	refundedBalance := new(big.Int).Add(currentBalance, refundAmount)
	tp.stateDB.SetBalance(senderAddr, refundedBalance)

	// Determine transaction status
	status := uint64(1) // Success
	if err != nil {
		status = 0 // Failed
		tp.stateDB.RevertToSnapshot(snapshot)
		// Still consume gas on failure, but revert state changes
		finalGasUsed = tx.Gas
	}

	// Create transaction receipt
	receipt := &TransactionReceipt{
		TransactionHash:  tx.Hash,
		TransactionIndex: uint64(txIndex),
		BlockHash:        blockHash,
		BlockNumber:      blockHeight,
		From:             tx.From,
		To:               tx.To,
		GasUsed:          finalGasUsed,
		Status:           status,
		Logs:             tp.stateDB.GetLogs(),
	}

	if result != nil && result.ContractAddress != nil {
		receipt.ContractAddress = result.ContractAddress
	}

	// Update logs with transaction info
	for i := range receipt.Logs {
		receipt.Logs[i].BlockNumber = blockHeight
		receipt.Logs[i].BlockHash = blockHash
		receipt.Logs[i].TxHash = tx.Hash
		receipt.Logs[i].TxIndex = uint64(txIndex)
		receipt.Logs[i].Index = uint64(i)
	}

	return receipt, nil
}

// processContractCreation handles contract creation transactions with gas tracking
func (tp *TransactionProcessor) processContractCreation(tx *EVMTransaction, gasTracker *GasTracker) (*ExecutionResult, error) {
	// Calculate gas for contract creation
	createGas := tp.gasCalculator.CalculateCreateGas(uint64(len(tx.Data)))
	if err := gasTracker.ConsumeGas(createGas); err != nil {
		return nil, fmt.Errorf("insufficient gas for contract creation: %v", err)
	}

	// Generate contract address
	senderAddr := []byte(tx.From)
	contractAddr := tp.generateContractAddress(senderAddr, tp.stateDB.GetNonce(senderAddr)-1)

	// Create contract account
	tp.stateDB.CreateAccount(contractAddr)

	// Transfer value if any
	if tx.Value.Sign() > 0 {
		senderBalance := tp.stateDB.GetBalance(senderAddr)
		newSenderBalance := new(big.Int).Sub(senderBalance, tx.Value)
		tp.stateDB.SetBalance(senderAddr, newSenderBalance)

		contractBalance := tp.stateDB.GetBalance(contractAddr)
		newContractBalance := new(big.Int).Add(contractBalance, tx.Value)
		tp.stateDB.SetBalance(contractAddr, newContractBalance)
	}

	// Execute constructor code
	result := &ExecutionResult{
		ReturnData:      []byte{},
		GasUsed:         gasTracker.GetGasUsed(),
		ContractAddress: contractAddr,
		Logs:            []*Log{},
	}

	if len(tx.Data) > 0 {
		// Set contract code
		tp.stateDB.SetCode(contractAddr, tx.Data)
	}

	return result, nil
}

// processContractCall handles contract calls and value transfers with gas tracking
func (tp *TransactionProcessor) processContractCall(tx *EVMTransaction, gasTracker *GasTracker) (*ExecutionResult, error) {
	senderAddr := []byte(tx.From)
	receiverAddr := []byte(tx.To)

	// For simple value transfers (no data), use minimal gas
	var callGas uint64
	if len(tx.Data) == 0 {
		// Simple value transfer - no additional gas needed beyond intrinsic
		callGas = 0
	} else {
		// Contract call - calculate proper gas
		accountExists := tp.stateDB.Exist(receiverAddr)
		var err error
		callGas, err = tp.gasCalculator.CalculateCallGas(tx.Value, gasTracker.GetGasLeft(), 0, accountExists)
		if err != nil {
			return nil, fmt.Errorf("gas calculation failed: %v", err)
		}
	}

	if callGas > 0 {
		if err := gasTracker.ConsumeGas(callGas); err != nil {
			return nil, fmt.Errorf("insufficient gas for call: %v", err)
		}
	}

	// Transfer value if any
	if tx.Value.Sign() > 0 {
		senderBalance := tp.stateDB.GetBalance(senderAddr)
		newSenderBalance := new(big.Int).Sub(senderBalance, tx.Value)
		tp.stateDB.SetBalance(senderAddr, newSenderBalance)

		receiverBalance := tp.stateDB.GetBalance(receiverAddr)
		newReceiverBalance := new(big.Int).Add(receiverBalance, tx.Value)
		tp.stateDB.SetBalance(receiverAddr, newReceiverBalance)
	}

	// If no data, it's a simple value transfer
	if len(tx.Data) == 0 {
		return &ExecutionResult{
			ReturnData: []byte{},
			GasUsed:    gasTracker.GetGasUsed(),
			Logs:       []*Log{},
		}, nil
	}

	// Execute contract call
	result := &ExecutionResult{
		ReturnData: []byte{},
		GasUsed:    gasTracker.GetGasUsed(),
		Logs:       []*Log{},
	}

	// Check if target has code
	code := tp.stateDB.GetCode(receiverAddr)
	if len(code) > 0 {
		// Execute contract code (simplified)
		// In a full implementation, this would use the EVM interpreter
		result.ReturnData = []byte("contract_executed")
	}

	return result, nil
}

// validateTransaction validates a transaction
func (tp *TransactionProcessor) validateTransaction(tx *EVMTransaction) error {
	// Check nonce
	expectedNonce := tp.stateDB.GetNonce(tx.From)
	if tx.Nonce != expectedNonce {
		return ErrInvalidNonce
	}

	// Check gas limit
	if tx.Gas < 21000 {
		return ErrInsufficientGas
	}

	// Check gas price
	if tx.GasPrice.Sign() <= 0 {
		return ErrInvalidGasPrice
	}

	// Validate signature (simplified - in production, implement proper ECDSA verification)
	if tx.V == nil || tx.R == nil || tx.S == nil {
		return ErrInvalidSignature
	}

	return nil
}

// generateContractAddress generates a contract address from sender and nonce
func (tp *TransactionProcessor) generateContractAddress(sender []byte, nonce uint64) []byte {
	// Simple address generation: hash(sender + nonce)
	// In production, use proper RLP encoding as per Ethereum spec
	data := make([]byte, len(sender)+8)
	copy(data, sender)
	
	// Convert nonce to bytes
	for i := 0; i < 8; i++ {
		data[len(sender)+i] = byte(nonce >> (8 * (7 - i)))
	}
	
	hash := sha256.Sum256(data)
	return hash[12:] // Take last 20 bytes for address
}

// EstimateGas estimates gas required for a transaction
func (tp *TransactionProcessor) EstimateGas(tx *EVMTransaction) (uint64, error) {
	// Create a copy of the state for simulation
	snapshot := tp.stateDB.Snapshot()
	defer tp.stateDB.RevertToSnapshot(snapshot)

	// Set a high gas limit for estimation
	estimateTx := *tx
	estimateTx.Gas = 10000000 // 10M gas limit

	var gasUsed uint64

	if len(tx.To) == 0 || IsZeroAddress(tx.To) {
		// Contract creation
		result, err := tp.executeContractCreation(&estimateTx, estimateTx.Gas)
		if err != nil {
			return 0, err
		}
		gasUsed = result.GasUsed
	} else {
		// Contract call
		result, err := tp.executeCall(&estimateTx, estimateTx.Gas)
		if err != nil {
			return 0, err
		}
		gasUsed = result.GasUsed
	}

	// Add some buffer (10%) to the estimated gas
	return gasUsed + (gasUsed / 10), nil
}

// CreateTransaction creates a new EVM transaction
func CreateTransaction(from, to []byte, value *big.Int, gas uint64, gasPrice *big.Int, data []byte, nonce uint64) *EVMTransaction {
	tx := &EVMTransaction{
		Nonce:     nonce,
		From:      from,
		To:        to,
		Value:     value,
		Gas:       gas,
		GasPrice:  gasPrice,
		Data:      data,
		Timestamp: time.Now(),
	}

	// Calculate transaction hash
	tx.Hash = tx.CalculateHash()

	return tx
}

// CalculateHash calculates the transaction hash
func (tx *EVMTransaction) CalculateHash() []byte {
	data := make([]byte, 0)
	
	// Add nonce
	nonceBytes := make([]byte, 8)
	for i := 0; i < 8; i++ {
		nonceBytes[i] = byte(tx.Nonce >> (8 * (7 - i)))
	}
	data = append(data, nonceBytes...)
	
	// Add from address
	data = append(data, tx.From...)
	
	// Add to address
	data = append(data, tx.To...)
	
	// Add value
	valueBytes := make([]byte, 32)
	tx.Value.FillBytes(valueBytes)
	data = append(data, valueBytes...)
	
	// Add gas
	gasBytes := make([]byte, 8)
	for i := 0; i < 8; i++ {
		gasBytes[i] = byte(tx.Gas >> (8 * (7 - i)))
	}
	data = append(data, gasBytes...)
	
	// Add gas price
	gasPriceBytes := make([]byte, 32)
	tx.GasPrice.FillBytes(gasPriceBytes)
	data = append(data, gasPriceBytes...)
	
	// Add data
	data = append(data, tx.Data...)
	
	hash := sha256.Sum256(data)
	return hash[:]
}

// ToHex converts transaction hash to hex string
func (tx *EVMTransaction) ToHex() string {
	return "0x" + hex.EncodeToString(tx.Hash)
}

// VMError represents a virtual machine error
type VMError struct {
	Code    string
	Message string
}

func (e *VMError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Additional error types for transaction processing
var (
	ErrInsufficientBalance = &VMError{Code: "insufficient_balance", Message: "insufficient balance for transfer"}
	ErrInvalidNonce        = &VMError{Code: "invalid_nonce", Message: "invalid transaction nonce"}
	ErrInsufficientGas     = &VMError{Code: "insufficient_gas", Message: "insufficient gas for transaction"}
	ErrInvalidGasPrice     = &VMError{Code: "invalid_gas_price", Message: "invalid gas price"}
	ErrInvalidSignature    = &VMError{Code: "invalid_signature", Message: "invalid transaction signature"}
)

// executeContractCreation handles contract creation transactions
func (tp *TransactionProcessor) executeContractCreation(tx *EVMTransaction, gasLimit uint64) (*ExecutionResult, error) {
	// Create new contract address
	contractAddr := tp.generateContractAddress(tx.From, tx.Nonce)
	
	// Create contract context
	codeHash := sha256.Sum256(tx.Data)
	contract := &Contract{
		Address:  contractAddr,
		Code:     tx.Data,
		CodeHash: codeHash[:],
		Input:    tx.Data,
		Gas:      gasLimit,
		Value:    tx.Value,
		Caller:   tx.From,
	}
	
	// Set contract in EVM
	tp.evm.contract = contract
	
	// Execute contract creation
	result, err := tp.evm.Run()
	if err != nil {
		return &ExecutionResult{
			ReturnData: nil,
			GasUsed:    gasLimit,
			Err:        err,
			Logs:       convertLogsToPointers(tp.stateDB.GetLogs()),
			CreatedAddr: nil,
			ContractAddress: nil,
		}, err
	}
	
	// Store contract code if successful
	if err == nil && len(result) > 0 {
		tp.stateDB.SetCode(contractAddr, result)
	}
	
	return &ExecutionResult{
		ReturnData: result,
		GasUsed:    gasLimit - contract.Gas,
		Err:        nil,
		Logs:       convertLogsToPointers(tp.stateDB.GetLogs()),
		CreatedAddr: contractAddr,
		ContractAddress: contractAddr,
	}, nil
}

// executeCall handles contract call transactions
func (tp *TransactionProcessor) executeCall(tx *EVMTransaction, gasLimit uint64) (*ExecutionResult, error) {
	// Get contract code
	code := tp.stateDB.GetCode(tx.To)
	if len(code) == 0 {
		// For simple value transfers (no contract code), calculate intrinsic gas
		intrinsicGas := tp.gasCalculator.CalculateIntrinsicGas(tx.Data, false)
		return &ExecutionResult{
			ReturnData: nil,
			GasUsed:    intrinsicGas,
			Err:        nil,
			Logs:       convertLogsToPointers(tp.stateDB.GetLogs()),
			CreatedAddr: nil,
			ContractAddress: nil,
		}, nil
	}
	
	// Create contract context
	codeHash := sha256.Sum256(code)
	contract := &Contract{
		Address:  tx.To,
		Code:     code,
		CodeHash: codeHash[:],
		Input:    tx.Data,
		Gas:      gasLimit,
		Value:    tx.Value,
		Caller:   tx.From,
	}
	
	// Set contract in EVM
	tp.evm.contract = contract
	
	// Execute contract call
	result, err := tp.evm.Run()
	if err != nil {
		return &ExecutionResult{
			ReturnData: nil,
			GasUsed:    gasLimit,
			Err:        err,
			Logs:       convertLogsToPointers(tp.stateDB.GetLogs()),
			CreatedAddr: nil,
			ContractAddress: nil,
		}, err
	}
	
	return &ExecutionResult{
		ReturnData: result,
		GasUsed:    gasLimit - contract.Gas,
		Err:        nil,
		Logs:       convertLogsToPointers(tp.stateDB.GetLogs()),
		CreatedAddr: nil,
		ContractAddress: nil,
	}, nil
}

// convertLogsToPointers converts []Log to []*Log
func convertLogsToPointers(logs []Log) []*Log {
	result := make([]*Log, len(logs))
	for i := range logs {
		result[i] = &logs[i]
	}
	return result
}