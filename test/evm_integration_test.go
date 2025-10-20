package test

import (
	"encoding/hex"
	"math/big"
	"strings"
	"testing"

	"github.com/torcnet/torcnet/pkg/core"
	"github.com/torcnet/torcnet/pkg/vm"
)

func TestEVMIntegration(t *testing.T) {
	// Create a new blockchain instance
	blockchain := core.NewBlockchain("test_db", "test_node")
	// Note: Blockchain doesn't have a Close method, so we remove the defer call

	// Test basic EVM functionality
	t.Run("TestBasicTransaction", func(t *testing.T) {
		testBasicTransaction(t, blockchain)
	})

	t.Run("TestContractDeployment", func(t *testing.T) {
		testContractDeployment(t, blockchain)
	})

	t.Run("TestContractCall", func(t *testing.T) {
		testContractCall(t, blockchain)
	})

	t.Run("TestGasCalculation", func(t *testing.T) {
		testGasCalculation(t, blockchain)
	})
}

func testBasicTransaction(t *testing.T, blockchain *core.Blockchain) {
	// Convert string addresses to byte arrays
	fromBytes, _ := hex.DecodeString(strings.TrimPrefix("0x1234567890123456789012345678901234567890", "0x"))
	toBytes, _ := hex.DecodeString(strings.TrimPrefix("0x0987654321098765432109876543210987654321", "0x"))
	
	// Set up initial balance for the sender account
	initialBalance := new(big.Int)
	initialBalance.SetString("10000000000000000000", 10) // 10 ETH in wei
	blockchain.EVMIntegration.GetStateDB().SetBalance(fromBytes, initialBalance)
	
	// Create a simple value transfer transaction
	tx := &vm.EVMTransaction{
		From:     fromBytes,
		To:       toBytes,
		Value:    big.NewInt(1000000000000000000), // 1 ETH in wei
		Gas:      21000,
		GasPrice: big.NewInt(20000000000), // 20 Gwei
		Data:     []byte{},
		Nonce:    0,
	}

	// Process the transaction
	receipt, err := blockchain.ProcessEVMTransaction(tx)
	if err != nil {
		t.Fatalf("Failed to process transaction: %v", err)
	}

	// Verify transaction was processed
	if receipt.Status != 1 {
		t.Errorf("Expected transaction status 1, got %d", receipt.Status)
	}

	if receipt.GasUsed == 0 {
		t.Error("Expected gas to be used")
	}
}

func testContractDeployment(t *testing.T, blockchain *core.Blockchain) {
	// Simple contract bytecode (constructor that stores a value)
	contractBytecode := []byte{
		0x60, 0x80, 0x60, 0x40, 0x52, 0x34, 0x80, 0x15,
		0x61, 0x00, 0x10, 0x57, 0x60, 0x00, 0x80, 0xfd,
		0x5b, 0x50, 0x60, 0x40, 0x51, 0x80, 0x82, 0x01,
		0x90, 0x50, 0x50, 0x60, 0x40, 0x51, 0x80, 0x91,
		0x03, 0x90, 0xf3,
	}

	// Convert string address to byte array
	fromBytes, _ := hex.DecodeString(strings.TrimPrefix("0x1234567890123456789012345678901234567890", "0x"))
	
	// Set up initial balance for the sender account
	initialBalance := new(big.Int)
	initialBalance.SetString("10000000000000000000", 10) // 10 ETH in wei
	blockchain.EVMIntegration.GetStateDB().SetBalance(fromBytes, initialBalance)
	
	tx := &vm.EVMTransaction{
		From:     fromBytes,
		To:       []byte{}, // Empty address for contract creation
		Value:    big.NewInt(0),
		Gas:      200000,
		GasPrice: big.NewInt(20000000000),
		Data:     contractBytecode,
		Nonce:    1,
	}

	receipt, err := blockchain.ProcessEVMTransaction(tx)
	if err != nil {
		t.Fatalf("Failed to deploy contract: %v", err)
	}

	if receipt.Status != 1 {
		t.Errorf("Expected contract deployment status 1, got %d", receipt.Status)
	}

	if len(receipt.ContractAddress) == 0 {
		t.Error("Expected contract address to be set")
	}
}

func testContractCall(t *testing.T, blockchain *core.Blockchain) {
	// First deploy a contract
	contractBytecode := []byte{
		0x60, 0x80, 0x60, 0x40, 0x52, 0x34, 0x80, 0x15,
		0x61, 0x00, 0x10, 0x57, 0x60, 0x00, 0x80, 0xfd,
		0x5b, 0x50, 0x60, 0x40, 0x51, 0x80, 0x82, 0x01,
		0x90, 0x50, 0x50, 0x60, 0x40, 0x51, 0x80, 0x91,
		0x03, 0x90, 0xf3,
	}

	// Convert string address to byte array
	fromBytes, _ := hex.DecodeString(strings.TrimPrefix("0x1234567890123456789012345678901234567890", "0x"))

	// Set up initial balance for the sender account
	initialBalance := new(big.Int)
	initialBalance.SetString("10000000000000000000", 10) // 10 ETH in wei
	blockchain.EVMIntegration.GetStateDB().SetBalance(fromBytes, initialBalance)

	deployTx := &vm.EVMTransaction{
		From:     fromBytes,
		To:       []byte{},
		Value:    big.NewInt(0),
		Gas:      200000,
		GasPrice: big.NewInt(20000000000),
		Data:     contractBytecode,
		Nonce:    2,
	}

	deployReceipt, err := blockchain.ProcessEVMTransaction(deployTx)
	if err != nil {
		t.Fatalf("Failed to deploy contract: %v", err)
	}

	// Now call the contract
	callData := []byte{0x60, 0xfe, 0x47, 0xb1} // Function selector for get()

	callTx := &vm.EVMTransaction{
		From:     fromBytes,
		To:       deployReceipt.ContractAddress,
		Value:    big.NewInt(0),
		Gas:      100000,
		GasPrice: big.NewInt(20000000000),
		Data:     callData,
		Nonce:    3,
	}

	callReceipt, err := blockchain.ProcessEVMTransaction(callTx)
	if err != nil {
		t.Fatalf("Failed to call contract: %v", err)
	}

	if callReceipt.Status != 1 {
		t.Errorf("Expected contract call status 1, got %d", callReceipt.Status)
	}
}

func testGasCalculation(t *testing.T, blockchain *core.Blockchain) {
	// Convert string addresses to byte arrays
	fromBytes, _ := hex.DecodeString(strings.TrimPrefix("0x1234567890123456789012345678901234567890", "0x"))
	toBytes, _ := hex.DecodeString(strings.TrimPrefix("0x0987654321098765432109876543210987654321", "0x"))
	
	// Set up initial balance for the sender account
	initialBalance := new(big.Int)
	initialBalance.SetString("10000000000000000000", 10) // 10 ETH in wei
	blockchain.EVMIntegration.GetStateDB().SetBalance(fromBytes, initialBalance)
	
	tx := &vm.EVMTransaction{
		From:     fromBytes,
		To:       toBytes,
		Value:    big.NewInt(1000000000000000000),
		Gas:      21000,
		GasPrice: big.NewInt(20000000000),
		Data:     []byte{},
		Nonce:    4,
	}

	// Test gas estimation
	estimatedGas, err := blockchain.EstimateGas(tx)
	if err != nil {
		t.Fatalf("Failed to estimate gas: %v", err)
	}

	if estimatedGas == 0 {
		t.Error("Expected non-zero gas estimation")
	}

	// Gas estimation should be reasonable for a simple transfer
	if estimatedGas < 21000 || estimatedGas > 50000 {
		t.Errorf("Gas estimation seems unreasonable: %d", estimatedGas)
	}
}

func TestEVMRPCMethods(t *testing.T) {
	blockchain := core.NewBlockchain("test_db_rpc", "test_node_rpc")
	// Note: Blockchain doesn't have a Close method, so we remove the defer call

	t.Run("TestGetBalance", func(t *testing.T) {
		balance := blockchain.GetBalance("0x1234567890123456789012345678901234567890")
		
		// Initial balance should be 0
		if balance.Sign() != 0 {
			t.Errorf("Expected initial balance to be 0, got %s", balance.String())
		}
	})

	t.Run("TestGetNonce", func(t *testing.T) {
		nonce := blockchain.GetNonce("0x1234567890123456789012345678901234567890")
		
		// Initial nonce should be 0
		if nonce != 0 {
			t.Errorf("Expected initial nonce to be 0, got %d", nonce)
		}
	})

	t.Run("TestGetChainID", func(t *testing.T) {
		chainID := blockchain.GetChainID()
		if chainID.Cmp(big.NewInt(1)) != 0 {
			t.Errorf("Expected chain ID to be 1, got %s", chainID.String())
		}
	})
}