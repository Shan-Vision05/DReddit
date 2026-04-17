package community

import (
	"fmt"

	"github.com/Shan-Vision05/Distributed-Reddit/internal/consensus"
	"github.com/Shan-Vision05/Distributed-Reddit/internal/models"
	"github.com/Shan-Vision05/Distributed-Reddit/internal/network"
	"github.com/Shan-Vision05/Distributed-Reddit/internal/storage"
)

type Manager struct {
	communityID models.CommunityID
	nodeID      models.NodeID
	store       *storage.ContentStore
	gossip      *network.GossipNode
	raft        *consensus.RaftNode
}

func NewManager(cid models.CommunityID, nid models.NodeID, s *storage.ContentStore, g *network.GossipNode, r *consensus.RaftNode) *Manager {
	return &Manager{
		communityID: cid,
		nodeID:      nid,
		store:       s,
		gossip:      g,
		raft:        r,
	}
}

func (m *Manager) CreatePost(post *models.Post) (models.ContentHash, error) {
	if post.CommunityID != m.communityID {
		return "", fmt.Errorf("post belongs to a different community")
	}
	hash, err := m.store.StorePost(post)
	if err != nil {
		return "", err
	}
	_ = m.gossip.BroadcastPost(post)
	return hash, nil
}

func (m *Manager) CreateComment(comment *models.Comment) (models.ContentHash, error) {
	hash, err := m.store.StoreComment(comment)
	if err != nil {
		return "", err
	}
	_ = m.gossip.BroadcastComment(comment)
	return hash, nil
}

func (m *Manager) Vote(vote models.Vote) error {
	if err := m.store.ApplyVote(vote, m.nodeID); err != nil {
		return err
	}
	vs := m.store.GetVoteState(vote.TargetHash)
	if vs != nil {
		_ = m.gossip.BroadcastVoteState(vs)
	}
	return nil
}

func (m *Manager) Moderate(action models.ModerationAction) error {
	if action.CommunityID != m.communityID {
		return fmt.Errorf("action belongs to a different community")
	}
	return m.raft.Propose(action)
}

func (m *Manager) GetModerationLog() []models.ModerationAction {
	return m.raft.GetLog()
}

// GetPosts retrieves all posts for the community along with computed CRDT scores
func (m *Manager) GetPosts() ([]*models.Post, map[models.ContentHash]int64) {
	hashes := m.store.GetCommunityPosts(m.communityID)
	var posts []*models.Post
	scores := make(map[models.ContentHash]int64)
	for _, h := range hashes {
		if p, err := m.store.GetPost(h); err == nil {
			posts = append(posts, p)
			score, _ := m.store.GetVoteScore(h)
			scores[h] = score
		}
	}
	return posts, scores
}

// GetComments retrieves all comments for a post along with computed CRDT scores
func (m *Manager) GetComments(postHash models.ContentHash) ([]*models.Comment, map[models.ContentHash]int64) {
	hashes := m.store.GetPostComments(postHash)
	var comments []*models.Comment
	scores := make(map[models.ContentHash]int64)
	for _, h := range hashes {
		if c, err := m.store.GetComment(h); err == nil {
			comments = append(comments, c)
			score, _ := m.store.GetVoteScore(h)
			scores[h] = score
		}
	}
	return comments, scores
}