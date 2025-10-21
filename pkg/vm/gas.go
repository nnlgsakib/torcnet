package vm

import (
	"fmt"
	"math/big"
)

// Gas constants for EVM operations
const (
	// Base gas costs
	GasQuickStep   uint64 = 2
	GasFastestStep uint64 = 3
	GasFastStep    uint64 = 5
	GasMidStep     uint64 = 8
	GasSlowStep    uint64 = 10
	GasExtStep     uint64 = 20

	// Memory operations
	GasMemory      uint64 = 3
	GasQuadDivisor uint64 = 512

	// Storage operations
	GasSSet        uint64 = 20000
	GasSReset      uint64 = 5000
	GasSClear      uint64 = 15000
	GasRefundClear uint64 = 15000

	// Transaction costs
	GasTransaction   uint64 = 21000
	GasTxCreate      uint64 = 32000
	GasTxDataZero    uint64 = 4
	GasTxDataNonZero uint64 = 16

	// Contract operations
	GasCreate      uint64 = 32000
	GasCall        uint64 = 700
	GasCallValue   uint64 = 9000
	GasCallStipend uint64 = 2300
	GasNewAccount  uint64 = 25000
	GasExp         uint64 = 10
	GasExpByte     uint64 = 50

	// Cryptographic operations
	GasSha3          uint64 = 30
	GasSha3Word      uint64 = 6
	GasCopy          uint64 = 3
	GasEcRecover     uint64 = 3000
	GasSha256        uint64 = 60
	GasSha256Word    uint64 = 12
	GasRipemd160     uint64 = 600
	GasRipemd160Word uint64 = 120
	GasIdentity      uint64 = 15
	GasIdentityWord  uint64 = 3

	// Log operations
	GasLog      uint64 = 375
	GasLogData  uint64 = 8
	GasLogTopic uint64 = 375

	// Jump operations
	GasJumpDest uint64 = 1

	// Block operations
	GasBalance     uint64 = 700
	GasExtCode     uint64 = 700
	GasExtCodeHash uint64 = 700
	GasBlockhash   uint64 = 20

	// Maximum gas limit
	MaxGasLimit uint64 = 30000000
)

// GasCalculator handles gas calculations for EVM operations
type GasCalculator struct {
	gasPrice *big.Int
}

// NewGasCalculator creates a new gas calculator
func NewGasCalculator(gasPrice *big.Int) *GasCalculator {
	return &GasCalculator{
		gasPrice: gasPrice,
	}
}

// CalculateIntrinsicGas calculates the intrinsic gas for a transaction
func (gc *GasCalculator) CalculateIntrinsicGas(data []byte, isContractCreation bool) uint64 {
	gas := GasTransaction

	if isContractCreation {
		gas += GasTxCreate
	}

	// Add gas for transaction data
	for _, b := range data {
		if b == 0 {
			gas += GasTxDataZero
		} else {
			gas += GasTxDataNonZero
		}
	}

	return gas
}

// CalculateMemoryGas calculates gas cost for memory expansion
func (gc *GasCalculator) CalculateMemoryGas(oldSize, newSize uint64) uint64 {
	if newSize <= oldSize {
		return 0
	}

	oldCost := gc.memoryGasCost(oldSize)
	newCost := gc.memoryGasCost(newSize)

	if newCost < oldCost {
		return 0
	}

	return newCost - oldCost
}

// memoryGasCost calculates the gas cost for a given memory size
func (gc *GasCalculator) memoryGasCost(size uint64) uint64 {
	if size == 0 {
		return 0
	}

	// Memory cost = (size * GasMemory) + (size^2 / GasQuadDivisor)
	linearCost := size * GasMemory
	quadraticCost := (size * size) / GasQuadDivisor

	return linearCost + quadraticCost
}

// CalculateStorageGas calculates gas cost for storage operations
func (gc *GasCalculator) CalculateStorageGas(oldValue, newValue []byte) (uint64, int64) {
	oldIsZero := isZero(oldValue)
	newIsZero := isZero(newValue)

	var gas uint64
	var refund int64

	if oldIsZero && !newIsZero {
		// Setting storage from zero to non-zero
		gas = GasSSet
	} else if !oldIsZero && newIsZero {
		// Clearing storage (non-zero to zero)
		gas = GasSReset
		refund = int64(GasRefundClear)
	} else {
		// Modifying existing storage
		gas = GasSReset
	}

	return gas, refund
}

// CalculateCallGas calculates gas cost for contract calls
func (gc *GasCalculator) CalculateCallGas(value *big.Int, gasAvailable uint64, gasRequested uint64, accountExists bool) (uint64, error) {
	gas := GasCall

	// Add gas for value transfer
	if value != nil && value.Sign() != 0 {
		gas += GasCallValue

		// Add gas for new account creation if needed
		if !accountExists {
			gas += GasNewAccount
		}
	}

	// Ensure we don't exceed available gas
	if gas > gasAvailable {
		return 0, fmt.Errorf("insufficient gas for call")
	}

	// Calculate actual gas to send
	gasToSend := gasRequested
	if gasToSend == 0 || gasToSend > gasAvailable-gas {
		gasToSend = gasAvailable - gas
	}

	// Apply EIP-150: all but one 64th of the remaining gas
	if gasToSend > gasAvailable/64 {
		gasToSend = gasAvailable - gasAvailable/64
	}

	return gas + gasToSend, nil
}

// CalculateCreateGas calculates gas cost for contract creation
func (gc *GasCalculator) CalculateCreateGas(codeSize uint64) uint64 {
	return GasCreate + (codeSize * GasMemory)
}

// CalculateExpGas calculates gas cost for exponentiation
func (gc *GasCalculator) CalculateExpGas(exponent *big.Int) uint64 {
	if exponent.Sign() == 0 {
		return GasExp
	}

	// Calculate number of bytes in exponent
	expBytes := uint64(len(exponent.Bytes()))
	return GasExp + (expBytes * GasExpByte)
}

// CalculateSha3Gas calculates gas cost for SHA3 operations
func (gc *GasCalculator) CalculateSha3Gas(dataSize uint64) uint64 {
	words := (dataSize + 31) / 32
	return GasSha3 + (words * GasSha3Word)
}

// CalculateCopyGas calculates gas cost for copy operations
func (gc *GasCalculator) CalculateCopyGas(dataSize uint64) uint64 {
	words := (dataSize + 31) / 32
	return GasCopy * words
}

// CalculateLogGas calculates gas cost for log operations
func (gc *GasCalculator) CalculateLogGas(dataSize uint64, topicCount int) uint64 {
	gas := GasLog + (dataSize * GasLogData)
	gas += uint64(topicCount) * GasLogTopic
	return gas
}

// CalculateFee calculates the total fee for a transaction
func (gc *GasCalculator) CalculateFee(gasUsed uint64) *big.Int {
	fee := new(big.Int).SetUint64(gasUsed)
	fee.Mul(fee, gc.gasPrice)
	return fee
}

// ValidateGasLimit validates if the gas limit is within acceptable bounds
func (gc *GasCalculator) ValidateGasLimit(gasLimit uint64) error {
	if gasLimit > MaxGasLimit {
		return fmt.Errorf("gas limit %d exceeds maximum %d", gasLimit, MaxGasLimit)
	}

	if gasLimit < GasTransaction {
		return fmt.Errorf("gas limit %d below minimum transaction cost %d", gasLimit, GasTransaction)
	}

	return nil
}

// EstimateGasForOperation estimates gas for specific EVM operations
func (gc *GasCalculator) EstimateGasForOperation(opcode byte, stackSize int, memorySize uint64) (uint64, error) {
	switch opcode {
	case 0x00: // STOP
		return 0, nil
	case 0x01: // ADD
		return GasFastestStep, nil
	case 0x02: // MUL
		return GasFastStep, nil
	case 0x03: // SUB
		return GasFastestStep, nil
	case 0x04: // DIV
		return GasFastStep, nil
	case 0x05: // SDIV
		return GasFastStep, nil
	case 0x06: // MOD
		return GasFastStep, nil
	case 0x07: // SMOD
		return GasFastStep, nil
	case 0x08: // ADDMOD
		return GasMidStep, nil
	case 0x09: // MULMOD
		return GasMidStep, nil
	case 0x0a: // EXP
		// EXP gas calculation requires the exponent value
		return GasExp, nil
	case 0x10: // LT
		return GasFastestStep, nil
	case 0x11: // GT
		return GasFastestStep, nil
	case 0x12: // SLT
		return GasFastestStep, nil
	case 0x13: // SGT
		return GasFastestStep, nil
	case 0x14: // EQ
		return GasFastestStep, nil
	case 0x15: // ISZERO
		return GasFastestStep, nil
	case 0x16: // AND
		return GasFastestStep, nil
	case 0x17: // OR
		return GasFastestStep, nil
	case 0x18: // XOR
		return GasFastestStep, nil
	case 0x19: // NOT
		return GasFastestStep, nil
	case 0x1a: // BYTE
		return GasFastestStep, nil
	case 0x20: // SHA3
		return GasSha3, nil
	case 0x30: // ADDRESS
		return GasQuickStep, nil
	case 0x31: // BALANCE
		return GasBalance, nil
	case 0x32: // ORIGIN
		return GasQuickStep, nil
	case 0x33: // CALLER
		return GasQuickStep, nil
	case 0x34: // CALLVALUE
		return GasQuickStep, nil
	case 0x35: // CALLDATALOAD
		return GasFastestStep, nil
	case 0x36: // CALLDATASIZE
		return GasQuickStep, nil
	case 0x37: // CALLDATACOPY
		return GasFastestStep, nil
	case 0x38: // CODESIZE
		return GasQuickStep, nil
	case 0x39: // CODECOPY
		return GasFastestStep, nil
	case 0x3a: // GASPRICE
		return GasQuickStep, nil
	case 0x3b: // EXTCODESIZE
		return GasExtCode, nil
	case 0x3c: // EXTCODECOPY
		return GasExtCode, nil
	case 0x3f: // EXTCODEHASH
		return GasExtCodeHash, nil
	case 0x40: // BLOCKHASH
		return GasBlockhash, nil
	case 0x41: // COINBASE
		return GasQuickStep, nil
	case 0x42: // TIMESTAMP
		return GasQuickStep, nil
	case 0x43: // NUMBER
		return GasQuickStep, nil
	case 0x44: // DIFFICULTY
		return GasQuickStep, nil
	case 0x45: // GASLIMIT
		return GasQuickStep, nil
	case 0x50: // POP
		return GasQuickStep, nil
	case 0x51: // MLOAD
		return GasFastestStep, nil
	case 0x52: // MSTORE
		return GasFastestStep, nil
	case 0x53: // MSTORE8
		return GasFastestStep, nil
	case 0x54: // SLOAD
		return 0, nil // SLOAD gas is complex and depends on state
	case 0x55: // SSTORE
		return 0, nil // SSTORE gas is complex and depends on state
	case 0x56: // JUMP
		return GasMidStep, nil
	case 0x57: // JUMPI
		return GasSlowStep, nil
	case 0x58: // PC
		return GasQuickStep, nil
	case 0x59: // MSIZE
		return GasQuickStep, nil
	case 0x5a: // GAS
		return GasQuickStep, nil
	case 0x5b: // JUMPDEST
		return GasJumpDest, nil
	case 0x60, 0x61, 0x62, 0x63, 0x64, 0x65, 0x66, 0x67, 0x68, 0x69, 0x6a, 0x6b, 0x6c, 0x6d, 0x6e, 0x6f: // PUSH1-PUSH16
		return GasFastestStep, nil
	case 0x70, 0x71, 0x72, 0x73, 0x74, 0x75, 0x76, 0x77, 0x78, 0x79, 0x7a, 0x7b, 0x7c, 0x7d, 0x7e, 0x7f: // PUSH17-PUSH32
		return GasFastestStep, nil
	case 0x80, 0x81, 0x82, 0x83, 0x84, 0x85, 0x86, 0x87, 0x88, 0x89, 0x8a, 0x8b, 0x8c, 0x8d, 0x8e, 0x8f: // DUP1-DUP16
		return GasFastestStep, nil
	case 0x90, 0x91, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97, 0x98, 0x99, 0x9a, 0x9b, 0x9c, 0x9d, 0x9e, 0x9f: // SWAP1-SWAP16
		return GasFastestStep, nil
	case 0xa0: // LOG0
		return GasLog, nil
	case 0xa1: // LOG1
		return GasLog + GasLogTopic, nil
	case 0xa2: // LOG2
		return GasLog + 2*GasLogTopic, nil
	case 0xa3: // LOG3
		return GasLog + 3*GasLogTopic, nil
	case 0xa4: // LOG4
		return GasLog + 4*GasLogTopic, nil
	case 0xf0: // CREATE
		return GasCreate, nil
	case 0xf1: // CALL
		return GasCall, nil
	case 0xf2: // CALLCODE
		return GasCall, nil
	case 0xf3: // RETURN
		return 0, nil
	case 0xf4: // DELEGATECALL
		return GasCall, nil
	case 0xf5: // CREATE2
		return GasCreate, nil
	case 0xfa: // STATICCALL
		return GasCall, nil
	case 0xfd: // REVERT
		return 0, nil
	case 0xfe: // INVALID
		return 0, fmt.Errorf("invalid opcode")
	case 0xff: // SELFDESTRUCT
		return 0, nil // SELFDESTRUCT gas is complex
	default:
		return 0, fmt.Errorf("unknown opcode: 0x%02x", opcode)
	}
}

// CalculatePrecompiledGas calculates gas for precompiled contracts
func (gc *GasCalculator) CalculatePrecompiledGas(address []byte, input []byte) (uint64, error) {
	if len(address) != 20 {
		return 0, fmt.Errorf("invalid precompiled contract address")
	}

	// Check if it's a precompiled contract (addresses 1-9)
	var addr uint64
	for i := 12; i < 20; i++ {
		addr = addr*256 + uint64(address[i])
	}

	switch addr {
	case 1: // ecRecover
		return GasEcRecover, nil
	case 2: // SHA256
		words := (uint64(len(input)) + 31) / 32
		return GasSha256 + words*GasSha256Word, nil
	case 3: // RIPEMD160
		words := (uint64(len(input)) + 31) / 32
		return GasRipemd160 + words*GasRipemd160Word, nil
	case 4: // Identity
		words := (uint64(len(input)) + 31) / 32
		return GasIdentity + words*GasIdentityWord, nil
	default:
		return 0, fmt.Errorf("unknown precompiled contract at address %d", addr)
	}
}

// GasTracker tracks gas usage during execution
type GasTracker struct {
	gasLimit   uint64
	gasUsed    uint64
	gasRefund  int64
	calculator *GasCalculator
}

// NewGasTracker creates a new gas tracker
func NewGasTracker(gasLimit uint64, calculator *GasCalculator) *GasTracker {
	return &GasTracker{
		gasLimit:   gasLimit,
		gasUsed:    0,
		gasRefund:  0,
		calculator: calculator,
	}
}

// ConsumeGas consumes gas and returns error if insufficient
func (gt *GasTracker) ConsumeGas(amount uint64) error {
	if gt.gasUsed+amount > gt.gasLimit {
		return fmt.Errorf("out of gas: need %d, have %d", amount, gt.gasLimit-gt.gasUsed)
	}

	gt.gasUsed += amount
	return nil
}

// AddRefund adds gas refund
func (gt *GasTracker) AddRefund(amount int64) {
	gt.gasRefund += amount
}

// GetGasLeft returns remaining gas
func (gt *GasTracker) GetGasLeft() uint64 {
	return gt.gasLimit - gt.gasUsed
}

// GetGasUsed returns gas used
func (gt *GasTracker) GetGasUsed() uint64 {
	return gt.gasUsed
}

// GetGasRefund returns gas refund
func (gt *GasTracker) GetGasRefund() int64 {
	return gt.gasRefund
}

// FinalizeGas calculates final gas usage after refunds
func (gt *GasTracker) FinalizeGas() uint64 {
	// Apply refunds (max 50% of gas used)
	maxRefund := int64(gt.gasUsed / 2)
	actualRefund := gt.gasRefund
	if actualRefund > maxRefund {
		actualRefund = maxRefund
	}

	finalGasUsed := int64(gt.gasUsed) - actualRefund
	if finalGasUsed < 0 {
		finalGasUsed = 0
	}

	return uint64(finalGasUsed)
}

// Helper functions

func isZero(data []byte) bool {
	for _, b := range data {
		if b != 0 {
			return false
		}
	}
	return true
}

// GetOpcodeGas returns the gas cost for a specific opcode
func GetOpcodeGas(opcode byte) uint64 {
	switch opcode {
	// Arithmetic operations
	case 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07: // ADD, MUL, SUB, DIV, SDIV, MOD, SMOD
		return GasFastStep
	case 0x08, 0x09, 0x0a: // ADDMOD, MULMOD, EXP
		return GasMidStep
	case 0x0b: // SIGNEXTEND
		return GasFastStep

	// Comparison operations
	case 0x10, 0x11, 0x12, 0x13, 0x14: // LT, GT, SLT, SGT, EQ
		return GasFastStep
	case 0x15: // ISZERO
		return GasFastStep
	case 0x16, 0x17, 0x18, 0x19: // AND, OR, XOR, NOT
		return GasFastStep
	case 0x1a: // BYTE
		return GasFastStep
	case 0x1b, 0x1c, 0x1d: // SHL, SHR, SAR
		return GasFastStep

	// Cryptographic operations
	case 0x20: // SHA3
		return GasSha3

	// Environmental operations
	case 0x30: // ADDRESS
		return GasQuickStep
	case 0x31: // BALANCE
		return GasBalance
	case 0x32: // ORIGIN
		return GasQuickStep
	case 0x33: // CALLER
		return GasQuickStep
	case 0x34: // CALLVALUE
		return GasQuickStep
	case 0x35: // CALLDATALOAD
		return GasFastStep
	case 0x36: // CALLDATASIZE
		return GasQuickStep
	case 0x37: // CALLDATACOPY
		return GasFastStep
	case 0x38: // CODESIZE
		return GasQuickStep
	case 0x39: // CODECOPY
		return GasFastStep
	case 0x3a: // GASPRICE
		return GasQuickStep
	case 0x3b: // EXTCODESIZE
		return GasExtCode
	case 0x3c: // EXTCODECOPY
		return GasExtCode
	case 0x3d: // RETURNDATASIZE
		return GasQuickStep
	case 0x3e: // RETURNDATACOPY
		return GasFastStep
	case 0x3f: // EXTCODEHASH
		return GasExtCodeHash

	// Block operations
	case 0x40: // BLOCKHASH
		return GasBlockhash
	case 0x41, 0x42, 0x43, 0x44, 0x45: // COINBASE, TIMESTAMP, NUMBER, DIFFICULTY, GASLIMIT
		return GasQuickStep

	// Stack operations
	case 0x50: // POP
		return GasQuickStep
	case 0x51, 0x52: // MLOAD, MSTORE
		return GasFastStep
	case 0x53: // MSTORE8
		return GasFastStep
	case 0x54, 0x55: // SLOAD, SSTORE
		return GasSReset // Will be calculated dynamically
	case 0x56: // JUMP
		return GasMidStep
	case 0x57: // JUMPI
		return GasSlowStep
	case 0x58: // PC
		return GasQuickStep
	case 0x59: // MSIZE
		return GasQuickStep
	case 0x5a: // GAS
		return GasQuickStep
	case 0x5b: // JUMPDEST
		return GasJumpDest

	// Push operations (0x60-0x7f)
	default:
		if opcode >= 0x60 && opcode <= 0x7f {
			return GasFastestStep
		}
		// Dup operations (0x80-0x8f)
		if opcode >= 0x80 && opcode <= 0x8f {
			return GasFastStep
		}
		// Swap operations (0x90-0x9f)
		if opcode >= 0x90 && opcode <= 0x9f {
			return GasFastStep
		}
		// Log operations (0xa0-0xa4)
		if opcode >= 0xa0 && opcode <= 0xa4 {
			return GasLog
		}

		// Default case
		return GasFastStep
	}
}