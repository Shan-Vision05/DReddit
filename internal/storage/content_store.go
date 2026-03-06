// Package storage provides content-addressed storage for DReddit.
//
// All posts and comments are stored as content-addressed objects,
// identified by their SHA-256 hash. This allows replicas to verify
// data integrity without relying on a central storage server.
package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/shan/dreddit/internal/crdt"
	"github.com/shan/dreddit/internal/models"
)

// ContentStore is a content-addressed store for posts, comments, and votes.
type ContentStore struct {
	mu sync.RWMutex

	// Content-addressed storage (hash -> object)
	posts    map[models.ContentHash]*models.Post
	comments map[models.ContentHash]*models.Comment

	// Indices
	communityPosts map[models.CommunityID][]models.ContentHash // community -> post hashes
	postComments   map[models.ContentHash][]models.ContentHash // post hash -> comment hashes

	// Vote state (CRDT-based)
	votes map[models.ContentHash]*crdt.VoteState

	// Persistence
	dataDir string
}

// NewContentStore creates a new content store.
func NewContentStore(dataDir string) (*ContentStore, error) {
	if dataDir != "" {
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create data dir: %w", err)
		}
	}

	cs := &ContentStore{
		posts:          make(map[models.ContentHash]*models.Post),
		comments:       make(map[models.ContentHash]*models.Comment),
		communityPosts: make(map[models.CommunityID][]models.ContentHash),
		postComments:   make(map[models.ContentHash][]models.ContentHash),
		votes:          make(map[models.ContentHash]*crdt.VoteState),
		dataDir:        dataDir,
	}

	// Try to load existing data from disk
	if dataDir != "" {
		_ = cs.LoadFromDisk()
	}

	return cs, nil
}

// -------------------------------------------------------------------
// Posts
// -------------------------------------------------------------------

// StorePost stores a post and returns its content hash.
func (cs *ContentStore) StorePost(post *models.Post) (models.ContentHash, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	hash := post.ComputeHash()
	post.Hash = hash

	cs.posts[hash] = post
	cs.communityPosts[post.CommunityID] = append(cs.communityPosts[post.CommunityID], hash)

	// Initialize vote state
	cs.votes[hash] = crdt.NewVoteState(hash)

	// Persist to disk
	cs.persistPost(post)

	return hash, nil
}

// GetPost retrieves a post by its hash.
func (cs *ContentStore) GetPost(hash models.ContentHash) (*models.Post, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	post, ok := cs.posts[hash]
	if !ok {
		return nil, fmt.Errorf("post not found: %s", hash)
	}
	return post, nil
}

// GetCommunityPosts returns all post hashes for a community.
func (cs *ContentStore) GetCommunityPosts(communityID models.CommunityID) []models.ContentHash {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	hashes, ok := cs.communityPosts[communityID]
	if !ok {
		return nil
	}

	result := make([]models.ContentHash, len(hashes))
	copy(result, hashes)
	return result
}

// -------------------------------------------------------------------
// Comments
// -------------------------------------------------------------------

// StoreComment stores a comment and returns its content hash.
func (cs *ContentStore) StoreComment(comment *models.Comment) (models.ContentHash, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	hash := comment.ComputeHash()
	comment.Hash = hash

	cs.comments[hash] = comment
	cs.postComments[comment.PostHash] = append(cs.postComments[comment.PostHash], hash)

	// Initialize vote state
	cs.votes[hash] = crdt.NewVoteState(hash)

	// Persist to disk
	cs.persistComment(comment)

	return hash, nil
}

// GetComment retrieves a comment by its hash.
func (cs *ContentStore) GetComment(hash models.ContentHash) (*models.Comment, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	comment, ok := cs.comments[hash]
	if !ok {
		return nil, fmt.Errorf("comment not found: %s", hash)
	}
	return comment, nil
}

// GetPostComments returns all comment hashes for a post.
func (cs *ContentStore) GetPostComments(postHash models.ContentHash) []models.ContentHash {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	hashes, ok := cs.postComments[postHash]
	if !ok {
		return nil
	}

	result := make([]models.ContentHash, len(hashes))
	copy(result, hashes)
	return result
}

// -------------------------------------------------------------------
// Votes (CRDT-based)
// -------------------------------------------------------------------

// ApplyVote applies a vote using CRDTs for convergence.
func (cs *ContentStore) ApplyVote(vote models.Vote, nodeID models.NodeID) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	vs, ok := cs.votes[vote.TargetHash]
	if !ok {
		return fmt.Errorf("target not found: %s", vote.TargetHash)
	}

	vs.ApplyVote(vote, nodeID)
	return nil
}

// GetVoteScore returns the current vote score for content.
func (cs *ContentStore) GetVoteScore(hash models.ContentHash) (int64, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	vs, ok := cs.votes[hash]
	if !ok {
		return 0, fmt.Errorf("vote state not found: %s", hash)
	}

	return vs.GetScore(), nil
}

// -------------------------------------------------------------------
// Persistence
// -------------------------------------------------------------------

func (cs *ContentStore) persistPost(post *models.Post) {
	if cs.dataDir == "" {
		return
	}

	dir := filepath.Join(cs.dataDir, "posts")
	_ = os.MkdirAll(dir, 0755)

	data, err := json.MarshalIndent(post, "", "  ")
	if err != nil {
		return
	}

	_ = os.WriteFile(filepath.Join(dir, string(post.Hash)+".json"), data, 0644)
}

func (cs *ContentStore) persistComment(comment *models.Comment) {
	if cs.dataDir == "" {
		return
	}

	dir := filepath.Join(cs.dataDir, "comments")
	_ = os.MkdirAll(dir, 0755)

	data, err := json.MarshalIndent(comment, "", "  ")
	if err != nil {
		return
	}

	_ = os.WriteFile(filepath.Join(dir, string(comment.Hash)+".json"), data, 0644)
}

// LoadFromDisk loads persisted data from disk.
func (cs *ContentStore) LoadFromDisk() error {
	// Load posts
	postsDir := filepath.Join(cs.dataDir, "posts")
	entries, err := os.ReadDir(postsDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(postsDir, entry.Name()))
			if err != nil {
				continue
			}
			var post models.Post
			if err := json.Unmarshal(data, &post); err != nil {
				continue
			}
			cs.posts[post.Hash] = &post
			cs.communityPosts[post.CommunityID] = append(cs.communityPosts[post.CommunityID], post.Hash)
			cs.votes[post.Hash] = crdt.NewVoteState(post.Hash)
		}
	}

	// Load comments
	commentsDir := filepath.Join(cs.dataDir, "comments")
	entries, err = os.ReadDir(commentsDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(commentsDir, entry.Name()))
			if err != nil {
				continue
			}
			var comment models.Comment
			if err := json.Unmarshal(data, &comment); err != nil {
				continue
			}
			cs.comments[comment.Hash] = &comment
			cs.postComments[comment.PostHash] = append(cs.postComments[comment.PostHash], comment.Hash)
			cs.votes[comment.Hash] = crdt.NewVoteState(comment.Hash)
		}
	}

	return nil
}
