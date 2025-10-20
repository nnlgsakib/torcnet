package vm

import (
	"errors"
	"fmt"
	"math/big"
)

const (
	// StackSize is the maximum size of the VM stack
	StackSize = 1024
	// MemorySize is the initial memory size
	MemorySize = 32
	// MaxMemorySize is the maximum memory size
	MaxMemorySize = 1024 * 1024 * 32 // 32MB
)

var (
	ErrStackOverflow     = errors.New("stack overflow")
	ErrStackUnderflow    = errors.New("stack underflow")
	ErrInvalidJump       = errors.New("invalid jump destination")
	ErrInvalidOpCode     = errors.New("invalid opcode")
	ErrOutOfGas          = errors.New("out of gas")
	ErrMemoryOutOfBounds = errors.New("memory out of bounds")
	ErrExecutionReverted = errors.New("execution reverted")
)

// Stack represents the EVM stack
type Stack struct {
	data []*big.Int
	ptr  int
}

// NewStack creates a new stack
func NewStack() *Stack {
	return &Stack{
		data: make([]*big.Int, StackSize),
		ptr:  0,
	}
}

// Push pushes a value onto the stack
func (s *Stack) Push(val *big.Int) error {
	if s.ptr >= StackSize {
		return ErrStackOverflow
	}
	s.data[s.ptr] = new(big.Int).Set(val)
	s.ptr++
	return nil
}

// Pop pops a value from the stack
func (s *Stack) Pop() (*big.Int, error) {
	if s.ptr <= 0 {
		return nil, ErrStackUnderflow
	}
	s.ptr--
	val := s.data[s.ptr]
	s.data[s.ptr] = nil
	return val, nil
}

// Peek returns the top value without removing it
func (s *Stack) Peek() (*big.Int, error) {
	if s.ptr <= 0 {
		return nil, ErrStackUnderflow
	}
	return new(big.Int).Set(s.data[s.ptr-1]), nil
}

// Len returns the current stack length
func (s *Stack) Len() int {
	return s.ptr
}

// Dup duplicates the nth item on the stack
func (s *Stack) Dup(n int) error {
	if s.ptr < n {
		return ErrStackUnderflow
	}
	if s.ptr >= StackSize {
		return ErrStackOverflow
	}
	val := new(big.Int).Set(s.data[s.ptr-n])
	return s.Push(val)
}

// Swap swaps the top item with the nth item
func (s *Stack) Swap(n int) error {
	if s.ptr < n+1 {
		return ErrStackUnderflow
	}
	s.data[s.ptr-1], s.data[s.ptr-n-1] = s.data[s.ptr-n-1], s.data[s.ptr-1]
	return nil
}

// Memory represents the EVM memory
type Memory struct {
	data []byte
}

// NewMemory creates a new memory instance
func NewMemory() *Memory {
	return &Memory{
		data: make([]byte, 0, MemorySize),
	}
}

// Resize resizes the memory to the given size
func (m *Memory) Resize(size uint64) error {
	if size > MaxMemorySize {
		return ErrMemoryOutOfBounds
	}
	if uint64(len(m.data)) < size {
		m.data = append(m.data, make([]byte, size-uint64(len(m.data)))...)
	}
	return nil
}

// Get returns a slice of memory
func (m *Memory) Get(offset, size uint64) ([]byte, error) {
	if err := m.Resize(offset + size); err != nil {
		return nil, err
	}
	return m.data[offset : offset+size], nil
}

// Set sets memory at the given offset
func (m *Memory) Set(offset uint64, data []byte) error {
	if err := m.Resize(offset + uint64(len(data))); err != nil {
		return err
	}
	copy(m.data[offset:], data)
	return nil
}

// Len returns the current memory size
func (m *Memory) Len() int {
	return len(m.data)
}

// Contract represents a smart contract
type Contract struct {
	Address  []byte
	Code     []byte
	CodeHash []byte
	Input    []byte
	Gas      uint64
	Value    *big.Int
	Caller   []byte
}

// NewContract creates a new contract
func NewContract(caller, address []byte, value *big.Int, gas uint64) *Contract {
	return &Contract{
		Address: address,
		Caller:  caller,
		Value:   value,
		Gas:     gas,
	}
}

// SetCode sets the contract code
func (c *Contract) SetCode(code []byte) {
	c.Code = code
}

// SetInput sets the contract input data
func (c *Contract) SetInput(input []byte) {
	c.Input = input
}

// EVM represents the Ethereum Virtual Machine
type EVM struct {
	stack    *Stack
	memory   *Memory
	contract *Contract
	pc       uint64
	gas      uint64
	stopped  bool
	err      error
	
	// State interface for blockchain interaction
	StateDB StateDB
	
	// Block context
	BlockNumber *big.Int
	Time        *big.Int
	Difficulty  *big.Int
	GasLimit    uint64
	Coinbase    []byte
	ChainID     *big.Int
}

// StateDB interface for blockchain state operations
type StateDB interface {
	GetBalance(addr []byte) *big.Int
	SetBalance(addr []byte, amount *big.Int)
	GetNonce(addr []byte) uint64
	SetNonce(addr []byte, nonce uint64)
	GetCode(addr []byte) []byte
	SetCode(addr []byte, code []byte)
	GetCodeHash(addr []byte) []byte
	GetState(addr []byte, key []byte) []byte
	SetState(addr []byte, key []byte, value []byte)
	Exist(addr []byte) bool
	Empty(addr []byte) bool
	CreateAccount(addr []byte)
	Suicide(addr []byte) bool
	HasSuicided(addr []byte) bool
	AddLog(addr []byte, topics [][]byte, data []byte)
	GetLogs() []Log
	Snapshot() int
	RevertToSnapshot(id int)
}

// NewEVM creates a new EVM instance
func NewEVM(stateDB StateDB, blockNumber, time, difficulty *big.Int, gasLimit uint64, coinbase []byte, chainID *big.Int) *EVM {
	return &EVM{
		stack:       NewStack(),
		memory:      NewMemory(),
		StateDB:     stateDB,
		BlockNumber: blockNumber,
		Time:        time,
		Difficulty:  difficulty,
		GasLimit:    gasLimit,
		Coinbase:    coinbase,
		ChainID:     chainID,
	}
}

// Call executes a contract call
func (evm *EVM) Call(caller, addr []byte, input []byte, gas uint64, value *big.Int) ([]byte, uint64, error) {
	contract := NewContract(caller, addr, value, gas)
	contract.SetInput(input)
	
	// Get contract code from state
	code := evm.StateDB.GetCode(addr)
	if len(code) == 0 {
		return nil, gas, nil
	}
	
	contract.SetCode(code)
	evm.contract = contract
	evm.gas = gas
	evm.pc = 0
	evm.stopped = false
	evm.err = nil
	
	// Execute the contract
	ret, err := evm.Run()
	
	return ret, evm.gas, err
}

// Create executes a contract creation
func (evm *EVM) Create(caller []byte, code []byte, gas uint64, value *big.Int) ([]byte, []byte, uint64, error) {
	// Generate contract address (simplified)
	addr := make([]byte, 20)
	copy(addr, caller[:20])
	
	contract := NewContract(caller, addr, value, gas)
	contract.SetCode(code)
	
	evm.contract = contract
	evm.gas = gas
	evm.pc = 0
	evm.stopped = false
	evm.err = nil
	
	// Execute constructor
	ret, err := evm.Run()
	if err != nil {
		return nil, nil, evm.gas, err
	}
	
	// Store the contract code
	evm.StateDB.SetCode(addr, ret)
	
	return ret, addr, evm.gas, nil
}

// Run executes the contract code
func (evm *EVM) Run() ([]byte, error) {
	var ret []byte
	
	for !evm.stopped && evm.pc < uint64(len(evm.contract.Code)) {
		if evm.gas <= 0 {
			return nil, ErrOutOfGas
		}
		
		opcode := OpCode(evm.contract.Code[evm.pc])
		
		// Execute the opcode
		result, err := evm.executeOpcode(opcode)
		if err != nil {
			return nil, err
		}
		
		if result != nil {
			ret = result
		}
		
		evm.pc++
	}
	
	return ret, evm.err
}

// executeOpcode executes a single opcode
func (evm *EVM) executeOpcode(opcode OpCode) ([]byte, error) {
	// Gas cost calculation (simplified)
	gasCost := evm.getGasCost(opcode)
	if evm.gas < gasCost {
		return nil, ErrOutOfGas
	}
	evm.gas -= gasCost
	
	switch opcode {
	case STOP:
		evm.stopped = true
		return nil, nil
	case RETURN:
		return evm.opReturn()
	case REVERT:
		return evm.opRevert()
	default:
		return evm.executeArithmeticOpcode(opcode)
	}
}

// getGasCost returns the gas cost for an opcode
func (evm *EVM) getGasCost(opcode OpCode) uint64 {
	// Simplified gas costs
	switch opcode {
	case STOP:
		return 0
	case ADD, SUB, MUL, DIV, MOD:
		return 3
	case SSTORE:
		return 20000
	case SLOAD:
		return 800
	default:
		return 3
	}
}

// opReturn handles the RETURN opcode
func (evm *EVM) opReturn() ([]byte, error) {
	offset, err := evm.stack.Pop()
	if err != nil {
		return nil, err
	}
	size, err := evm.stack.Pop()
	if err != nil {
		return nil, err
	}
	
	data, err := evm.memory.Get(offset.Uint64(), size.Uint64())
	if err != nil {
		return nil, err
	}
	
	evm.stopped = true
	return data, nil
}

// opRevert handles the REVERT opcode
func (evm *EVM) opRevert() ([]byte, error) {
	offset, err := evm.stack.Pop()
	if err != nil {
		return nil, err
	}
	size, err := evm.stack.Pop()
	if err != nil {
		return nil, err
	}
	
	data, err := evm.memory.Get(offset.Uint64(), size.Uint64())
	if err != nil {
		return nil, err
	}
	
	evm.stopped = true
	return data, ErrExecutionReverted
}

// executeArithmeticOpcode executes arithmetic opcodes
func (evm *EVM) executeArithmeticOpcode(opcode OpCode) ([]byte, error) {
	switch opcode {
	case ADD:
		return nil, evm.opAdd()
	case SUB:
		return nil, evm.opSub()
	case MUL:
		return nil, evm.opMul()
	case DIV:
		return nil, evm.opDiv()
	case MOD:
		return nil, evm.opMod()
	default:
		return nil, fmt.Errorf("%w: %s", ErrInvalidOpCode, opcode.String())
	}
}

// Arithmetic operations
func (evm *EVM) opAdd() error {
	a, err := evm.stack.Pop()
	if err != nil {
		return err
	}
	b, err := evm.stack.Pop()
	if err != nil {
		return err
	}
	result := new(big.Int).Add(a, b)
	return evm.stack.Push(result)
}

func (evm *EVM) opSub() error {
	a, err := evm.stack.Pop()
	if err != nil {
		return err
	}
	b, err := evm.stack.Pop()
	if err != nil {
		return err
	}
	result := new(big.Int).Sub(a, b)
	return evm.stack.Push(result)
}

func (evm *EVM) opMul() error {
	a, err := evm.stack.Pop()
	if err != nil {
		return err
	}
	b, err := evm.stack.Pop()
	if err != nil {
		return err
	}
	result := new(big.Int).Mul(a, b)
	return evm.stack.Push(result)
}

func (evm *EVM) opDiv() error {
	a, err := evm.stack.Pop()
	if err != nil {
		return err
	}
	b, err := evm.stack.Pop()
	if err != nil {
		return err
	}
	if b.Sign() == 0 {
		return evm.stack.Push(big.NewInt(0))
	}
	result := new(big.Int).Div(a, b)
	return evm.stack.Push(result)
}

func (evm *EVM) opMod() error {
	a, err := evm.stack.Pop()
	if err != nil {
		return err
	}
	b, err := evm.stack.Pop()
	if err != nil {
		return err
	}
	if b.Sign() == 0 {
		return evm.stack.Push(big.NewInt(0))
	}
	result := new(big.Int).Mod(a, b)
	return evm.stack.Push(result)
}