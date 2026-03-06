// Package node implements the main DReddit node that ties all components together.
package node

import (
	"fmt"
	"log"
	"sync"

	"github.com/shan/dreddit/internal/community"
	"github.com/shan/dreddit/internal/config"
	"github.com/shan/dreddit/internal/dht"
	"github.com/shan/dreddit/internal/models"
	"github.com/shan/dreddit/internal/network"
)

// Node represents a single DReddit node in the network.
type Node struct {
	mu sync.RWMutex

	// Node identity
	ID     models.NodeID
	Config *config.Config

	// Components
	CommunityManager *community.Manager   // manages communities on this node
	PeerManager      *network.PeerManager // manages peer connections
	DHT              *dht.DHT             // decentralized community lookup

	// State
	isRunning bool
}

// NewNode creates a new DReddit node.
func NewNode(cfg *config.Config) (*Node, error) {
	nodeID := models.NodeID(cfg.NodeID)
	if nodeID == "" {
		nodeID = models.NodeID(fmt.Sprintf("node_%d", cfg.RPCPort))
	}

	n := &Node{
		ID:               nodeID,
		Config:           cfg,
		CommunityManager: community.NewManager(nodeID),
		PeerManager:      network.NewPeerManager(nodeID),
		DHT:              dht.NewDHT(nodeID, cfg.ReplicaCount),
	}

	// Register self in DHT
	n.DHT.AddNode(&models.NodeInfo{
		ID:       nodeID,
		Address:  fmt.Sprintf("%s:%d", cfg.Address, cfg.RPCPort),
		RPCPort:  cfg.RPCPort,
		HTTPPort: cfg.HTTPPort,
		IsAlive:  true,
	})

	log.Printf("[Node %s] Created at %s (RPC: %d, HTTP: %d)",
		nodeID, cfg.Address, cfg.RPCPort, cfg.HTTPPort)

	return n, nil
}

// Start starts the node and all its components.
func (n *Node) Start() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.isRunning {
		return fmt.Errorf("node already running")
	}

	log.Printf("[Node %s] Starting...", n.ID)

	// Connect to bootstrap nodes
	for _, addr := range n.Config.BootstrapNodes {
		log.Printf("[Node %s] Connecting to bootstrap node: %s", n.ID, addr)
		// TODO: Establish gRPC connection and exchange node info
		_ = addr
	}

	n.isRunning = true
	log.Printf("[Node %s] Started successfully", n.ID)

	return nil
}

// Stop gracefully shuts down the node.
func (n *Node) Stop() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if !n.isRunning {
		return nil
	}

	log.Printf("[Node %s] Stopping...", n.ID)
	n.isRunning = false

	return nil
}

// IsRunning returns whether the node is currently active.
func (n *Node) IsRunning() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.isRunning
}

// -------------------------------------------------------------------
// High-level operations
// -------------------------------------------------------------------

// CreateCommunity creates a new community and assigns replica nodes.
func (n *Node) CreateCommunity(name, description string, creator models.UserID) (*models.Community, error) {
	comm, err := n.CommunityManager.CreateCommunity(name, description, creator, n.Config.DataDir)
	if err != nil {
		return nil, err
	}

	// Register community in DHT
	n.DHT.RegisterCommunity(comm.ID, comm.ReplicaSet)

	return comm, nil
}

// JoinPeer adds a remote node as a peer.
func (n *Node) JoinPeer(info models.NodeInfo) {
	n.PeerManager.AddPeer(info)
	n.DHT.AddNode(&info)
}

// GetNodeInfo returns this node's info.
func (n *Node) GetNodeInfo() models.NodeInfo {
	return models.NodeInfo{
		ID:       n.ID,
		Address:  fmt.Sprintf("%s:%d", n.Config.Address, n.Config.RPCPort),
		RPCPort:  n.Config.RPCPort,
		HTTPPort: n.Config.HTTPPort,
		IsAlive:  n.isRunning,
	}
}
