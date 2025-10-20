package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/torcnet/torcnet/cmd/commands"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "torcnet",
		Short: "TORC NET - Advanced dual-layer blockchain with nano and mega blocks",
		Long: `TORC NET is a next-generation blockchain platform featuring a dual-layer architecture
with nano blocks and mega blocks for enhanced scalability and performance.
It uses BLS for validator signatures, VRF for randomness, and supports parallel execution.`,
	}

	// Add subcommands
	rootCmd.AddCommand(commands.StartCmd())
	rootCmd.AddCommand(commands.VersionCmd())
	rootCmd.AddCommand(commands.InitCmd())
	rootCmd.AddCommand(commands.AccountCmd())
	rootCmd.AddCommand(commands.GenesisCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}