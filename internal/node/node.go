package node

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/Shan-Vision05/Distributed-Reddit/internal/community"
	"github.com/Shan-Vision05/Distributed-Reddit/internal/consensus"
	"github.com/Shan-Vision05/Distributed-Reddit/internal/dht"
	"github.com/Shan-Vision05/Distributed-Reddit/internal/models"
	"github.com/Shan-Vision05/Distributed-Reddit/internal/network"
	"github.com/Shan-Vision05/Distributed-Reddit/internal/storage"
)

type Node struct {
	NodeID models.NodeID
	DHT    *dht.CommunityDHT
	Gossip *network.GossipNode

	mu          sync.RWMutex
	communities map[models.CommunityID]*community.Manager
}

func NewNode(nodeID models.NodeID, bindAddr string) (*Node, error) {
	// 1. Initialize DHT
	dhtNode := dht.NewCommunityDHT(dht.DHTConfig{VirtualNodes: 150, ReplicationFactor: 3})
	dhtNode.AddNode(&models.NodeInfo{ID: nodeID, Address: bindAddr, IsAlive: true})

	// 2. Initialize Gossip Network
	store, _ := storage.NewContentStore("")
	rand.Seed(time.Now().UnixNano())
	gossipPort := 10000 + rand.Intn(10000) // Random port for gossip

	gossipNode, err := network.NewGossipNode(network.GossipConfig{
		NodeID:   nodeID,
		BindAddr: "127.0.0.1",
		BindPort: gossipPort,
	}, store)
	if err != nil {
		return nil, fmt.Errorf("failed to start gossip: %v", err)
	}

	return &Node{
		NodeID:      nodeID,
		DHT:         dhtNode,
		Gossip:      gossipNode,
		communities: make(map[models.CommunityID]*community.Manager),
	}, nil
}

func (n *Node) JoinCommunity(communityID models.CommunityID) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if _, exists := n.communities[communityID]; exists {
		return fmt.Errorf("already a member of community %s", communityID)
	}

	store, _ := storage.NewContentStore("")

	raftCfg := consensus.RaftConfig{
		NodeID:      n.NodeID,
		CommunityID: communityID,
		BindAddr:    fmt.Sprintf("127.0.0.1:%d", 20000+rand.Intn(10000)), // Random port for raft
		Bootstrap:   true,
	}
	raftNode, err := consensus.NewRaftNode(raftCfg)
	if err != nil {
		return fmt.Errorf("failed to initialize raft for community: %v", err)
	}

	manager := community.NewManager(communityID, n.NodeID, store, n.Gossip, raftNode)
	n.communities[communityID] = manager

	return nil
}

func (n *Node) GetCommunity(communityID models.CommunityID) (*community.Manager, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	manager, exists := n.communities[communityID]
	if !exists {
		return nil, fmt.Errorf("not a member of community %s", communityID)
	}
	return manager, nil
}

func (n *Node) GetJoinedCommunities() []models.CommunityID {
	n.mu.RLock()
	defer n.mu.RUnlock()

	var list []models.CommunityID
	for cid := range n.communities {
		list = append(list, cid)
	}
	return list
}