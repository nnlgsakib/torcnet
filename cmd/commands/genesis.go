package commands

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/torcnet/torcnet/pkg/account"
	"github.com/torcnet/torcnet/pkg/core"
)

var (
	genesisPath      string
	genesisNetwork   string
	genesisCoinbase  string
	genesisValidator string
	genesisAlloc     string
	genesisChainID   string
)

// genesisCmd represents the genesis command
var genesisCmd = &cobra.Command{
	Use:   "genesis",
	Short: "Manage genesis configuration",
	Long:  `Create and manage genesis configuration for the TORC NET blockchain.`,
}

// genesisInitCmd represents the genesis init command
var genesisInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new genesis configuration",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Initializing genesis configuration...")

		var genesis *core.Genesis

		// Select network type
		switch genesisNetwork {
		case "mainnet":
			genesis = core.DefaultGenesisBlock()
		case "testnet":
			genesis = core.TestnetGenesisBlock()
		case "devnet":
			genesis = core.DevGenesisBlock()
		default:
			fmt.Printf("Invalid network type: %s. Using default.\n", genesisNetwork)
			genesis = core.DefaultGenesisBlock()
		}

		// Set chain ID if provided
		if genesisChainID != "" {
			genesis.SetChainID(genesisChainID)
			fmt.Printf("Chain ID set to: %s\n", genesisChainID)
		}

		// Set coinbase if provided
		if genesisCoinbase != "" {
			if err := genesis.SetCoinbase(genesisCoinbase); err != nil {
				fmt.Printf("Error setting coinbase: %v\n", err)
			} else {
				fmt.Printf("Coinbase set to: %s\n", genesisCoinbase)
			}
		} else {
			// Create a new account for coinbase if not provided
			wallet, err := account.NewWallet(getWalletPath())
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
			} else {
				fmt.Printf("Created and set coinbase to: %s\n", acc.Address)
			}
		}

		// Add validator if provided
		if genesisValidator != "" {
			validators := strings.Split(genesisValidator, ",")
			for _, validator := range validators {
				validator = strings.TrimSpace(validator)
				if err := genesis.AddValidator(validator); err != nil {
					fmt.Printf("Error adding validator %s: %v\n", validator, err)
				} else {
					fmt.Printf("Added validator: %s\n", validator)
				}
			}
		}

		// Add allocations if provided
		if genesisAlloc != "" {
			allocations := strings.Split(genesisAlloc, ",")
			for _, allocation := range allocations {
				parts := strings.Split(allocation, ":")
				if len(parts) != 2 {
					fmt.Printf("Invalid allocation format: %s\n", allocation)
					continue
				}

				address := strings.TrimSpace(parts[0])
				amountStr := strings.TrimSpace(parts[1])

				// Parse amount
				amount := new(big.Int)
				_, success := amount.SetString(amountStr, 10)
				if !success {
					fmt.Printf("Invalid amount: %s\n", amountStr)
					continue
				}

				// Add allocation
				if err := genesis.AddAllocation(address, amount); err != nil {
					fmt.Printf("Error adding allocation to %s: %v\n", address, err)
				} else {
					fmt.Printf("Added allocation: %s -> %s\n", address, amount.String())
				}
			}
		}

		// Ensure genesis directory exists
		genesisDir := filepath.Dir(genesisPath)
		if err := os.MkdirAll(genesisDir, 0755); err != nil {
			fmt.Printf("Error creating genesis directory: %v\n", err)
			return
		}

		// Save genesis configuration
		if err := genesis.ToJSON(genesisPath); err != nil {
			fmt.Printf("Error saving genesis configuration: %v\n", err)
			return
		}

		fmt.Printf("Genesis configuration saved to: %s\n", genesisPath)
	},
}

// genesisAddValidatorCmd represents the genesis add-validator command
var genesisAddValidatorCmd = &cobra.Command{
	Use:   "add-validator <address>",
	Short: "Add a validator to the genesis configuration",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		address := args[0]
		fmt.Printf("Adding validator %s to genesis configuration...\n", address)

		// Load genesis configuration
		genesis, err := core.FromJSON(genesisPath)
		if err != nil {
			fmt.Printf("Error loading genesis configuration: %v\n", err)
			return
		}

		// Add validator
		if err := genesis.AddValidator(address); err != nil {
			fmt.Printf("Error adding validator: %v\n", err)
			return
		}

		// Save genesis configuration
		if err := genesis.ToJSON(genesisPath); err != nil {
			fmt.Printf("Error saving genesis configuration: %v\n", err)
			return
		}

		fmt.Printf("Validator %s added to genesis configuration.\n", address)
	},
}

// genesisAddAllocationCmd represents the genesis add-allocation command
var genesisAddAllocationCmd = &cobra.Command{
	Use:   "add-allocation <address> <amount>",
	Short: "Add an allocation to the genesis configuration",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		address := args[0]
		amountStr := args[1]

		fmt.Printf("Adding allocation of %s to %s in genesis configuration...\n", amountStr, address)

		// Parse amount
		amount := new(big.Int)
		_, success := amount.SetString(amountStr, 10)
		if !success {
			fmt.Printf("Invalid amount: %s\n", amountStr)
			return
		}

		// Load genesis configuration
		genesis, err := core.FromJSON(genesisPath)
		if err != nil {
			fmt.Printf("Error loading genesis configuration: %v\n", err)
			return
		}

		// Add allocation
		if err := genesis.AddAllocation(address, amount); err != nil {
			fmt.Printf("Error adding allocation: %v\n", err)
			return
		}

		// Save genesis configuration
		if err := genesis.ToJSON(genesisPath); err != nil {
			fmt.Printf("Error saving genesis configuration: %v\n", err)
			return
		}

		fmt.Printf("Allocation of %s to %s added to genesis configuration.\n", amount.String(), address)
	},
}

// genesisShowCmd represents the genesis show command
var genesisShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the genesis configuration",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Showing genesis configuration...")

		// Load genesis configuration
		genesis, err := core.FromJSON(genesisPath)
		if err != nil {
			fmt.Printf("Error loading genesis configuration: %v\n", err)
			return
		}

		// Convert to JSON
		data, err := json.MarshalIndent(genesis, "", "  ")
		if err != nil {
			fmt.Printf("Error marshaling genesis configuration: %v\n", err)
			return
		}

		// Print JSON
		fmt.Println(string(data))
	},
}

func init() {
	// Add genesis command to root command
	RootCmd.AddCommand(genesisCmd)

	// Add subcommands to genesis command
	genesisCmd.AddCommand(genesisInitCmd)
	genesisCmd.AddCommand(genesisAddValidatorCmd)
	genesisCmd.AddCommand(genesisAddAllocationCmd)
	genesisCmd.AddCommand(genesisShowCmd)

	// Add flags to genesis init command
	genesisInitCmd.Flags().StringVar(&genesisPath, "output", "genesis.json", "Output file for genesis configuration")
	genesisInitCmd.Flags().StringVar(&genesisNetwork, "network", "mainnet", "Network type (mainnet, testnet, devnet)")
	genesisInitCmd.Flags().StringVar(&genesisCoinbase, "coinbase", "", "Coinbase address")
	genesisInitCmd.Flags().StringVar(&genesisValidator, "validators", "", "Comma-separated list of validator addresses")
	genesisInitCmd.Flags().StringVar(&genesisAlloc, "alloc", "", "Comma-separated list of allocations in format address:amount")
	genesisInitCmd.Flags().StringVar(&genesisChainID, "chain-id", "", "Chain ID")

	// Add flags to other genesis commands
	genesisAddValidatorCmd.Flags().StringVar(&genesisPath, "genesis", "genesis.json", "Genesis configuration file")
	genesisAddAllocationCmd.Flags().StringVar(&genesisPath, "genesis", "genesis.json", "Genesis configuration file")
	genesisShowCmd.Flags().StringVar(&genesisPath, "genesis", "genesis.json", "Genesis configuration file")
}

// GenesisCmd returns the genesis command
func GenesisCmd() *cobra.Command {
	return genesisCmd
}