package vm

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"sync"
	"time"
)

// ContractHandler manages smart contract deployment and execution
type ContractHandler struct {
	stateDB   *EVMStateDB
	evm       *EVM
	processor *TransactionProcessor
	
	// Contract registry
	contracts map[string]*DeployedContract
	mutex     sync.RWMutex
	
	// Event handling
	eventListeners map[string][]EventListener
	eventMutex     sync.RWMutex
}

// DeployedContract represents a deployed smart contract
type DeployedContract struct {
	Address     []byte            `json:"address"`
	Code        []byte            `json:"code"`
	ABI         []ABIMethod       `json:"abi"`
	Creator     []byte            `json:"creator"`
	DeployedAt  time.Time         `json:"deployedAt"`
	TxHash      []byte            `json:"txHash"`
	BlockNumber uint64            `json:"blockNumber"`
	Metadata    map[string]string `json:"metadata"`
}

// ABIMethod represents a contract method in the ABI
type ABIMethod struct {
	Name     string      `json:"name"`
	Type     string      `json:"type"`
	Inputs   []ABIParam  `json:"inputs"`
	Outputs  []ABIParam  `json:"outputs"`
	Constant bool        `json:"constant"`
	Payable  bool        `json:"payable"`
}

// ABIParam represents a method parameter in the ABI
type ABIParam struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// EventListener represents an event listener for contract events
type EventListener struct {
	ContractAddress []byte
	EventSignature  []byte
	Callback        func(log Log)
}

// ContractDeploymentRequest represents a contract deployment request
type ContractDeploymentRequest struct {
	From        []byte            `json:"from"`
	Code        []byte            `json:"code"`
	Constructor []byte            `json:"constructor"`
	Value       *big.Int          `json:"value"`
	Gas         uint64            `json:"gas"`
	GasPrice    *big.Int          `json:"gasPrice"`
	ABI         []ABIMethod       `json:"abi"`
	Metadata    map[string]string `json:"metadata"`
}

// ContractCallRequest represents a contract method call request
type ContractCallRequest struct {
	From     []byte   `json:"from"`
	To       []byte   `json:"to"`
	Method   string   `json:"method"`
	Params   [][]byte `json:"params"`
	Value    *big.Int `json:"value"`
	Gas      uint64   `json:"gas"`
	GasPrice *big.Int `json:"gasPrice"`
}

// ContractCallResult represents the result of a contract call
type ContractCallResult struct {
	Success     bool        `json:"success"`
	ReturnData  []byte      `json:"returnData"`
	GasUsed     uint64      `json:"gasUsed"`
	Logs        []Log       `json:"logs"`
	Error       string      `json:"error,omitempty"`
	DecodedData interface{} `json:"decodedData,omitempty"`
}

// NewContractHandler creates a new contract handler
func NewContractHandler(stateDB *EVMStateDB, evm *EVM, processor *TransactionProcessor) *ContractHandler {
	return &ContractHandler{
		stateDB:        stateDB,
		evm:            evm,
		processor:      processor,
		contracts:      make(map[string]*DeployedContract),
		eventListeners: make(map[string][]EventListener),
	}
}

// DeployContract deploys a new smart contract
func (ch *ContractHandler) DeployContract(req *ContractDeploymentRequest) (*DeployedContract, *TransactionReceipt, error) {
	// Validate deployment request
	if err := ch.validateDeploymentRequest(req); err != nil {
		return nil, nil, fmt.Errorf("invalid deployment request: %v", err)
	}

	// Get deployer nonce
	nonce := ch.stateDB.GetNonce(req.From)

	// Combine constructor code with contract code
	deploymentCode := append(req.Constructor, req.Code...)

	// Create deployment transaction
	tx := CreateTransaction(
		req.From,
		nil, // Contract creation (to = nil)
		req.Value,
		req.Gas,
		req.GasPrice,
		deploymentCode,
		nonce,
	)

	// Process deployment transaction
	blockHeight := uint64(1) // TODO: Get actual block height
	blockHash := make([]byte, 32) // TODO: Get actual block hash
	receipt, err := ch.processor.ProcessTransaction(tx, blockHeight, blockHash, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("deployment failed: %v", err)
	}

	if receipt.Status == 0 {
		return nil, receipt, fmt.Errorf("deployment transaction failed")
	}

	// Create deployed contract record
	contract := &DeployedContract{
		Address:     receipt.ContractAddress,
		Code:        req.Code,
		ABI:         req.ABI,
		Creator:     req.From,
		DeployedAt:  time.Now(),
		TxHash:      tx.Hash,
		BlockNumber: blockHeight,
		Metadata:    req.Metadata,
	}

	// Register contract
	ch.registerContract(contract)

	return contract, receipt, nil
}

// CallContract calls a method on a deployed contract
func (ch *ContractHandler) CallContract(req *ContractCallRequest) (*ContractCallResult, error) {
	// Validate call request
	if err := ch.validateCallRequest(req); err != nil {
		return nil, fmt.Errorf("invalid call request: %v", err)
	}

	// Get contract
	contract := ch.GetContract(req.To)
	if contract == nil {
		return &ContractCallResult{
			Success: false,
			Error:   "contract not found",
		}, nil
	}

	// Encode method call
	callData, err := ch.encodeMethodCall(contract, req.Method, req.Params)
	if err != nil {
		return &ContractCallResult{
			Success: false,
			Error:   fmt.Sprintf("failed to encode method call: %v", err),
		}, nil
	}

	// Get caller nonce
	nonce := ch.stateDB.GetNonce(req.From)

	// Create call transaction
	tx := CreateTransaction(
		req.From,
		req.To,
		req.Value,
		req.Gas,
		req.GasPrice,
		callData,
		nonce,
	)

	// Process call transaction
	blockHeight := uint64(1) // TODO: Get actual block height
	blockHash := make([]byte, 32) // TODO: Get actual block hash
	receipt, err := ch.processor.ProcessTransaction(tx, blockHeight, blockHash, 0)
	if err != nil {
		return &ContractCallResult{
			Success: false,
			Error:   fmt.Sprintf("call failed: %v", err),
		}, nil
	}

	result := &ContractCallResult{
		Success:    receipt.Status == 1,
		GasUsed:    receipt.GasUsed,
		Logs:       receipt.Logs,
	}

	if receipt.Status == 0 {
		result.Error = "transaction reverted"
	}

	// TODO: Decode return data based on method ABI
	// For now, return raw data
	// result.ReturnData = receipt.ReturnData
	// result.DecodedData = ch.decodeReturnData(contract, req.Method, receipt.ReturnData)

	return result, nil
}

// CallContractView performs a read-only call to a contract (no state changes)
func (ch *ContractHandler) CallContractView(req *ContractCallRequest) (*ContractCallResult, error) {
	// Create snapshot for read-only execution
	snapshot := ch.stateDB.Snapshot()
	defer ch.stateDB.RevertToSnapshot(snapshot)

	// Get contract code
	code := ch.stateDB.GetCode(req.To)
	if len(code) == 0 {
		return &ContractCallResult{
			Success: false,
			Error:   "no code at address",
		}, nil
	}

	// Get contract
	contract := ch.GetContract(req.To)
	if contract == nil {
		return &ContractCallResult{
			Success: false,
			Error:   "contract not found",
		}, nil
	}

	// Encode method call
	callData, err := ch.encodeMethodCall(contract, req.Method, req.Params)
	if err != nil {
		return &ContractCallResult{
			Success: false,
			Error:   fmt.Sprintf("failed to encode method call: %v", err),
		}, nil
	}

	// Create contract context
	contractCtx := &Contract{
		Address: req.To,
		Caller:  req.From,
		Value:   req.Value,
		Gas:     req.Gas,
		Code:    code,
		Input:   callData,
	}

	// Execute contract (read-only)
	// Set the contract in EVM context
	ch.evm.contract = contractCtx
	result, err := ch.evm.Run()

	callResult := &ContractCallResult{
		Success:    err == nil,
		ReturnData: result,
		GasUsed:    contractCtx.Gas, // Simplified gas tracking
		Logs:       ch.stateDB.GetLogs(),
	}

	if err != nil {
		callResult.Error = err.Error()
	}

	// TODO: Decode return data
	// callResult.DecodedData = ch.decodeReturnData(contract, req.Method, result.ReturnData)

	return callResult, nil
}

// GetContract retrieves a deployed contract by address
func (ch *ContractHandler) GetContract(address []byte) *DeployedContract {
	ch.mutex.RLock()
	defer ch.mutex.RUnlock()
	
	addrStr := hex.EncodeToString(address)
	return ch.contracts[addrStr]
}

// ListContracts returns all deployed contracts
func (ch *ContractHandler) ListContracts() []*DeployedContract {
	ch.mutex.RLock()
	defer ch.mutex.RUnlock()
	
	contracts := make([]*DeployedContract, 0, len(ch.contracts))
	for _, contract := range ch.contracts {
		contracts = append(contracts, contract)
	}
	
	return contracts
}

// GetContractsByCreator returns contracts deployed by a specific address
func (ch *ContractHandler) GetContractsByCreator(creator []byte) []*DeployedContract {
	ch.mutex.RLock()
	defer ch.mutex.RUnlock()
	
	var contracts []*DeployedContract
	for _, contract := range ch.contracts {
		if hex.EncodeToString(contract.Creator) == hex.EncodeToString(creator) {
			contracts = append(contracts, contract)
		}
	}
	
	return contracts
}

// AddEventListener adds an event listener for contract events
func (ch *ContractHandler) AddEventListener(contractAddr []byte, eventSig []byte, callback func(Log)) {
	ch.eventMutex.Lock()
	defer ch.eventMutex.Unlock()
	
	addrStr := hex.EncodeToString(contractAddr)
	listener := EventListener{
		ContractAddress: contractAddr,
		EventSignature:  eventSig,
		Callback:        callback,
	}
	
	ch.eventListeners[addrStr] = append(ch.eventListeners[addrStr], listener)
}

// ProcessContractEvents processes contract events from logs
func (ch *ContractHandler) ProcessContractEvents(logs []Log) {
	ch.eventMutex.RLock()
	defer ch.eventMutex.RUnlock()
	
	for _, log := range logs {
		addrStr := hex.EncodeToString(log.Address)
		if listeners, exists := ch.eventListeners[addrStr]; exists {
			for _, listener := range listeners {
				// Check if event signature matches (first topic)
				if len(log.Topics) > 0 && 
				   hex.EncodeToString(log.Topics[0]) == hex.EncodeToString(listener.EventSignature) {
					go listener.Callback(log)
				}
			}
		}
	}
}

// Helper methods

func (ch *ContractHandler) registerContract(contract *DeployedContract) {
	ch.mutex.Lock()
	defer ch.mutex.Unlock()
	
	addrStr := hex.EncodeToString(contract.Address)
	ch.contracts[addrStr] = contract
}

func (ch *ContractHandler) validateDeploymentRequest(req *ContractDeploymentRequest) error {
	if len(req.From) != 20 {
		return fmt.Errorf("invalid from address")
	}
	
	if len(req.Code) == 0 {
		return fmt.Errorf("contract code cannot be empty")
	}
	
	if req.Gas < 21000 {
		return fmt.Errorf("insufficient gas")
	}
	
	if req.GasPrice == nil || req.GasPrice.Sign() <= 0 {
		return fmt.Errorf("invalid gas price")
	}
	
	if req.Value == nil {
		req.Value = big.NewInt(0)
	}
	
	return nil
}

func (ch *ContractHandler) validateCallRequest(req *ContractCallRequest) error {
	if len(req.From) != 20 {
		return fmt.Errorf("invalid from address")
	}
	
	if len(req.To) != 20 {
		return fmt.Errorf("invalid to address")
	}
	
	if req.Method == "" {
		return fmt.Errorf("method name cannot be empty")
	}
	
	if req.Gas < 21000 {
		return fmt.Errorf("insufficient gas")
	}
	
	if req.GasPrice == nil || req.GasPrice.Sign() <= 0 {
		return fmt.Errorf("invalid gas price")
	}
	
	if req.Value == nil {
		req.Value = big.NewInt(0)
	}
	
	return nil
}

func (ch *ContractHandler) encodeMethodCall(contract *DeployedContract, method string, params [][]byte) ([]byte, error) {
	// Find method in ABI
	var methodABI *ABIMethod
	for _, m := range contract.ABI {
		if m.Name == method {
			methodABI = &m
			break
		}
	}
	
	if methodABI == nil {
		return nil, fmt.Errorf("method %s not found in contract ABI", method)
	}
	
	// Generate method signature (first 4 bytes of keccak256 hash)
	signature := fmt.Sprintf("%s(", method)
	for i, input := range methodABI.Inputs {
		if i > 0 {
			signature += ","
		}
		signature += input.Type
	}
	signature += ")"
	
	// Calculate method selector (simplified - use SHA256 instead of Keccak256)
	hash := sha256.Sum256([]byte(signature))
	selector := hash[:4]
	
	// Encode parameters (simplified encoding)
	callData := make([]byte, len(selector))
	copy(callData, selector)
	
	// Append parameters (simplified - just concatenate)
	for _, param := range params {
		// Pad to 32 bytes
		paddedParam := make([]byte, 32)
		copy(paddedParam[32-len(param):], param)
		callData = append(callData, paddedParam...)
	}
	
	return callData, nil
}

// EstimateContractDeployment estimates gas for contract deployment
func (ch *ContractHandler) EstimateContractDeployment(req *ContractDeploymentRequest) (uint64, error) {
	// Create a temporary transaction for estimation
	nonce := ch.stateDB.GetNonce(req.From)
	deploymentCode := append(req.Constructor, req.Code...)
	
	tx := CreateTransaction(
		req.From,
		nil,
		req.Value,
		10000000, // High gas limit for estimation
		req.GasPrice,
		deploymentCode,
		nonce,
	)
	
	return ch.processor.EstimateGas(tx)
}

// EstimateContractCall estimates gas for contract method call
func (ch *ContractHandler) EstimateContractCall(req *ContractCallRequest) (uint64, error) {
	contract := ch.GetContract(req.To)
	if contract == nil {
		return 0, fmt.Errorf("contract not found")
	}
	
	callData, err := ch.encodeMethodCall(contract, req.Method, req.Params)
	if err != nil {
		return 0, err
	}
	
	nonce := ch.stateDB.GetNonce(req.From)
	
	tx := CreateTransaction(
		req.From,
		req.To,
		req.Value,
		10000000, // High gas limit for estimation
		req.GasPrice,
		callData,
		nonce,
	)
	
	return ch.processor.EstimateGas(tx)
}

// GetContractStorage retrieves storage value from a contract
func (ch *ContractHandler) GetContractStorage(contractAddr []byte, key []byte) []byte {
	return ch.stateDB.GetState(contractAddr, key)
}

// IsContract checks if an address contains contract code
func (ch *ContractHandler) IsContract(addr []byte) bool {
	code := ch.stateDB.GetCode(addr)
	return len(code) > 0
}

// GetContractCodeSize returns the size of contract code
func (ch *ContractHandler) GetContractCodeSize(addr []byte) uint64 {
	code := ch.stateDB.GetCode(addr)
	return uint64(len(code))
}

// Standard contract interfaces

// ERC20Token represents an ERC-20 token contract interface
type ERC20Token struct {
	handler *ContractHandler
	address []byte
}

// NewERC20Token creates a new ERC-20 token interface
func NewERC20Token(handler *ContractHandler, address []byte) *ERC20Token {
	return &ERC20Token{
		handler: handler,
		address: address,
	}
}

// BalanceOf returns the token balance of an address
func (token *ERC20Token) BalanceOf(owner []byte) (*big.Int, error) {
	// Encode balanceOf(address) call
	req := &ContractCallRequest{
		From:   make([]byte, 20), // Zero address for view calls
		To:     token.address,
		Method: "balanceOf",
		Params: [][]byte{owner},
		Gas:    100000,
		GasPrice: big.NewInt(1),
	}
	
	result, err := token.handler.CallContractView(req)
	if err != nil {
		return nil, err
	}
	
	if !result.Success {
		return nil, fmt.Errorf("call failed: %s", result.Error)
	}
	
	// Decode return data (simplified)
	if len(result.ReturnData) >= 32 {
		balance := new(big.Int).SetBytes(result.ReturnData[:32])
		return balance, nil
	}
	
	return big.NewInt(0), nil
}

// Transfer transfers tokens to another address
func (token *ERC20Token) Transfer(from []byte, to []byte, amount *big.Int, gasPrice *big.Int) (*TransactionReceipt, error) {
	// Encode transfer(address,uint256) call
	amountBytes := make([]byte, 32)
	amount.FillBytes(amountBytes)
	
	req := &ContractCallRequest{
		From:     from,
		To:       token.address,
		Method:   "transfer",
		Params:   [][]byte{to, amountBytes},
		Gas:      100000,
		GasPrice: gasPrice,
	}
	
	result, err := token.handler.CallContract(req)
	if err != nil {
		return nil, err
	}
	
	if !result.Success {
		return nil, fmt.Errorf("transfer failed: %s", result.Error)
	}
	
	// TODO: Return actual transaction receipt
	return &TransactionReceipt{
		Status:  1,
		GasUsed: result.GasUsed,
		Logs:    result.Logs,
	}, nil
}