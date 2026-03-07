package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Shan-Vision05/DReddit/internal/crdt"
	"github.com/Shan-Vision05/DReddit/internal/models"
)

type ContentStore struct {
	mu sync.RWMutex

	posts    map[models.ContentHash]*models.Post
	comments map[models.ContentHash]*models.Comment

	communityPosts map[models.CommunityID][]models.ContentHash
	postComments   map[models.ContentHash][]models.ContentHash // post hash → comment hashes

	votes map[models.ContentHash]*crdt.VoteState

	dataDir string
}

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

	if dataDir != "" {
		_ = cs.LoadFromDisk()
	}

	return cs, nil
}

func (cs *ContentStore) StorePost(post *models.Post) (models.ContentHash, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	hash := post.ComputeHash()
	post.Hash = hash

	cs.posts[hash] = post
	cs.communityPosts[post.CommunityID] = append(cs.communityPosts[post.CommunityID], hash)
	cs.votes[hash] = crdt.NewVoteState(hash)

	cs.persistPost(post)
	return hash, nil
}

func (cs *ContentStore) GetPost(hash models.ContentHash) (*models.Post, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	post, ok := cs.posts[hash]
	if !ok {
		return nil, fmt.Errorf("post not found: %s", hash)
	}
	return post, nil
}

func (cs *ContentStore) GetCommunityPosts(communityID models.CommunityID) []models.ContentHash {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	hashes := cs.communityPosts[communityID]
	result := make([]models.ContentHash, len(hashes))
	copy(result, hashes)
	return result
}

func (cs *ContentStore) StoreComment(comment *models.Comment) (models.ContentHash, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	hash := comment.ComputeHash()
	comment.Hash = hash

	cs.comments[hash] = comment
	cs.postComments[comment.PostHash] = append(cs.postComments[comment.PostHash], hash)
	cs.votes[hash] = crdt.NewVoteState(hash)

	cs.persistComment(comment)
	return hash, nil
}

func (cs *ContentStore) GetComment(hash models.ContentHash) (*models.Comment, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	comment, ok := cs.comments[hash]
	if !ok {
		return nil, fmt.Errorf("comment not found: %s", hash)
	}
	return comment, nil
}

func (cs *ContentStore) GetPostComments(postHash models.ContentHash) []models.ContentHash {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	hashes := cs.postComments[postHash]
	result := make([]models.ContentHash, len(hashes))
	copy(result, hashes)
	return result
}

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

func (cs *ContentStore) GetVoteScore(hash models.ContentHash) (int64, error) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	vs, ok := cs.votes[hash]
	if !ok {
		return 0, fmt.Errorf("vote state not found: %s", hash)
	}
	return vs.GetScore(), nil
}

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

func (cs *ContentStore) LoadFromDisk() error {
	postsDir := filepath.Join(cs.dataDir, "posts")
	if entries, err := os.ReadDir(postsDir); err == nil {
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

	commentsDir := filepath.Join(cs.dataDir, "comments")
	if entries, err := os.ReadDir(commentsDir); err == nil {
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
