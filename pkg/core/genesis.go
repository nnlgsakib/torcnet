package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"time"

	"github.com/torcnet/torcnet/pkg/account"
)

// Genesis specifies the header fields, state of a genesis block
type Genesis struct {
	Config        *ChainConfig     `json:"config"`
	Timestamp     uint64           `json:"timestamp"`
	ExtraData     []byte           `json:"extraData"`
	GasLimit      uint64           `json:"gasLimit"`
	Difficulty    *big.Int         `json:"difficulty"`
	Mixhash       []byte           `json:"mixHash"`
	Coinbase      string           `json:"coinbase"`
	Alloc         GenesisAlloc     `json:"alloc"`
	Validators    []ValidatorInfo  `json:"validators"`
	InitialHeight uint64           `json:"initialHeight"`
	ChainID       string           `json:"chainId"`
}

// ValidatorInfo contains validator address and staked amount
type ValidatorInfo struct {
	Address string   `json:"addr"`
	Stake   *big.Int `json:"staked_amount"`
}

// GenesisAlloc specifies the initial state that is part of the genesis block
type GenesisAlloc map[string]*big.Int

// ChainConfig is the core config which determines the blockchain settings
type ChainConfig struct {
	ChainID                 string   `json:"chainId"`
	Bootnodes               []string `json:"bootnodes"`
	// TORC NET specific parameters
	NanoBlockTime          uint64              `json:"nanoBlockTime"`
	MegaBlockTime          uint64              `json:"megaBlockTime"`
	NanoBlocksPerMegaBlock uint64              `json:"nanoBlocksPerMegaBlock"`
	MinValidators          uint64              `json:"minValidators"`
	MaxValidators          uint64              `json:"maxValidators"`
	ValidatorStakes        map[string]*big.Int `json:"validatorStakes"`
	RewardPerMegaBlock     *big.Int            `json:"rewardPerMegaBlock"`
}

// DefaultGenesisBlock returns the TORC NET main net genesis block
func DefaultGenesisBlock() *Genesis {
	return &Genesis{
		Config: &ChainConfig{
			ChainID:                "torcnet-1",
			Bootnodes:              []string{
				"/ip4/127.0.0.1/tcp/30301/p2p/QmVbXMtypJg3RgHMvLw3nHsxC8CuCHYwYtgvYKdvuiNsKe",
				"/ip4/127.0.0.1/tcp/30302/p2p/QmTtcVUMCEPuYsJ5fva7HGibhFDwG6Ksk62SKTXJNF6YJy",
			},
			NanoBlockTime:          1,
			MegaBlockTime:          10,
			NanoBlocksPerMegaBlock: 10,
			MinValidators:          1,
			MaxValidators:          100,
			ValidatorStakes:        make(map[string]*big.Int),
			RewardPerMegaBlock:     new(big.Int).Mul(big.NewInt(1), new(big.Int).Exp(big.NewInt(10), big.NewInt(TokenDecimals), nil)),   // 1 TRC
		},
		Timestamp:     uint64(time.Now().Unix()),
		ExtraData:     []byte("TORC NET Genesis Block"),
		GasLimit:      30000000,
		Difficulty:    big.NewInt(1),
		Mixhash:       make([]byte, 32),
		Coinbase:      "",
		Alloc:         make(GenesisAlloc),
		Validators:    []ValidatorInfo{},
		InitialHeight: 0,
		ChainID:       "torcnet-1",
	}
}

// TestnetGenesisBlock returns the TORC NET test net genesis block
func TestnetGenesisBlock() *Genesis {
	genesis := DefaultGenesisBlock()
	genesis.Config.ChainID = "torcnet-testnet-1"
	genesis.ChainID = "torcnet-testnet-1"
	return genesis
}

// DevGenesisBlock returns the TORC NET development genesis block
func DevGenesisBlock() *Genesis {
	genesis := DefaultGenesisBlock()
	genesis.Config.ChainID = "torcnet-dev-1"
	genesis.ChainID = "torcnet-dev-1"
	genesis.Config.NanoBlockTime = 1
	genesis.Config.MegaBlockTime = 10
	genesis.Config.NanoBlocksPerMegaBlock = 10
	return genesis
}

// ToJSON converts the Genesis struct to JSON
func (g *Genesis) ToJSON(filePath string) error {
	data, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(filePath, data, 0644)
}

// FromJSON loads the Genesis struct from JSON
func FromJSON(filePath string) (*Genesis, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	data, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, err
	}

	var genesis Genesis
	if err := json.Unmarshal(data, &genesis); err != nil {
		return nil, err
	}

	return &genesis, nil
}

// SetCoinbase sets the coinbase address
func (g *Genesis) SetCoinbase(address string) error {
	if !account.IsValidAddress(address) {
		return errors.New("invalid coinbase address")
	}
	g.Coinbase = address
	return nil
}

// AddValidator adds a validator to the genesis block
func (g *Genesis) AddValidator(address string) error {
	if !account.IsValidAddress(address) {
		return errors.New("invalid validator address")
	}
	
	// Check if validator already exists
	for _, v := range g.Validators {
		if v.Address == address {
			return errors.New("validator already exists")
		}
	}
	
	defaultStake := new(big.Int).Mul(big.NewInt(1000), new(big.Int).Exp(big.NewInt(10), big.NewInt(TokenDecimals), nil))
	g.Validators = append(g.Validators, ValidatorInfo{
		Address: address,
		Stake:   defaultStake,
	})
	return nil
}

// AddValidatorWithStake adds a validator with a specific stake to the genesis block
func (g *Genesis) AddValidatorWithStake(address string, stake *big.Int) error {
	if !account.IsValidAddress(address) {
		return errors.New("invalid validator address")
	}
	
	// Check if validator already exists
	for _, v := range g.Validators {
		if v.Address == address {
			return errors.New("validator already exists")
		}
	}
	
	if stake == nil || stake.Cmp(big.NewInt(0)) <= 0 {
		return errors.New("invalid stake amount")
	}
	
	g.Validators = append(g.Validators, ValidatorInfo{
		Address: address,
		Stake:   stake,
	})
	return nil
}

// RemoveValidator removes a validator from the genesis block
func (g *Genesis) RemoveValidator(address string) error {
	for i, v := range g.Validators {
		if v.Address == address {
			g.Validators = append(g.Validators[:i], g.Validators[i+1:]...)
			// Also remove from validator stakes if present
			if g.Config != nil && g.Config.ValidatorStakes != nil {
				delete(g.Config.ValidatorStakes, address)
			}
			return nil
		}
	}
	return errors.New("validator not found")
}

// AddAllocation adds an allocation to the genesis block
func (g *Genesis) AddAllocation(address string, amount *big.Int) error {
	if !account.IsValidAddress(address) {
		return errors.New("invalid address")
	}
	
	if amount == nil || amount.Cmp(big.NewInt(0)) <= 0 {
		return errors.New("invalid amount")
	}
	
	g.Alloc[address] = amount
	return nil
}

// RemoveAllocation removes an allocation from the genesis block
func (g *Genesis) RemoveAllocation(address string) error {
	if _, ok := g.Alloc[address]; !ok {
		return errors.New("allocation not found")
	}
	
	delete(g.Alloc, address)
	return nil
}

// SetChainID sets the chain ID
func (g *Genesis) SetChainID(chainID string) {
	g.ChainID = chainID
	g.Config.ChainID = chainID
}

// Commit applies the genesis block to the blockchain
func (g *Genesis) Commit(blockchain *Blockchain) error {
	// Create genesis nano block
	nanoBlock := &NanoBlock{
		Height:       g.InitialHeight,
		PrevHash:     make([]byte, 32),
		Timestamp:    time.Unix(int64(g.Timestamp), 0),
		Transactions: []Transaction{},
		NodeID:       "genesis",
		Signature:    make([]byte, 96),
		Hash:         make([]byte, 32),
	}
	
	// Calculate hash
	nanoBlock.CalculateHash()
	
	// Create genesis mega block
	validatorAddresses := make([]string, len(g.Validators))
	for i, v := range g.Validators {
		validatorAddresses[i] = v.Address
	}
	
	megaBlock := &MegaBlock{
		Height:       g.InitialHeight,
		PrevHash:     make([]byte, 32),
		Timestamp:    time.Unix(int64(g.Timestamp), 0),
		NanoBlockIDs: [][]byte{nanoBlock.Hash},
		StateRoot:    make([]byte, 32),
		ValidatorSet: validatorAddresses,
		Signatures:   make(map[string][]byte),
		Hash:         make([]byte, 32),
	}

	// Validator set already initialized
	
	// Calculate hash
	megaBlock.CalculateHash()
	
	// Initialize token state
	tokenState := NewTokenState()
	
	// Apply allocations
	for address, amount := range g.Alloc {
		if err := tokenState.Transfer(g.Coinbase, address, amount); err != nil {
			return fmt.Errorf("failed to apply allocation to %s: %v", address, err)
		}
	}
	
	// Initialize validator stakes from validator info
	if len(g.Validators) > 0 {
		if g.Config.ValidatorStakes == nil {
			g.Config.ValidatorStakes = make(map[string]*big.Int)
		}
		for _, validator := range g.Validators {
			g.Config.ValidatorStakes[validator.Address] = validator.Stake
		}
	}
	
	// Set blockchain state
	blockchain.NanoChain = append(blockchain.NanoChain, nanoBlock)
	blockchain.MegaChain = append(blockchain.MegaChain, megaBlock)
	blockchain.CurrentNanoHeight = g.InitialHeight
	blockchain.CurrentMegaHeight = g.InitialHeight
	
	return nil
}