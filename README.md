# TORC NET Blockchain

TORC NET is a high-performance blockchain with dual-layer blocks, featuring nano blocks for fast transaction processing and mega blocks for consensus.

## Features

- **Dual-Layer Block Architecture**: Nano blocks for fast transaction processing and mega blocks for consensus
- **Parallel Execution**: Efficient transaction processing with parallel execution
- **Multiple Database Backends**: Support for LevelDB, PebbleDB, and RocksDB
- **P2P Networking**: Built with libp2p and QUIC transport for high-performance networking
- **Advanced Cryptography**: BLS signatures, VRF for randomness, BLAKE3 for hashing, and ECDSA for accounts
- **Token Economics**: TORC NET (TRC) with 100M initial supply
- **Account Management**: Ethereum-compatible key length and wallet management

## Getting Started

### Prerequisites

- Go 1.23.8 or higher

### Installation

```bash
# Clone the repository
git clone https://github.com/torcnet/torcnet.git
cd torcnet

# Build the project
go build -o torcnet ./cmd
```

### Usage

```bash
# Initialize the blockchain
./torcnet init --chain-id torcnet-1

# Create an account
./torcnet account create

# List accounts
./torcnet account list

# Start the node
./torcnet start --validator
```

## CLI Commands

### Global Flags

- `--db`: Database backend (leveldb, pebble, rocksdb)
- `--validator`: Run as a validator node
- `--p2p`: P2P listen address
- `--rpc`: RPC listen address
- `--chain-id`: Chain ID
- `--data-dir`: Data directory

### Commands

- `version`: Print the version number
- `init`: Initialize the TORC NET node
- `start`: Start the TORC NET node
- `account`: Manage accounts
  - `create`: Create a new account
  - `list`: List all accounts
  - `import`: Import an account from a private key
  - `export`: Export an account's private key
  - `default`: Set the default account

## Architecture

### Core Components

- **Blockchain**: Manages the chain of blocks and transactions
- **Consensus**: Implements the consensus mechanism with parallel execution
- **P2P Network**: Handles communication between nodes
- **Database**: Provides storage for blockchain data
- **Account**: Manages user accounts and wallet

### Dual-Layer Blocks

- **Nano Blocks**: Fast transaction processing with minimal validation
- **Mega Blocks**: Aggregate nano blocks and provide consensus finality

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Acknowledgments

- [Go Ethereum](https://github.com/ethereum/go-ethereum)
- [libp2p](https://github.com/libp2p/go-libp2p)
- [Cobra](https://github.com/spf13/cobra)
- [Viper](https://github.com/spf13/viper)