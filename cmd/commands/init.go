package commands

import (
	"fmt"
	"math/big"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/torcnet/torcnet/pkg/account"
	"github.com/torcnet/torcnet/pkg/core"
)

var (
	initNetwork   string
	initCoinbase  string
	initValidator string
	initAlloc     string
)

// initCmd represents the init command
var initCommand = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new blockchain",
	Long:  `Initialize a new blockchain with genesis configuration.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Initializing blockchain...")

		// Create data directory if it doesn't exist
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			fmt.Printf("Error creating data directory: %v\n", err)
			return
		}

		// Create genesis configuration
		var genesis *core.Genesis
		switch initNetwork {
		case "mainnet":
			genesis = core.DefaultGenesisBlock()
		case "testnet":
			genesis = core.TestnetGenesisBlock()
		case "devnet":
			genesis = core.DevGenesisBlock()
		default:
			fmt.Printf("Invalid network type: %s. Using default.\n", initNetwork)
			genesis = core.DefaultGenesisBlock()
		}

		// Set chain ID
		genesis.SetChainID(chainID)

		// Set coinbase if provided
		if initCoinbase != "" {
			if err := genesis.SetCoinbase(initCoinbase); err != nil {
				fmt.Printf("Error setting coinbase: %v\n", err)
				return
			}
		} else {
			// Create a new account for coinbase if not provided
			walletPath := filepath.Join(dataDir, "wallet")
			wallet, err := account.NewWallet(walletPath)
			if err != nil {
				fmt.Printf("Error creating wallet: %v\n", err)
				return
			}

			// Create new account
			acc, err := wallet.CreateAccount()
			if err != nil {
				fmt.Printf("Error creating account: %v\n", err)
				return
			}

			if err := genesis.SetCoinbase(acc.Address); err != nil {
				fmt.Printf("Error setting coinbase: %v\n", err)
				return
			}

			fmt.Printf("Created and set coinbase to: %s\n", acc.Address)
			
			// Save wallet
			if err := wallet.Save(); err != nil {
				fmt.Printf("Error saving wallet: %v\n", err)
				return
			}
		}

		// Add validator if provided or use coinbase
		if initValidator != "" {
			if err := genesis.AddValidator(initValidator); err != nil {
				fmt.Printf("Error adding validator: %v\n", err)
			}
		} else if genesis.Coinbase != "" {
			// Add coinbase as validator with stake
			defaultStake := new(big.Int).Mul(big.NewInt(1000), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
			if err := genesis.AddValidatorWithStake(genesis.Coinbase, defaultStake); err != nil {
				fmt.Printf("Error adding validator: %v\n", err)
			} else {
				fmt.Printf("Added coinbase as validator: %s with stake: %s\n", genesis.Coinbase, defaultStake.String())
			}
		}

		// Save genesis configuration
		genesisPath := filepath.Join(dataDir, "genesis.json")
		if err := genesis.ToJSON(genesisPath); err != nil {
			fmt.Printf("Error saving genesis configuration: %v\n", err)
			return
		}

		// Initialize blockchain with genesis state
		blockchain := core.NewBlockchain(dataDir, dbBackend)
		if err := genesis.Commit(blockchain); err != nil {
			fmt.Printf("Error initializing blockchain: %v\n", err)
			return
		}

		fmt.Printf("Genesis configuration saved to: %s\n", genesisPath)
		fmt.Println("Blockchain initialized successfully!")
		fmt.Println("To start the node, run: torcnet start")
	},
}

func init() {
	// Add flags to init command
	initCommand.Flags().StringVar(&initNetwork, "network", "mainnet", "Network type (mainnet, testnet, devnet)")
	initCommand.Flags().StringVar(&initCoinbase, "coinbase", "", "Coinbase address")
	initCommand.Flags().StringVar(&initValidator, "validator", "", "Validator address")
	initCommand.Flags().StringVar(&initAlloc, "alloc", "", "Comma-separated list of allocations in format address:amount")
}

// InitCmd returns the init command
func InitCmd() *cobra.Command {
	return initCommand
}