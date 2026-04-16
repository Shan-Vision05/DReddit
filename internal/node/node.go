package node

import (
	"fmt"
	"sync"

	"github.com/Shan-Vision05/Distributed-Reddit/internal/community"
	"github.com/Shan-Vision05/Distributed-Reddit/internal/consensus"
	"github.com/Shan-Vision05/Distributed-Reddit/internal/dht"
	"github.com/Shan-Vision05/Distributed-Reddit/internal/models"
	"github.com/Shan-Vision05/Distributed-Reddit/internal/network"
	"github.com/Shan-Vision05/Distributed-Reddit/internal/storage"
)

// Node represents a single peer in the distributed network.
// It acts as the host for multiple communities simultaneously.
type Node struct {
	NodeID models.NodeID
	DHT    *dht.DHTNode
	Gossip *network.GossipNode

	mu          sync.RWMutex
	communities map[models.CommunityID]*community.Manager
}

// NewNode boots up a new Distributed Reddit node on your machine.
func NewNode(nodeID models.NodeID, bindAddr string) (*Node, error) {
	// 1. Initialize the global DHT (Distributed Hash Table) so peers can find you
	dhtNode := dht.NewDHTNode(nodeID, bindAddr)

	// 2. Initialize the global Gossip network for passing messages
	gossipNode := network.NewGossipNode(nodeID, bindAddr)

	return &Node{
		NodeID:      nodeID,
		DHT:         dhtNode,
		Gossip:      gossipNode,
		communities: make(map[models.CommunityID]*community.Manager),
	}, nil
}

// JoinCommunity spins up the resources needed to participate in a specific community.
func (n *Node) JoinCommunity(communityID models.CommunityID) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Check if we are already in this community
	if _, exists := n.communities[communityID]; exists {
		return fmt.Errorf("already a member of community %s", communityID)
	}

	// 1. Set up isolated storage just for this community
	store := storage.NewContentStore()

	// 2. Set up isolated Raft consensus just for this community's moderation
	raftNode, err := consensus.NewRaftNode(string(n.NodeID), communityID, store)
	if err != nil {
		return fmt.Errorf("failed to initialize raft for community: %v", err)
	}

	// 3. Create the Community Manager (the code from Step 7)
	manager := community.NewManager(communityID, n.NodeID, store, n.Gossip, raftNode)

	// 4. Save it to our active map
	n.communities[communityID] = manager

	// 5. Announce to the global DHT that our IP address is now hosting this community
	n.DHT.Announce(string(communityID))

	return nil
}

// GetCommunity retrieves the manager so the API can route user actions (like posting) to the right place.
func (n *Node) GetCommunity(communityID models.CommunityID) (*community.Manager, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	manager, exists := n.communities[communityID]
	if !exists {
		return nil, fmt.Errorf("not a member of community %s", communityID)
	}

	return manager, nil
}