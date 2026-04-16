package community

import (
	"fmt"

	"github.com/Shan-Vision05/Distributed-Reddit/internal/consensus"
	"github.com/Shan-Vision05/Distributed-Reddit/internal/models"
	"github.com/Shan-Vision05/Distributed-Reddit/internal/network"
	"github.com/Shan-Vision05/Distributed-Reddit/internal/storage"
)

// Manager handles the operations for a single community,
// bridging Storage (Eventual), Network (Gossip), and Consensus (Raft).
type Manager struct {
	communityID models.CommunityID
	nodeID      models.NodeID
	store       *storage.ContentStore
	gossip      *network.GossipNode
	raft        *consensus.RaftNode
}

// NewManager creates a new community manager.
func NewManager(cid models.CommunityID, nid models.NodeID, s *storage.ContentStore, g *network.GossipNode, r *consensus.RaftNode) *Manager {
	return &Manager{
		communityID: cid,
		nodeID:      nid,
		store:       s,
		gossip:      g,
		raft:        r,
	}
}

// CreatePost stores a post locally and broadcasts it to the network.
func (m *Manager) CreatePost(post *models.Post) (models.ContentHash, error) {
	if post.CommunityID != m.communityID {
		return "", fmt.Errorf("post belongs to a different community")
	}

	// 1. Store locally
	hash, err := m.store.StorePost(post)
	if err != nil {
		return "", err
	}

	// 2. Eventual consistency: replicate to other nodes via gossip
	_ = m.gossip.BroadcastPost(post)

	return hash, nil
}

// CreateComment stores a comment and broadcasts it.
func (m *Manager) CreateComment(comment *models.Comment) (models.ContentHash, error) {
	hash, err := m.store.StoreComment(comment)
	if err != nil {
		return "", err
	}

	_ = m.gossip.BroadcastComment(comment)
	return hash, nil
}

// Vote applies a CRDT vote locally and broadcasts the updated state.
func (m *Manager) Vote(vote models.Vote) error {
	// 1. Apply vote locally (resolves conflicts using CRDT rules)
	if err := m.store.ApplyVote(vote, m.nodeID); err != nil {
		return err
	}

	// 2. Fetch the newly updated state and broadcast it
	vs := m.store.GetVoteState(vote.TargetHash)
	if vs != nil {
		_ = m.gossip.BroadcastVoteState(vs)
	}
	return nil
}

// Moderate proposes a moderation action via Raft (Strong Consistency).
func (m *Manager) Moderate(action models.ModerationAction) error {
	if action.CommunityID != m.communityID {
		return fmt.Errorf("action belongs to a different community")
	}

	// Strong consistency: all nodes must agree via Raft before it applies
	return m.raft.Propose(action)
}

// GetModerationLog retrieves the committed moderation actions for this community.
func (m *Manager) GetModerationLog() []models.ModerationAction {
	return m.raft.GetLog()
}