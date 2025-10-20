package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/torcnet/torcnet/pkg/account"
	"github.com/torcnet/torcnet/pkg/consensus"
	"github.com/torcnet/torcnet/pkg/core"
	"github.com/torcnet/torcnet/pkg/network"
	"github.com/torcnet/torcnet/pkg/rpc"
)

var (
	// Global flags
	dbBackend   string
	isValidator bool
	p2pAddress  string
	rpcAddress  string
	chainID     string
	dataDir     string
)

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:   "torcnet",
	Short: "TORC NET - High-performance blockchain with dual-layer blocks",
	Long: `TORC NET is a high-performance blockchain with dual-layer blocks.
It features nano blocks for fast transaction processing and mega blocks for consensus.`,
}

// VersionCmd returns the version command
func VersionCmd() *cobra.Command {
	return versionCmd
}

// StartCmd returns the start command
func StartCmd() *cobra.Command {
	return startCmd
}

// AccountCmd returns the account command
func AccountCmd() *cobra.Command {
	return accountCmd
}

// Execute adds all child commands to the root command and sets flags appropriately
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	RootCmd.PersistentFlags().StringVar(&dbBackend, "db", "pebble", "Database backend (leveldb, pebble, rocksdb)")
	RootCmd.PersistentFlags().BoolVar(&isValidator, "validator", false, "Run as a validator node")
	RootCmd.PersistentFlags().StringVar(&p2pAddress, "p2p", "0.0.0.0:26656", "P2P listen address")
	RootCmd.PersistentFlags().StringVar(&rpcAddress, "rpc", "0.0.0.0:26657", "RPC listen address")
	RootCmd.PersistentFlags().StringVar(&chainID, "chain-id", "torcnet-1", "Chain ID")
	RootCmd.PersistentFlags().StringVar(&dataDir, "data-dir", defaultDataDir(), "Data directory")

	// Add commands
	RootCmd.AddCommand(versionCmd)
	RootCmd.AddCommand(startCmd)
	RootCmd.AddCommand(initCmd)
	RootCmd.AddCommand(accountCmd)
}

// defaultDataDir returns the default data directory
func defaultDataDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "./.torcnet"
	}
	return filepath.Join(homeDir, ".torcnet")
}

// initConfig reads in config file and ENV variables if set
func initConfig() {
	// Set config defaults
	viper.SetDefault("db", "pebble")
	viper.SetDefault("validator", false)
	viper.SetDefault("p2p", "0.0.0.0:26656")
	viper.SetDefault("rpc", "0.0.0.0:26657")
	viper.SetDefault("chain-id", "torcnet-1")
	viper.SetDefault("data-dir", defaultDataDir())

	// Read from config file
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("$HOME/.torcnet")
	viper.AddConfigPath(".")
	viper.AutomaticEnv()

	// If a config file is found, read it in
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("TORC NET v0.1.0")
	},
}

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the TORC NET node",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Starting TORC NET node...")
		
		// Load genesis file
		genesis, err := core.FromJSON(filepath.Join(dataDir, "genesis.json"))
		if err != nil {
			fmt.Printf("Error loading genesis file: %v\n", err)
			return
		}
		
		// Initialize blockchain
		blockchain := core.NewBlockchain(dataDir, dbBackend)
		if err := genesis.Commit(blockchain); err != nil {
			fmt.Printf("Error initializing blockchain: %v\n", err)
			return
		}
		
		// Initialize P2P node
		p2pNode := network.NewP2PNode()
		if err := p2pNode.Start(p2pAddress); err != nil {
			fmt.Printf("Error starting P2P node: %v\n", err)
			return
		}
		
		// Load wallet
		wallet, err := account.NewWallet(getWalletPath())
		if err != nil {
			fmt.Printf("Error loading wallet: %v\n", err)
			return
		}
		
		// Get validator ID if running as validator
		validatorID := ""
		if isValidator {
			if wallet.DefaultAccount == "" {
				fmt.Println("No default account set. Please set a default account to run as validator.")
				return
			}
			validatorID = wallet.DefaultAccount
		}
		
		// Initialize consensus engine
		consensusEngine := consensus.NewConsensusEngine(blockchain, isValidator, validatorID, 4, p2pNode)
		
		// Start P2P node
		if err := p2pNode.Start(p2pAddress); err != nil {
			fmt.Printf("Error starting P2P node: %v\n", err)
			return
		}
		
		// Start consensus engine if running as validator
		if isValidator {
			if err := consensusEngine.Start(); err != nil {
				fmt.Printf("Error starting consensus engine: %v\n", err)
				return
			}
		}
		
		// Start RPC server
		rpcServer := rpc.NewRPCServer(rpcAddress, blockchain, p2pNode)
		if err := rpcServer.Start(); err != nil {
			fmt.Printf("Error starting RPC server: %v\n", err)
			return
		}
		
		// Wait for interrupt signal
		fmt.Println("Node started successfully. Press Ctrl+C to stop.")
		select {}
	},
}

// initCmd represents the init command
var initCmd = &cobra.Command{
	Use:   "init [network]",
	Short: "Initialize the TORC NET node",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		network := args[0]
		fmt.Printf("Initializing TORC NET node for network: %s\n", network)
		
		// Create data directory if it doesn't exist
		if err := os.MkdirAll(dataDir, 0700); err != nil {
			fmt.Printf("Error creating data directory: %v\n", err)
			return
		}
		
		// Create or load wallet
		wallet, err := account.NewWallet(getWalletPath())
		if err != nil {
			fmt.Printf("Error opening wallet: %v\n", err)
			return
		}
		
		// Create coinbase account if not exists
		if wallet.DefaultAccount == "" {
			acc, err := wallet.CreateAccount()
			if err != nil {
				fmt.Printf("Error creating coinbase account: %v\n", err)
				return
			}
			fmt.Printf("Created coinbase account: %s\n", acc.Address)
		}
		
		// Create genesis configuration
		genesis := core.DefaultGenesisBlock()
		genesis.ChainID = chainID
		
		// Add coinbase account as validator
		if err := genesis.AddValidator(wallet.DefaultAccount); err != nil {
			fmt.Printf("Error adding validator: %v\n", err)
			return
		}
		
		// Save genesis configuration
		genesisPath := filepath.Join(dataDir, "genesis.json")
		if err := genesis.ToJSON(genesisPath); err != nil {
			fmt.Printf("Error saving genesis configuration: %v\n", err)
			return
		}
		
		fmt.Printf("Genesis configuration saved to %s\n", genesisPath)
		fmt.Println("Node initialization completed successfully.")
	},
}

// accountCmd represents the account command
var accountCmd = &cobra.Command{
	Use:   "account",
	Short: "Manage accounts",
}

func init() {
	// Add account subcommands
	accountCmd.AddCommand(accountCreateCmd)
	accountCmd.AddCommand(accountListCmd)
	accountCmd.AddCommand(accountImportCmd)
	accountCmd.AddCommand(accountExportCmd)
	accountCmd.AddCommand(accountDefaultCmd)
}

// getWalletPath returns the wallet path
func getWalletPath() string {
	return filepath.Join(dataDir, "wallet.json")
}

// accountCreateCmd represents the account create command
var accountCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new account",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Creating new account...")
		
		// Create wallet directory if it doesn't exist
		if err := os.MkdirAll(dataDir, 0700); err != nil {
			fmt.Printf("Error creating data directory: %v\n", err)
			return
		}
		
		// Create or load wallet
		wallet, err := account.NewWallet(getWalletPath())
		if err != nil {
			fmt.Printf("Error opening wallet: %v\n", err)
			return
		}
		
		// Create new account
		acc, err := wallet.CreateAccount()
		if err != nil {
			fmt.Printf("Error creating account: %v\n", err)
			return
		}
		
		fmt.Printf("Created new account: %s\n", acc.Address)
		if wallet.DefaultAccount == acc.Address {
			fmt.Println("This account is set as the default account.")
		}
	},
}

// accountListCmd represents the account list command
var accountListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all accounts",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Listing accounts...")
		
		// Create or load wallet
		wallet, err := account.NewWallet(getWalletPath())
		if err != nil {
			fmt.Printf("Error opening wallet: %v\n", err)
			return
		}
		
		// List accounts
		accounts := wallet.ListAccounts()
		if len(accounts) == 0 {
			fmt.Println("No accounts found.")
			return
		}
		
		fmt.Printf("Found %d account(s):\n", len(accounts))
		for i, addr := range accounts {
			if addr == wallet.DefaultAccount {
				fmt.Printf("%d. %s (default)\n", i+1, addr)
			} else {
				fmt.Printf("%d. %s\n", i+1, addr)
			}
		}
	},
}

// accountImportCmd represents the account import command
var accountImportCmd = &cobra.Command{
	Use:   "import <private-key>",
	Short: "Import an account from a private key",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		privateKey := args[0]
		fmt.Println("Importing account...")
		
		// Create wallet directory if it doesn't exist
		if err := os.MkdirAll(dataDir, 0700); err != nil {
			fmt.Printf("Error creating data directory: %v\n", err)
			return
		}
		
		// Create or load wallet
		wallet, err := account.NewWallet(getWalletPath())
		if err != nil {
			fmt.Printf("Error opening wallet: %v\n", err)
			return
		}
		
		// Import account
		acc, err := wallet.ImportAccount(privateKey)
		if err != nil {
			fmt.Printf("Error importing account: %v\n", err)
			return
		}
		
		fmt.Printf("Imported account: %s\n", acc.Address)
		if wallet.DefaultAccount == acc.Address {
			fmt.Println("This account is set as the default account.")
		}
	},
}

// accountExportCmd represents the account export command
var accountExportCmd = &cobra.Command{
	Use:   "export <address>",
	Short: "Export an account's private key",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		address := args[0]
		fmt.Println("Exporting account...")
		
		// Create or load wallet
		wallet, err := account.NewWallet(getWalletPath())
		if err != nil {
			fmt.Printf("Error opening wallet: %v\n", err)
			return
		}
		
		// Get account
		acc, err := wallet.GetAccount(address)
		if err != nil {
			fmt.Printf("Error getting account: %v\n", err)
			return
		}
		
		// Export private key
		privateKey := acc.ExportPrivateKeyHex()
		fmt.Printf("Private key: %s\n", privateKey)
		fmt.Println("WARNING: Never disclose your private key. Anyone with your private key can access your account.")
	},
}

// accountDefaultCmd represents the account default command
var accountDefaultCmd = &cobra.Command{
	Use:   "default <address>",
	Short: "Set the default account",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		address := args[0]
		fmt.Println("Setting default account...")
		
		// Create or load wallet
		wallet, err := account.NewWallet(getWalletPath())
		if err != nil {
			fmt.Printf("Error opening wallet: %v\n", err)
			return
		}
		
		// Set default account
		if err := wallet.SetDefaultAccount(address); err != nil {
			fmt.Printf("Error setting default account: %v\n", err)
			return
		}
		
		fmt.Printf("Set %s as the default account.\n", address)
	},
}