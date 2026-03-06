// Package network provides the networking and gossip layer for DReddit.
//
// Nodes communicate via gRPC for RPCs (Raft, data replication) and
// use a gossip protocol for CRDT state synchronization and failure detection.
package network

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/shan/dreddit/internal/consensus"
	"github.com/shan/dreddit/internal/models"
)

// -------------------------------------------------------------------
// PeerManager: manages connections to peer nodes
// -------------------------------------------------------------------

// PeerInfo holds information about a connected peer.
type PeerInfo struct {
	models.NodeInfo
	IsConnected bool
	LastPing    time.Time
	RTT         time.Duration // round-trip time
}

// PeerManager manages connections to peer nodes.
type PeerManager struct {
	mu    sync.RWMutex
	self  models.NodeID
	peers map[models.NodeID]*PeerInfo
}

// NewPeerManager creates a new peer manager.
func NewPeerManager(selfID models.NodeID) *PeerManager {
	return &PeerManager{
		self:  selfID,
		peers: make(map[models.NodeID]*PeerInfo),
	}
}

// AddPeer adds a new peer to the manager.
func (pm *PeerManager) AddPeer(info models.NodeInfo) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.peers[info.ID] = &PeerInfo{
		NodeInfo:    info,
		IsConnected: true,
		LastPing:    time.Now(),
	}
	log.Printf("[PeerManager %s] Added peer %s at %s", pm.self, info.ID, info.Address)
}

// RemovePeer removes a peer from the manager.
func (pm *PeerManager) RemovePeer(nodeID models.NodeID) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.peers, nodeID)
}

// GetPeer returns info about a specific peer.
func (pm *PeerManager) GetPeer(nodeID models.NodeID) (*PeerInfo, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	info, ok := pm.peers[nodeID]
	return info, ok
}

// GetAlivePeers returns all currently connected peers.
func (pm *PeerManager) GetAlivePeers() []models.NodeID {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	result := make([]models.NodeID, 0)
	for id, info := range pm.peers {
		if info.IsConnected {
			result = append(result, id)
		}
	}
	return result
}

// MarkDead marks a peer as disconnected.
func (pm *PeerManager) MarkDead(nodeID models.NodeID) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if info, ok := pm.peers[nodeID]; ok {
		info.IsConnected = false
		log.Printf("[PeerManager %s] Peer %s marked as dead", pm.self, nodeID)
	}
}

// MarkAlive marks a peer as connected.
func (pm *PeerManager) MarkAlive(nodeID models.NodeID) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if info, ok := pm.peers[nodeID]; ok {
		info.IsConnected = true
		info.LastPing = time.Now()
	}
}

// -------------------------------------------------------------------
// GossipProtocol: Anti-entropy synchronization for CRDTs
// -------------------------------------------------------------------

// GossipMessage represents a gossip message exchanged between nodes.
type GossipMessage struct {
	SenderID    models.NodeID      `json:"sender_id"`
	Type        GossipMessageType  `json:"type"`
	CommunityID models.CommunityID `json:"community_id,omitempty"`
	Payload     []byte             `json:"payload"`
	Timestamp   time.Time          `json:"timestamp"`
}

// GossipMessageType enumerates gossip message types.
type GossipMessageType string

const (
	GossipPing      GossipMessageType = "PING"
	GossipPong      GossipMessageType = "PONG"
	GossipSyncCRDT  GossipMessageType = "SYNC_CRDT"
	GossipSyncPosts GossipMessageType = "SYNC_POSTS"
	GossipSyncVotes GossipMessageType = "SYNC_VOTES"
	GossipNodeJoin  GossipMessageType = "NODE_JOIN"
	GossipNodeLeave GossipMessageType = "NODE_LEAVE"
)

// GossipHandler defines the interface for handling gossip messages.
type GossipHandler interface {
	HandleGossip(msg *GossipMessage) (*GossipMessage, error)
}

// GossipProtocol implements periodic anti-entropy synchronization.
type GossipProtocol struct {
	mu       sync.RWMutex
	nodeID   models.NodeID
	peers    *PeerManager
	handler  GossipHandler
	interval time.Duration
	stopCh   chan struct{}
}

// NewGossipProtocol creates a new gossip protocol instance.
func NewGossipProtocol(nodeID models.NodeID, peers *PeerManager, handler GossipHandler, interval time.Duration) *GossipProtocol {
	return &GossipProtocol{
		nodeID:   nodeID,
		peers:    peers,
		handler:  handler,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start begins periodic gossip rounds.
func (gp *GossipProtocol) Start() {
	log.Printf("[Gossip %s] Starting with interval %s", gp.nodeID, gp.interval)
	go gp.run()
}

// Stop halts gossip rounds.
func (gp *GossipProtocol) Stop() {
	close(gp.stopCh)
}

func (gp *GossipProtocol) run() {
	ticker := time.NewTicker(gp.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			gp.doGossipRound()
		case <-gp.stopCh:
			return
		}
	}
}

func (gp *GossipProtocol) doGossipRound() {
	peers := gp.peers.GetAlivePeers()
	if len(peers) == 0 {
		return
	}

	// Pick a random peer to sync with
	// In a full implementation, this would use proper random selection
	target := peers[time.Now().UnixNano()%int64(len(peers))]

	msg := &GossipMessage{
		SenderID:  gp.nodeID,
		Type:      GossipPing,
		Timestamp: time.Now(),
	}
	_ = msg
	_ = target
	// In a full implementation, this would send over the network
	log.Printf("[Gossip %s] Sending gossip to %s", gp.nodeID, target)
}

// -------------------------------------------------------------------
// InMemoryRaftTransport: for testing
// -------------------------------------------------------------------

// InMemoryRaftTransport implements RaftTransport for local/testing use.
type InMemoryRaftTransport struct {
	mu    sync.RWMutex
	nodes map[models.NodeID]*consensus.RaftNode
}

// NewInMemoryRaftTransport creates a new in-memory transport.
func NewInMemoryRaftTransport() *InMemoryRaftTransport {
	return &InMemoryRaftTransport{
		nodes: make(map[models.NodeID]*consensus.RaftNode),
	}
}

// RegisterNode registers a Raft node with the transport.
func (t *InMemoryRaftTransport) RegisterNode(id models.NodeID, node *consensus.RaftNode) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.nodes[id] = node
}

// RequestVote sends a RequestVote RPC to a peer (in-memory).
func (t *InMemoryRaftTransport) RequestVote(peer models.NodeID, req *consensus.RequestVoteRequest) (*consensus.RequestVoteResponse, error) {
	t.mu.RLock()
	node, ok := t.nodes[peer]
	t.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("node %s not found", peer)
	}
	return node.HandleRequestVote(req), nil
}

// AppendEntries sends an AppendEntries RPC to a peer (in-memory).
func (t *InMemoryRaftTransport) AppendEntries(peer models.NodeID, req *consensus.AppendEntriesRequest) (*consensus.AppendEntriesResponse, error) {
	t.mu.RLock()
	node, ok := t.nodes[peer]
	t.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("node %s not found", peer)
	}
	return node.HandleAppendEntries(req), nil
}
