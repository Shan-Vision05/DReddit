// Package community manages community state and operations.
//
// Each community has its own storage, CRDT state, and Raft consensus
// group for moderation.
package community

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/shan/dreddit/internal/consensus"
	"github.com/shan/dreddit/internal/crdt"
	"github.com/shan/dreddit/internal/models"
	"github.com/shan/dreddit/internal/storage"
)

// Manager manages all communities on this node.
type Manager struct {
	mu          sync.RWMutex
	nodeID      models.NodeID
	communities map[models.CommunityID]*CommunityState
}

// CommunityState holds the full state for a single community on this node.
type CommunityState struct {
	mu sync.RWMutex

	// Community metadata
	Community models.Community

	// Content storage (content-addressed)
	Store *storage.ContentStore

	// CRDT state
	MemberSet   *crdt.ORSet // community members (banned users removed)
	BannedUsers *crdt.GSet  // permanently banned users

	// Consensus (Raft) for moderation
	RaftNode *consensus.RaftNode

	// Moderation log (applied from Raft)
	ModLog []models.ModerationAction
}

// NewManager creates a new community manager.
func NewManager(nodeID models.NodeID) *Manager {
	return &Manager{
		nodeID:      nodeID,
		communities: make(map[models.CommunityID]*CommunityState),
	}
}

// CreateCommunity creates a new community on this node.
func (m *Manager) CreateCommunity(name, description string, creator models.UserID, dataDir string) (*models.Community, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	communityID := models.CommunityID(fmt.Sprintf("c_%s_%d", name, time.Now().UnixNano()))

	community := models.Community{
		ID:          communityID,
		Name:        name,
		Description: description,
		CreatedAt:   time.Now(),
		CreatedBy:   creator,
		Moderators:  []models.UserID{creator},
		ReplicaSet:  []models.NodeID{m.nodeID},
	}

	store, err := storage.NewContentStore(fmt.Sprintf("%s/%s", dataDir, communityID))
	if err != nil {
		return nil, fmt.Errorf("failed to create store: %w", err)
	}

	state := &CommunityState{
		Community:   community,
		Store:       store,
		MemberSet:   crdt.NewORSet(),
		BannedUsers: crdt.NewGSet(),
		ModLog:      make([]models.ModerationAction, 0),
	}

	// Add creator as first member
	state.MemberSet.Add(string(creator), fmt.Sprintf("join_%d", time.Now().UnixNano()))

	m.communities[communityID] = state
	log.Printf("[CommunityManager %s] Created community %s (%s)", m.nodeID, name, communityID)

	return &community, nil
}

// GetCommunity returns the state of a community.
func (m *Manager) GetCommunity(id models.CommunityID) (*CommunityState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, ok := m.communities[id]
	if !ok {
		return nil, fmt.Errorf("community not found: %s", id)
	}
	return state, nil
}

// ListCommunities returns all communities on this node.
func (m *Manager) ListCommunities() []models.Community {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]models.Community, 0, len(m.communities))
	for _, state := range m.communities {
		result = append(result, state.Community)
	}
	return result
}

// -------------------------------------------------------------------
// Community operations
// -------------------------------------------------------------------

// CreatePost creates a new post in a community.
func (cs *CommunityState) CreatePost(author models.UserID, title, body, url string) (*models.Post, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	// Check if user is banned
	if cs.BannedUsers.Contains(string(author)) {
		return nil, fmt.Errorf("user %s is banned from community %s", author, cs.Community.ID)
	}

	post := &models.Post{
		CommunityID: cs.Community.ID,
		AuthorID:    author,
		Title:       title,
		Body:        body,
		URL:         url,
		CreatedAt:   time.Now(),
	}

	hash, err := cs.Store.StorePost(post)
	if err != nil {
		return nil, err
	}
	post.Hash = hash

	log.Printf("[Community %s] New post by %s: %s (hash: %s)",
		cs.Community.Name, author, title, hash)

	return post, nil
}

// CreateComment creates a new comment on a post.
func (cs *CommunityState) CreateComment(author models.UserID, postHash models.ContentHash, parentHash models.ContentHash, body string) (*models.Comment, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.BannedUsers.Contains(string(author)) {
		return nil, fmt.Errorf("user %s is banned from community %s", author, cs.Community.ID)
	}

	comment := &models.Comment{
		PostHash:   postHash,
		ParentHash: parentHash,
		AuthorID:   author,
		Body:       body,
		CreatedAt:  time.Now(),
	}

	hash, err := cs.Store.StoreComment(comment)
	if err != nil {
		return nil, err
	}
	comment.Hash = hash

	return comment, nil
}

// Vote applies a vote to a post or comment.
func (cs *CommunityState) Vote(userID models.UserID, targetHash models.ContentHash, value models.VoteType, nodeID models.NodeID) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.BannedUsers.Contains(string(userID)) {
		return fmt.Errorf("user %s is banned", userID)
	}

	vote := models.Vote{
		TargetHash: targetHash,
		UserID:     userID,
		Value:      value,
		Timestamp:  time.Now(),
	}

	return cs.Store.ApplyVote(vote, nodeID)
}

// GetPosts returns all posts in the community.
func (cs *CommunityState) GetPosts() ([]*models.Post, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	hashes := cs.Store.GetCommunityPosts(cs.Community.ID)
	posts := make([]*models.Post, 0, len(hashes))
	for _, hash := range hashes {
		post, err := cs.Store.GetPost(hash)
		if err != nil {
			continue
		}
		if !post.IsRemoved {
			posts = append(posts, post)
		}
	}
	return posts, nil
}

// GetPostWithComments returns a post with its comment tree.
func (cs *CommunityState) GetPostWithComments(postHash models.ContentHash) (*models.Post, []*models.Comment, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	post, err := cs.Store.GetPost(postHash)
	if err != nil {
		return nil, nil, err
	}

	commentHashes := cs.Store.GetPostComments(postHash)
	comments := make([]*models.Comment, 0, len(commentHashes))
	for _, hash := range commentHashes {
		comment, err := cs.Store.GetComment(hash)
		if err != nil {
			continue
		}
		if !comment.IsRemoved {
			comments = append(comments, comment)
		}
	}

	return post, comments, nil
}

// -------------------------------------------------------------------
// Moderation (applied via Raft consensus)
// -------------------------------------------------------------------

// ApplyModeration applies a moderation action (called when Raft commits).
func (cs *CommunityState) ApplyModeration(action models.ModerationAction) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	switch action.ActionType {
	case models.ModRemovePost:
		post, err := cs.Store.GetPost(action.TargetHash)
		if err != nil {
			return err
		}
		post.IsRemoved = true
		log.Printf("[Moderation] Post %s removed by %s: %s", action.TargetHash, action.ModeratorID, action.Reason)

	case models.ModRestorePost:
		post, err := cs.Store.GetPost(action.TargetHash)
		if err != nil {
			return err
		}
		post.IsRemoved = false

	case models.ModRemoveComment:
		comment, err := cs.Store.GetComment(action.TargetHash)
		if err != nil {
			return err
		}
		comment.IsRemoved = true

	case models.ModBanUser:
		cs.BannedUsers.Add(string(action.TargetUser))
		cs.MemberSet.Remove(string(action.TargetUser))
		log.Printf("[Moderation] User %s banned from %s: %s", action.TargetUser, cs.Community.Name, action.Reason)

	case models.ModUnbanUser:
		// GSet is grow-only, so we'd need a separate "unbanned" set in practice
		log.Printf("[Moderation] User %s unbanned from %s", action.TargetUser, cs.Community.Name)

	default:
		return fmt.Errorf("unknown moderation action: %s", action.ActionType)
	}

	cs.ModLog = append(cs.ModLog, action)
	return nil
}

// IsModerator checks if a user is a moderator of this community.
func (cs *CommunityState) IsModerator(userID models.UserID) bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	for _, mod := range cs.Community.Moderators {
		if mod == userID {
			return true
		}
	}
	return false
}
