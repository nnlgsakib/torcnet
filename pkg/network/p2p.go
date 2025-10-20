package network

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	quicTransport "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/multiformats/go-multiaddr"
	"github.com/torcnet/torcnet/pkg/core"
)

const (
	// ProtocolID is the protocol ID for TORC NET
	ProtocolID = "/torcnet/1.0.0"
	
	// BlockProtocol is the protocol for block propagation
	BlockProtocol = "/torcnet/block/1.0.0"
	
	// TxProtocol is the protocol for transaction propagation
	TxProtocol = "/torcnet/tx/1.0.0"
)

// P2PNode represents a P2P network node
type P2PNode struct {
	host       host.Host
	protocols  []protocol.ID
	peerList   []peer.AddrInfo
	ctx        context.Context
	cancelFunc context.CancelFunc
}

// NewP2PNode creates a new P2P node
func NewP2PNode() *P2PNode {
	ctx, cancel := context.WithCancel(context.Background())
	return &P2PNode{
		protocols:  []protocol.ID{protocol.ID(ProtocolID), protocol.ID(BlockProtocol), protocol.ID(TxProtocol)},
		ctx:        ctx,
		cancelFunc: cancel,
	}
}

// Start starts the P2P node
func (node *P2PNode) Start(listenAddr string) error {
	// Generate a key pair for this host
	priv, _, err := crypto.GenerateKeyPairWithReader(crypto.Ed25519, 2048, rand.Reader)
	if err != nil {
		return err
	}
	
	// Create a multiaddress
	addr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/udp/0/quic", listenAddr))
	if err != nil {
		return err
	}
	
	// Create libp2p host
	h, err := libp2p.New(
		libp2p.ListenAddrs(addr),
		libp2p.Identity(priv),
		libp2p.Transport(quicTransport.NewTransport),
		libp2p.NATPortMap(),
	)
	if err != nil {
		return err
	}
	
	node.host = h
	
	// Set stream handlers for each protocol
	for _, protocolID := range node.protocols {
		node.host.SetStreamHandler(protocolID, node.handleStream)
	}
	
	// Print node address info
	fmt.Printf("P2P node started with ID: %s\n", node.host.ID().String())
	fmt.Printf("Listening on: %s\n", node.host.Addrs())
	
	return nil
}

// Connect connects to a peer
func (node *P2PNode) Connect(peerAddr string) error {
	// Parse the multiaddress
	addr, err := multiaddr.NewMultiaddr(peerAddr)
	if err != nil {
		return err
	}
	
	// Extract the peer ID from the multiaddress
	info, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		return err
	}
	
	// Connect to the peer
	ctx, cancel := context.WithTimeout(node.ctx, 10*time.Second)
	defer cancel()
	
	if err := node.host.Connect(ctx, *info); err != nil {
		return err
	}
	
	// Add to peer list
	node.peerList = append(node.peerList, *info)
	
	fmt.Printf("Connected to peer: %s\n", info.ID.String())
	return nil
}

// Broadcast broadcasts a message to all connected peers
func (node *P2PNode) Broadcast(protocolID protocol.ID, data []byte) error {
	// Get all connected peers
	for _, peer := range node.host.Network().Peers() {
		// Open a stream to the peer
		stream, err := node.host.NewStream(node.ctx, peer, protocolID)
		if err != nil {
			fmt.Printf("Failed to open stream to peer %s: %v\n", peer.String(), err)
			continue
		}
		
		// Write the data
		_, err = stream.Write(data)
		if err != nil {
			stream.Close()
			fmt.Printf("Failed to write to peer %s: %v\n", peer.String(), err)
			continue
		}
		
		// Close the stream
		stream.Close()
	}
	
	return nil
}

// Stop stops the P2P node
func (node *P2PNode) Stop() error {
	if node.cancelFunc != nil {
		node.cancelFunc()
	}
	
	if node.host != nil {
		return node.host.Close()
	}
	
	return nil
}

// handleStream handles incoming streams
func (node *P2PNode) handleStream(stream network.Stream) {
	// Read the data
	buf := make([]byte, 1024)
	n, err := stream.Read(buf)
	if err != nil {
		fmt.Printf("Error reading from stream: %v\n", err)
		stream.Close()
		return
	}
	
	// Process the data based on the protocol
	switch stream.Protocol() {
	case protocol.ID(BlockProtocol):
		// Handle block data
		fmt.Printf("Received block data from %s\n", stream.Conn().RemotePeer().String())
		
		// Deserialize block data
		var block core.MegaBlock
		if err := json.Unmarshal(buf[:n], &block); err != nil {
			fmt.Printf("Error deserializing block: %v\n", err)
			break
		}
		
		// Validate the block
		if !node.validateBlock(&block) {
			fmt.Printf("Invalid block received from %s\n", stream.Conn().RemotePeer().String())
			break
		}
		
		// Process the block (add to blockchain)
		if err := node.processBlock(&block); err != nil {
			fmt.Printf("Error processing block: %v\n", err)
			break
		}
		
		// Forward to other peers
		node.forwardBlock(&block)
		
	case protocol.ID(TxProtocol):
		// Handle transaction data
		fmt.Printf("Received transaction data from %s\n", stream.Conn().RemotePeer().String())
		
		// Deserialize transaction data
		var tx core.Transaction
		if err := json.Unmarshal(buf[:n], &tx); err != nil {
			fmt.Printf("Error deserializing transaction: %v\n", err)
			break
		}
		
		// Validate the transaction
		if !node.validateTransaction(&tx) {
			fmt.Printf("Invalid transaction received from %s\n", stream.Conn().RemotePeer().String())
			break
		}
		
		// Process the transaction (add to mempool)
		if err := node.processTransaction(&tx); err != nil {
			fmt.Printf("Error processing transaction: %v\n", err)
			break
		}
		
		// Forward to other peers
		node.forwardTransaction(&tx)
		
	default:
		// Handle other protocols
		fmt.Printf("Received data on protocol %s from %s\n", stream.Protocol(), stream.Conn().RemotePeer().String())
	}
	
	// Close the stream
	stream.Close()
}

// Helper functions for block and transaction processing
func (node *P2PNode) validateBlock(block *core.MegaBlock) bool {
	// Validate block signatures
	return block.VerifySignatures()
}

func (node *P2PNode) processBlock(block *core.MegaBlock) error {
	// Add block to blockchain (this would be implemented by connecting to a blockchain instance)
	return nil
}

func (node *P2PNode) forwardBlock(block *core.MegaBlock) {
	// Serialize the block
	blockData, err := json.Marshal(block)
	if err != nil {
		fmt.Printf("Error serializing block: %v\n", err)
		return
	}
	
	// Broadcast to peers
	node.Broadcast(protocol.ID(BlockProtocol), blockData)
}

func (node *P2PNode) validateTransaction(tx *core.Transaction) bool {
	// Validate transaction signature
	return tx.VerifySignature()
}

func (node *P2PNode) processTransaction(tx *core.Transaction) error {
	// Add transaction to mempool (this would be implemented by connecting to a blockchain instance)
	return nil
}

func (node *P2PNode) forwardTransaction(tx *core.Transaction) {
	// Serialize the transaction
	txData, err := json.Marshal(tx)
	if err != nil {
		fmt.Printf("Error serializing transaction: %v\n", err)
		return
	}
	
	// Broadcast to peers
	node.Broadcast(protocol.ID(TxProtocol), txData)
}