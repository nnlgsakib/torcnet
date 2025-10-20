package vm

import (
	"crypto/sha256"
	"math/big"
	"golang.org/x/crypto/sha3"
)

// Interpreter executes EVM opcodes
type Interpreter struct {
	evm *EVM
}

// NewInterpreter creates a new interpreter
func NewInterpreter(evm *EVM) *Interpreter {
	return &Interpreter{evm: evm}
}

// Run executes the contract with full opcode support
func (in *Interpreter) Run() ([]byte, error) {
	var ret []byte
	
	for !in.evm.stopped && in.evm.pc < uint64(len(in.evm.contract.Code)) {
		if in.evm.gas <= 0 {
			return nil, ErrOutOfGas
		}
		
		opcode := OpCode(in.evm.contract.Code[in.evm.pc])
		
		// Execute the opcode
		result, err := in.executeOpcode(opcode)
		if err != nil {
			return nil, err
		}
		
		if result != nil {
			ret = result
		}
		
		in.evm.pc++
	}
	
	return ret, in.evm.err
}

// executeOpcode executes a single opcode with full EVM compatibility
func (in *Interpreter) executeOpcode(opcode OpCode) ([]byte, error) {
	// Gas cost calculation
	gasCost := in.getGasCost(opcode)
	if in.evm.gas < gasCost {
		return nil, ErrOutOfGas
	}
	in.evm.gas -= gasCost
	
	switch opcode {
	// 0x0 range - arithmetic ops
	case STOP:
		return in.opStop()
	case ADD:
		return nil, in.opAdd()
	case MUL:
		return nil, in.opMul()
	case SUB:
		return nil, in.opSub()
	case DIV:
		return nil, in.opDiv()
	case SDIV:
		return nil, in.opSDiv()
	case MOD:
		return nil, in.opMod()
	case SMOD:
		return nil, in.opSMod()
	case ADDMOD:
		return nil, in.opAddMod()
	case MULMOD:
		return nil, in.opMulMod()
	case EXP:
		return nil, in.opExp()
	case SIGNEXTEND:
		return nil, in.opSignExtend()
		
	// 0x10 range - comparison ops
	case LT:
		return nil, in.opLt()
	case GT:
		return nil, in.opGt()
	case SLT:
		return nil, in.opSlt()
	case SGT:
		return nil, in.opSgt()
	case EQ:
		return nil, in.opEq()
	case ISZERO:
		return nil, in.opIsZero()
	case AND:
		return nil, in.opAnd()
	case OR:
		return nil, in.opOr()
	case XOR:
		return nil, in.opXor()
	case NOT:
		return nil, in.opNot()
	case BYTE:
		return nil, in.opByte()
	case SHL:
		return nil, in.opShl()
	case SHR:
		return nil, in.opShr()
	case SAR:
		return nil, in.opSar()
		
	// 0x20 range - crypto
	case KECCAK256:
		return nil, in.opKeccak256()
		
	// 0x30 range - closure state
	case ADDRESS:
		return nil, in.opAddress()
	case BALANCE:
		return nil, in.opBalance()
	case ORIGIN:
		return nil, in.opOrigin()
	case CALLER:
		return nil, in.opCaller()
	case CALLVALUE:
		return nil, in.opCallValue()
	case CALLDATALOAD:
		return nil, in.opCallDataLoad()
	case CALLDATASIZE:
		return nil, in.opCallDataSize()
	case CALLDATACOPY:
		return nil, in.opCallDataCopy()
	case CODESIZE:
		return nil, in.opCodeSize()
	case CODECOPY:
		return nil, in.opCodeCopy()
	case GASPRICE:
		return nil, in.opGasPrice()
	case EXTCODESIZE:
		return nil, in.opExtCodeSize()
	case EXTCODECOPY:
		return nil, in.opExtCodeCopy()
	case RETURNDATASIZE:
		return nil, in.opReturnDataSize()
	case RETURNDATACOPY:
		return nil, in.opReturnDataCopy()
	case EXTCODEHASH:
		return nil, in.opExtCodeHash()
		
	// 0x40 range - block operations
	case BLOCKHASH:
		return nil, in.opBlockHash()
	case COINBASE:
		return nil, in.opCoinbase()
	case TIMESTAMP:
		return nil, in.opTimestamp()
	case NUMBER:
		return nil, in.opNumber()
	case DIFFICULTY:
		return nil, in.opDifficulty()
	case GASLIMIT:
		return nil, in.opGasLimit()
	case CHAINID:
		return nil, in.opChainID()
	case SELFBALANCE:
		return nil, in.opSelfBalance()
	case BASEFEE:
		return nil, in.opBaseFee()
		
	// 0x50 range - storage and execution
	case POP:
		return nil, in.opPop()
	case MLOAD:
		return nil, in.opMLoad()
	case MSTORE:
		return nil, in.opMStore()
	case MSTORE8:
		return nil, in.opMStore8()
	case SLOAD:
		return nil, in.opSLoad()
	case SSTORE:
		return nil, in.opSStore()
	case JUMP:
		return nil, in.opJump()
	case JUMPI:
		return nil, in.opJumpI()
	case PC:
		return nil, in.opPC()
	case MSIZE:
		return nil, in.opMSize()
	case GAS:
		return nil, in.opGas()
	case JUMPDEST:
		return nil, in.opJumpDest()
	case PUSH0:
		return nil, in.opPush0()
		
	// 0x60-0x7f range - push operations
	case PUSH1, PUSH2, PUSH3, PUSH4, PUSH5, PUSH6, PUSH7, PUSH8,
		 PUSH9, PUSH10, PUSH11, PUSH12, PUSH13, PUSH14, PUSH15, PUSH16,
		 PUSH17, PUSH18, PUSH19, PUSH20, PUSH21, PUSH22, PUSH23, PUSH24,
		 PUSH25, PUSH26, PUSH27, PUSH28, PUSH29, PUSH30, PUSH31, PUSH32:
		return nil, in.opPush(int(opcode - PUSH1 + 1))
		
	// 0x80-0x8f range - dup operations
	case DUP1, DUP2, DUP3, DUP4, DUP5, DUP6, DUP7, DUP8,
		 DUP9, DUP10, DUP11, DUP12, DUP13, DUP14, DUP15, DUP16:
		return nil, in.opDup(int(opcode - DUP1 + 1))
		
	// 0x90-0x9f range - swap operations
	case SWAP1, SWAP2, SWAP3, SWAP4, SWAP5, SWAP6, SWAP7, SWAP8,
		 SWAP9, SWAP10, SWAP11, SWAP12, SWAP13, SWAP14, SWAP15, SWAP16:
		return nil, in.opSwap(int(opcode - SWAP1 + 1))
		
	// 0xa0-0xa4 range - log operations
	case LOG0, LOG1, LOG2, LOG3, LOG4:
		return nil, in.opLog(int(opcode - LOG0))
		
	// 0xf0 range - closures
	case CREATE:
		return nil, in.opCreate()
	case CALL:
		return nil, in.opCall()
	case CALLCODE:
		return nil, in.opCallCode()
	case RETURN:
		return in.opReturn()
	case DELEGATECALL:
		return nil, in.opDelegateCall()
	case CREATE2:
		return nil, in.opCreate2()
	case STATICCALL:
		return nil, in.opStaticCall()
	case REVERT:
		return in.opRevert()
	case SELFDESTRUCT:
		return nil, in.opSelfDestruct()
		
	default:
		return nil, ErrInvalidOpCode
	}
}

// Gas cost calculation
func (in *Interpreter) getGasCost(opcode OpCode) uint64 {
	switch opcode {
	case STOP:
		return 0
	case ADD, SUB, LT, GT, SLT, SGT, EQ, ISZERO, AND, OR, XOR, NOT, BYTE, CALLDATALOAD, CALLER, CALLVALUE, CALLDATASIZE, CODESIZE, GASPRICE, COINBASE, TIMESTAMP, NUMBER, DIFFICULTY, GASLIMIT, CHAINID, SELFBALANCE, POP, PC, MSIZE, GAS:
		return 3
	case MUL, DIV, SDIV, MOD, SMOD, SIGNEXTEND:
		return 5
	case ADDMOD, MULMOD:
		return 8
	case EXP:
		return 10
	case SHL, SHR, SAR:
		return 3
	case KECCAK256:
		return 30
	case BALANCE, EXTCODESIZE, EXTCODEHASH:
		return 700
	case BLOCKHASH:
		return 20
	case MLOAD, MSTORE, MSTORE8:
		return 3
	case SLOAD:
		return 800
	case SSTORE:
		return 20000
	case JUMP:
		return 8
	case JUMPI:
		return 10
	case JUMPDEST:
		return 1
	case PUSH0:
		return 2
	case PUSH1, PUSH2, PUSH3, PUSH4, PUSH5, PUSH6, PUSH7, PUSH8, PUSH9, PUSH10, PUSH11, PUSH12, PUSH13, PUSH14, PUSH15, PUSH16, PUSH17, PUSH18, PUSH19, PUSH20, PUSH21, PUSH22, PUSH23, PUSH24, PUSH25, PUSH26, PUSH27, PUSH28, PUSH29, PUSH30, PUSH31, PUSH32:
		return 3
	case DUP1, DUP2, DUP3, DUP4, DUP5, DUP6, DUP7, DUP8, DUP9, DUP10, DUP11, DUP12, DUP13, DUP14, DUP15, DUP16:
		return 3
	case SWAP1, SWAP2, SWAP3, SWAP4, SWAP5, SWAP6, SWAP7, SWAP8, SWAP9, SWAP10, SWAP11, SWAP12, SWAP13, SWAP14, SWAP15, SWAP16:
		return 3
	case LOG0:
		return 375
	case LOG1, LOG2, LOG3, LOG4:
		return 375 + 375*uint64(opcode-LOG0)
	case CREATE:
		return 32000
	case CALL, CALLCODE, DELEGATECALL, STATICCALL:
		return 700
	case CREATE2:
		return 32000
	case SELFDESTRUCT:
		return 5000
	default:
		return 3
	}
}

// Arithmetic operations
func (in *Interpreter) opStop() ([]byte, error) {
	in.evm.stopped = true
	return nil, nil
}

func (in *Interpreter) opAdd() error {
	a, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	b, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	result := new(big.Int).Add(a, b)
	// Ensure 256-bit overflow
	result.And(result, maxUint256)
	return in.evm.stack.Push(result)
}

func (in *Interpreter) opMul() error {
	a, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	b, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	result := new(big.Int).Mul(a, b)
	result.And(result, maxUint256)
	return in.evm.stack.Push(result)
}

func (in *Interpreter) opSub() error {
	a, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	b, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	result := new(big.Int).Sub(a, b)
	result.And(result, maxUint256)
	return in.evm.stack.Push(result)
}

func (in *Interpreter) opDiv() error {
	a, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	b, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	if b.Sign() == 0 {
		return in.evm.stack.Push(big.NewInt(0))
	}
	result := new(big.Int).Div(a, b)
	return in.evm.stack.Push(result)
}

func (in *Interpreter) opSDiv() error {
	a, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	b, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	if b.Sign() == 0 {
		return in.evm.stack.Push(big.NewInt(0))
	}
	
	// Convert to signed
	if a.Bit(255) == 1 {
		a = new(big.Int).Sub(maxUint256Plus1, a)
		a = new(big.Int).Neg(a)
	}
	if b.Bit(255) == 1 {
		b = new(big.Int).Sub(maxUint256Plus1, b)
		b = new(big.Int).Neg(b)
	}
	
	result := new(big.Int).Div(a, b)
	
	// Convert back to unsigned
	if result.Sign() < 0 {
		result = new(big.Int).Add(maxUint256Plus1, result)
	}
	
	return in.evm.stack.Push(result)
}

func (in *Interpreter) opMod() error {
	a, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	b, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	if b.Sign() == 0 {
		return in.evm.stack.Push(big.NewInt(0))
	}
	result := new(big.Int).Mod(a, b)
	return in.evm.stack.Push(result)
}

func (in *Interpreter) opSMod() error {
	a, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	b, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	if b.Sign() == 0 {
		return in.evm.stack.Push(big.NewInt(0))
	}
	
	// Convert to signed
	if a.Bit(255) == 1 {
		a = new(big.Int).Sub(maxUint256Plus1, a)
		a = new(big.Int).Neg(a)
	}
	if b.Bit(255) == 1 {
		b = new(big.Int).Sub(maxUint256Plus1, b)
		b = new(big.Int).Neg(b)
	}
	
	result := new(big.Int).Mod(a, b)
	
	// Convert back to unsigned
	if result.Sign() < 0 {
		result = new(big.Int).Add(maxUint256Plus1, result)
	}
	
	return in.evm.stack.Push(result)
}

func (in *Interpreter) opAddMod() error {
	a, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	b, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	n, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	if n.Sign() == 0 {
		return in.evm.stack.Push(big.NewInt(0))
	}
	
	result := new(big.Int).Add(a, b)
	result.Mod(result, n)
	return in.evm.stack.Push(result)
}

func (in *Interpreter) opMulMod() error {
	a, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	b, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	n, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	if n.Sign() == 0 {
		return in.evm.stack.Push(big.NewInt(0))
	}
	
	result := new(big.Int).Mul(a, b)
	result.Mod(result, n)
	return in.evm.stack.Push(result)
}

func (in *Interpreter) opExp() error {
	a, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	b, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	result := new(big.Int).Exp(a, b, maxUint256Plus1)
	return in.evm.stack.Push(result)
}

func (in *Interpreter) opSignExtend() error {
	back, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	num, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	if back.Cmp(big.NewInt(31)) >= 0 {
		return in.evm.stack.Push(num)
	}
	
	bit := uint(back.Uint64()*8 + 7)
	mask := new(big.Int).Lsh(big.NewInt(1), bit)
	mask.Sub(mask, big.NewInt(1))
	
	if num.Bit(int(bit)) == 1 {
		num.Or(num, new(big.Int).Xor(maxUint256, mask))
	} else {
		num.And(num, mask)
	}
	
	return in.evm.stack.Push(num)
}

// Comparison operations
func (in *Interpreter) opLt() error {
	a, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	b, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	if a.Cmp(b) < 0 {
		return in.evm.stack.Push(big.NewInt(1))
	}
	return in.evm.stack.Push(big.NewInt(0))
}

func (in *Interpreter) opGt() error {
	a, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	b, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	if a.Cmp(b) > 0 {
		return in.evm.stack.Push(big.NewInt(1))
	}
	return in.evm.stack.Push(big.NewInt(0))
}

func (in *Interpreter) opSlt() error {
	a, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	b, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	// Convert to signed
	if a.Bit(255) == 1 {
		a = new(big.Int).Sub(a, maxUint256Plus1)
	}
	if b.Bit(255) == 1 {
		b = new(big.Int).Sub(b, maxUint256Plus1)
	}
	
	if a.Cmp(b) < 0 {
		return in.evm.stack.Push(big.NewInt(1))
	}
	return in.evm.stack.Push(big.NewInt(0))
}

func (in *Interpreter) opSgt() error {
	a, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	b, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	// Convert to signed
	if a.Bit(255) == 1 {
		a = new(big.Int).Sub(a, maxUint256Plus1)
	}
	if b.Bit(255) == 1 {
		b = new(big.Int).Sub(b, maxUint256Plus1)
	}
	
	if a.Cmp(b) > 0 {
		return in.evm.stack.Push(big.NewInt(1))
	}
	return in.evm.stack.Push(big.NewInt(0))
}

func (in *Interpreter) opEq() error {
	a, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	b, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	if a.Cmp(b) == 0 {
		return in.evm.stack.Push(big.NewInt(1))
	}
	return in.evm.stack.Push(big.NewInt(0))
}

func (in *Interpreter) opIsZero() error {
	a, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	if a.Sign() == 0 {
		return in.evm.stack.Push(big.NewInt(1))
	}
	return in.evm.stack.Push(big.NewInt(0))
}

// Bitwise operations
func (in *Interpreter) opAnd() error {
	a, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	b, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	result := new(big.Int).And(a, b)
	return in.evm.stack.Push(result)
}

func (in *Interpreter) opOr() error {
	a, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	b, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	result := new(big.Int).Or(a, b)
	return in.evm.stack.Push(result)
}

func (in *Interpreter) opXor() error {
	a, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	b, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	result := new(big.Int).Xor(a, b)
	return in.evm.stack.Push(result)
}

func (in *Interpreter) opNot() error {
	a, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	result := new(big.Int).Xor(a, maxUint256)
	return in.evm.stack.Push(result)
}

func (in *Interpreter) opByte() error {
	i, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	val, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	if i.Cmp(big.NewInt(32)) >= 0 {
		return in.evm.stack.Push(big.NewInt(0))
	}
	
	// Get the byte at position i
	byteVal := val.Bytes()
	if int(i.Uint64()) >= len(byteVal) {
		return in.evm.stack.Push(big.NewInt(0))
	}
	
	// Reverse index (EVM uses big-endian)
	idx := len(byteVal) - 1 - int(i.Uint64())
	if idx < 0 {
		return in.evm.stack.Push(big.NewInt(0))
	}
	
	return in.evm.stack.Push(big.NewInt(int64(byteVal[idx])))
}

func (in *Interpreter) opShl() error {
	shift, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	value, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	if shift.Cmp(big.NewInt(256)) >= 0 {
		return in.evm.stack.Push(big.NewInt(0))
	}
	
	result := new(big.Int).Lsh(value, uint(shift.Uint64()))
	result.And(result, maxUint256)
	return in.evm.stack.Push(result)
}

func (in *Interpreter) opShr() error {
	shift, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	value, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	if shift.Cmp(big.NewInt(256)) >= 0 {
		return in.evm.stack.Push(big.NewInt(0))
	}
	
	result := new(big.Int).Rsh(value, uint(shift.Uint64()))
	return in.evm.stack.Push(result)
}

func (in *Interpreter) opSar() error {
	shift, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	value, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	if shift.Cmp(big.NewInt(256)) >= 0 {
		if value.Bit(255) == 1 {
			return in.evm.stack.Push(maxUint256)
		}
		return in.evm.stack.Push(big.NewInt(0))
	}
	
	result := new(big.Int).Rsh(value, uint(shift.Uint64()))
	
	// Sign extend if negative
	if value.Bit(255) == 1 {
		mask := new(big.Int).Lsh(maxUint256, uint(256-shift.Uint64()))
		result.Or(result, mask)
	}
	
	return in.evm.stack.Push(result)
}

// Crypto operations
func (in *Interpreter) opKeccak256() error {
	offset, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	size, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	data, err := in.evm.memory.Get(offset.Uint64(), size.Uint64())
	if err != nil {
		return err
	}
	
	hash := sha3.NewLegacyKeccak256()
	hash.Write(data)
	result := hash.Sum(nil)
	
	return in.evm.stack.Push(new(big.Int).SetBytes(result))
}

// Environmental operations
func (in *Interpreter) opAddress() error {
	return in.evm.stack.Push(new(big.Int).SetBytes(in.evm.contract.Address))
}

func (in *Interpreter) opBalance() error {
	addr, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	addrBytes := addr.Bytes()
	if len(addrBytes) > 20 {
		addrBytes = addrBytes[len(addrBytes)-20:]
	}
	
	balance := in.evm.StateDB.GetBalance(addrBytes)
	return in.evm.stack.Push(balance)
}

func (in *Interpreter) opOrigin() error {
	// For simplicity, use caller as origin
	return in.evm.stack.Push(new(big.Int).SetBytes(in.evm.contract.Caller))
}

func (in *Interpreter) opCaller() error {
	return in.evm.stack.Push(new(big.Int).SetBytes(in.evm.contract.Caller))
}

func (in *Interpreter) opCallValue() error {
	return in.evm.stack.Push(new(big.Int).Set(in.evm.contract.Value))
}

func (in *Interpreter) opCallDataLoad() error {
	offset, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	data := make([]byte, 32)
	if offset.Uint64() < uint64(len(in.evm.contract.Input)) {
		copy(data, in.evm.contract.Input[offset.Uint64():])
	}
	
	return in.evm.stack.Push(new(big.Int).SetBytes(data))
}

func (in *Interpreter) opCallDataSize() error {
	return in.evm.stack.Push(big.NewInt(int64(len(in.evm.contract.Input))))
}

func (in *Interpreter) opCallDataCopy() error {
	memOffset, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	dataOffset, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	length, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	data := make([]byte, length.Uint64())
	if dataOffset.Uint64() < uint64(len(in.evm.contract.Input)) {
		copy(data, in.evm.contract.Input[dataOffset.Uint64():])
	}
	
	return in.evm.memory.Set(memOffset.Uint64(), data)
}

func (in *Interpreter) opCodeSize() error {
	return in.evm.stack.Push(big.NewInt(int64(len(in.evm.contract.Code))))
}

func (in *Interpreter) opCodeCopy() error {
	memOffset, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	codeOffset, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	length, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	data := make([]byte, length.Uint64())
	if codeOffset.Uint64() < uint64(len(in.evm.contract.Code)) {
		copy(data, in.evm.contract.Code[codeOffset.Uint64():])
	}
	
	return in.evm.memory.Set(memOffset.Uint64(), data)
}

func (in *Interpreter) opGasPrice() error {
	// Return a default gas price
	return in.evm.stack.Push(big.NewInt(1000000000)) // 1 gwei
}

func (in *Interpreter) opExtCodeSize() error {
	addr, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	addrBytes := addr.Bytes()
	if len(addrBytes) > 20 {
		addrBytes = addrBytes[len(addrBytes)-20:]
	}
	
	code := in.evm.StateDB.GetCode(addrBytes)
	return in.evm.stack.Push(big.NewInt(int64(len(code))))
}

func (in *Interpreter) opExtCodeCopy() error {
	addr, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	memOffset, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	codeOffset, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	length, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	addrBytes := addr.Bytes()
	if len(addrBytes) > 20 {
		addrBytes = addrBytes[len(addrBytes)-20:]
	}
	
	code := in.evm.StateDB.GetCode(addrBytes)
	data := make([]byte, length.Uint64())
	if codeOffset.Uint64() < uint64(len(code)) {
		copy(data, code[codeOffset.Uint64():])
	}
	
	return in.evm.memory.Set(memOffset.Uint64(), data)
}

func (in *Interpreter) opReturnDataSize() error {
	// For simplicity, return 0
	return in.evm.stack.Push(big.NewInt(0))
}

func (in *Interpreter) opReturnDataCopy() error {
	memOffset, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	_, err = in.evm.stack.Pop() // dataOffset
	if err != nil {
		return err
	}
	length, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	// For simplicity, just zero out the memory
	data := make([]byte, length.Uint64())
	return in.evm.memory.Set(memOffset.Uint64(), data)
}

func (in *Interpreter) opExtCodeHash() error {
	addr, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	addrBytes := addr.Bytes()
	if len(addrBytes) > 20 {
		addrBytes = addrBytes[len(addrBytes)-20:]
	}
	
	codeHash := in.evm.StateDB.GetCodeHash(addrBytes)
	return in.evm.stack.Push(new(big.Int).SetBytes(codeHash))
}

// Block operations
func (in *Interpreter) opBlockHash() error {
	blockNumber, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	// For simplicity, return a hash based on block number
	hash := sha256.Sum256(blockNumber.Bytes())
	return in.evm.stack.Push(new(big.Int).SetBytes(hash[:]))
}

func (in *Interpreter) opCoinbase() error {
	return in.evm.stack.Push(new(big.Int).SetBytes(in.evm.Coinbase))
}

func (in *Interpreter) opTimestamp() error {
	return in.evm.stack.Push(new(big.Int).Set(in.evm.Time))
}

func (in *Interpreter) opNumber() error {
	return in.evm.stack.Push(new(big.Int).Set(in.evm.BlockNumber))
}

func (in *Interpreter) opDifficulty() error {
	return in.evm.stack.Push(new(big.Int).Set(in.evm.Difficulty))
}

func (in *Interpreter) opGasLimit() error {
	return in.evm.stack.Push(big.NewInt(int64(in.evm.GasLimit)))
}

func (in *Interpreter) opChainID() error {
	return in.evm.stack.Push(new(big.Int).Set(in.evm.ChainID))
}

func (in *Interpreter) opSelfBalance() error {
	balance := in.evm.StateDB.GetBalance(in.evm.contract.Address)
	return in.evm.stack.Push(balance)
}

func (in *Interpreter) opBaseFee() error {
	// Return a default base fee
	return in.evm.stack.Push(big.NewInt(1000000000)) // 1 gwei
}

// Stack operations
func (in *Interpreter) opPop() error {
	_, err := in.evm.stack.Pop()
	return err
}

// Memory operations
func (in *Interpreter) opMLoad() error {
	offset, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	data, err := in.evm.memory.Get(offset.Uint64(), 32)
	if err != nil {
		return err
	}
	
	return in.evm.stack.Push(new(big.Int).SetBytes(data))
}

func (in *Interpreter) opMStore() error {
	offset, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	val, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	data := make([]byte, 32)
	valBytes := val.Bytes()
	copy(data[32-len(valBytes):], valBytes)
	
	return in.evm.memory.Set(offset.Uint64(), data)
}

func (in *Interpreter) opMStore8() error {
	offset, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	val, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	data := []byte{byte(val.Uint64() & 0xff)}
	return in.evm.memory.Set(offset.Uint64(), data)
}

// Storage operations
func (in *Interpreter) opSLoad() error {
	key, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	keyBytes := make([]byte, 32)
	copy(keyBytes[32-len(key.Bytes()):], key.Bytes())
	
	value := in.evm.StateDB.GetState(in.evm.contract.Address, keyBytes)
	return in.evm.stack.Push(new(big.Int).SetBytes(value))
}

func (in *Interpreter) opSStore() error {
	key, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	val, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	keyBytes := make([]byte, 32)
	copy(keyBytes[32-len(key.Bytes()):], key.Bytes())
	
	valBytes := make([]byte, 32)
	copy(valBytes[32-len(val.Bytes()):], val.Bytes())
	
	in.evm.StateDB.SetState(in.evm.contract.Address, keyBytes, valBytes)
	return nil
}

// Control flow operations
func (in *Interpreter) opJump() error {
	pos, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	if !in.validJumpDest(pos.Uint64()) {
		return ErrInvalidJump
	}
	
	in.evm.pc = pos.Uint64() - 1 // -1 because pc will be incremented
	return nil
}

func (in *Interpreter) opJumpI() error {
	pos, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	cond, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	if cond.Sign() != 0 {
		if !in.validJumpDest(pos.Uint64()) {
			return ErrInvalidJump
		}
		in.evm.pc = pos.Uint64() - 1 // -1 because pc will be incremented
	}
	
	return nil
}

func (in *Interpreter) opPC() error {
	return in.evm.stack.Push(big.NewInt(int64(in.evm.pc)))
}

func (in *Interpreter) opMSize() error {
	return in.evm.stack.Push(big.NewInt(int64(in.evm.memory.Len())))
}

func (in *Interpreter) opGas() error {
	return in.evm.stack.Push(big.NewInt(int64(in.evm.gas)))
}

func (in *Interpreter) opJumpDest() error {
	// JUMPDEST is a valid jump destination, no operation needed
	return nil
}

// Push operations
func (in *Interpreter) opPush0() error {
	return in.evm.stack.Push(big.NewInt(0))
}

func (in *Interpreter) opPush(size int) error {
	if in.evm.pc+1+uint64(size) > uint64(len(in.evm.contract.Code)) {
		return ErrInvalidOpCode
	}
	
	data := in.evm.contract.Code[in.evm.pc+1 : in.evm.pc+1+uint64(size)]
	in.evm.pc += uint64(size) // Skip the pushed bytes
	
	return in.evm.stack.Push(new(big.Int).SetBytes(data))
}

// Dup operations
func (in *Interpreter) opDup(n int) error {
	return in.evm.stack.Dup(n)
}

// Swap operations
func (in *Interpreter) opSwap(n int) error {
	return in.evm.stack.Swap(n)
}

// Log operations
func (in *Interpreter) opLog(n int) error {
	offset, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	size, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	topics := make([][]byte, n)
	for i := 0; i < n; i++ {
		topic, err := in.evm.stack.Pop()
		if err != nil {
			return err
		}
		topicBytes := make([]byte, 32)
		copy(topicBytes[32-len(topic.Bytes()):], topic.Bytes())
		topics[i] = topicBytes
	}
	
	data, err := in.evm.memory.Get(offset.Uint64(), size.Uint64())
	if err != nil {
		return err
	}
	
	in.evm.StateDB.AddLog(in.evm.contract.Address, topics, data)
	return nil
}

// Contract operations
func (in *Interpreter) opCreate() error {
	value, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	offset, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	size, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	code, err := in.evm.memory.Get(offset.Uint64(), size.Uint64())
	if err != nil {
		return err
	}
	
	_, addr, _, err := in.evm.Create(in.evm.contract.Address, code, in.evm.gas/2, value)
	if err != nil {
		return in.evm.stack.Push(big.NewInt(0))
	}
	
	return in.evm.stack.Push(new(big.Int).SetBytes(addr))
}

func (in *Interpreter) opCall() error {
	gas, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	addr, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	value, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	inOffset, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	inSize, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	outOffset, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	outSize, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	input, err := in.evm.memory.Get(inOffset.Uint64(), inSize.Uint64())
	if err != nil {
		return err
	}
	
	addrBytes := addr.Bytes()
	if len(addrBytes) > 20 {
		addrBytes = addrBytes[len(addrBytes)-20:]
	}
	
	ret, _, err := in.evm.Call(in.evm.contract.Address, addrBytes, input, gas.Uint64(), value)
	if err != nil {
		return in.evm.stack.Push(big.NewInt(0))
	}
	
	// Copy return data to memory
	if len(ret) > 0 {
		copySize := outSize.Uint64()
		if uint64(len(ret)) < copySize {
			copySize = uint64(len(ret))
		}
		in.evm.memory.Set(outOffset.Uint64(), ret[:copySize])
	}
	
	return in.evm.stack.Push(big.NewInt(1))
}

func (in *Interpreter) opCallCode() error {
	// Similar to CALL but uses current contract's context
	return in.opCall()
}

func (in *Interpreter) opReturn() ([]byte, error) {
	offset, err := in.evm.stack.Pop()
	if err != nil {
		return nil, err
	}
	size, err := in.evm.stack.Pop()
	if err != nil {
		return nil, err
	}
	
	data, err := in.evm.memory.Get(offset.Uint64(), size.Uint64())
	if err != nil {
		return nil, err
	}
	
	in.evm.stopped = true
	return data, nil
}

func (in *Interpreter) opDelegateCall() error {
	// Similar to CALL but preserves caller context
	return in.opCall()
}

func (in *Interpreter) opCreate2() error {
	// Similar to CREATE but with deterministic address
	return in.opCreate()
}

func (in *Interpreter) opStaticCall() error {
	// Similar to CALL but read-only
	return in.opCall()
}

func (in *Interpreter) opRevert() ([]byte, error) {
	offset, err := in.evm.stack.Pop()
	if err != nil {
		return nil, err
	}
	size, err := in.evm.stack.Pop()
	if err != nil {
		return nil, err
	}
	
	data, err := in.evm.memory.Get(offset.Uint64(), size.Uint64())
	if err != nil {
		return nil, err
	}
	
	in.evm.stopped = true
	return data, ErrExecutionReverted
}

func (in *Interpreter) opSelfDestruct() error {
	beneficiary, err := in.evm.stack.Pop()
	if err != nil {
		return err
	}
	
	beneficiaryBytes := beneficiary.Bytes()
	if len(beneficiaryBytes) > 20 {
		beneficiaryBytes = beneficiaryBytes[len(beneficiaryBytes)-20:]
	}
	
	balance := in.evm.StateDB.GetBalance(in.evm.contract.Address)
	in.evm.StateDB.SetBalance(beneficiaryBytes, 
		new(big.Int).Add(in.evm.StateDB.GetBalance(beneficiaryBytes), balance))
	in.evm.StateDB.Suicide(in.evm.contract.Address)
	
	in.evm.stopped = true
	return nil
}

// Helper functions
func (in *Interpreter) validJumpDest(dest uint64) bool {
	if dest >= uint64(len(in.evm.contract.Code)) {
		return false
	}
	return OpCode(in.evm.contract.Code[dest]) == JUMPDEST
}

// Constants for 256-bit arithmetic
var (
	maxUint256      = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	maxUint256Plus1 = new(big.Int).Lsh(big.NewInt(1), 256)
)