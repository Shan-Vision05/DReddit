package storage

import (
	"os"
	"testing"
	"time"

	"github.com/Shan-Vision05/DReddit/internal/models"
)

func TestStoreAndGetPost(t *testing.T) {
	store, err := NewContentStore("")
	if err != nil {
		t.Fatalf("NewContentStore: %v", err)
	}

	post := &models.Post{
		CommunityID: "community1",
		AuthorID:    "alice",
		Title:       "Hello World",
		Body:        "This is the first post",
		CreatedAt:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	hash, err := store.StorePost(post)
	if err != nil {
		t.Fatalf("StorePost: %v", err)
	}
	if hash == "" {
		t.Fatal("StorePost returned empty hash")
	}

	got, err := store.GetPost(hash)
	if err != nil {
		t.Fatalf("GetPost: %v", err)
	}
	if got.Title != "Hello World" {
		t.Errorf("GetPost title = %q, want %q", got.Title, "Hello World")
	}
	if got.Hash != hash {
		t.Errorf("Post.Hash = %q, want %q", got.Hash, hash)
	}
}

func TestGetPost_NotFound(t *testing.T) {
	store, _ := NewContentStore("")

	_, err := store.GetPost("nonexistent")
	if err == nil {
		t.Error("GetPost should return error for nonexistent hash")
	}
}

func TestCommunityPostsIndex(t *testing.T) {
	store, _ := NewContentStore("")

	p1 := &models.Post{CommunityID: "comm1", AuthorID: "alice", Title: "Post 1", CreatedAt: time.Now()}
	p2 := &models.Post{CommunityID: "comm1", AuthorID: "bob", Title: "Post 2", CreatedAt: time.Now()}
	p3 := &models.Post{CommunityID: "comm2", AuthorID: "alice", Title: "Post 3", CreatedAt: time.Now()}

	store.StorePost(p1)
	store.StorePost(p2)
	store.StorePost(p3)

	comm1Posts := store.GetCommunityPosts("comm1")
	if len(comm1Posts) != 2 {
		t.Errorf("comm1 should have 2 posts, got %d", len(comm1Posts))
	}

	comm2Posts := store.GetCommunityPosts("comm2")
	if len(comm2Posts) != 1 {
		t.Errorf("comm2 should have 1 post, got %d", len(comm2Posts))
	}
}

func TestStoreAndGetComment(t *testing.T) {
	store, _ := NewContentStore("")

	post := &models.Post{CommunityID: "comm1", AuthorID: "alice", Title: "Post", CreatedAt: time.Now()}
	postHash, _ := store.StorePost(post)

	comment := &models.Comment{
		PostHash:  postHash,
		AuthorID:  "bob",
		Body:      "Great post!",
		CreatedAt: time.Now(),
	}

	commentHash, err := store.StoreComment(comment)
	if err != nil {
		t.Fatalf("StoreComment: %v", err)
	}

	got, err := store.GetComment(commentHash)
	if err != nil {
		t.Fatalf("GetComment: %v", err)
	}
	if got.Body != "Great post!" {
		t.Errorf("Comment body = %q, want %q", got.Body, "Great post!")
	}

	// Check post→comments index
	commentHashes := store.GetPostComments(postHash)
	if len(commentHashes) != 1 {
		t.Errorf("Post should have 1 comment, got %d", len(commentHashes))
	}
}

func TestVoteOnPost(t *testing.T) {
	store, _ := NewContentStore("")

	post := &models.Post{CommunityID: "comm1", AuthorID: "alice", Title: "Post", CreatedAt: time.Now()}
	hash, _ := store.StorePost(post)

	store.ApplyVote(models.Vote{TargetHash: hash, UserID: "bob", Value: models.Upvote}, "node1")
	store.ApplyVote(models.Vote{TargetHash: hash, UserID: "carol", Value: models.Upvote}, "node1")
	store.ApplyVote(models.Vote{TargetHash: hash, UserID: "dave", Value: models.Downvote}, "node1")

	score, err := store.GetVoteScore(hash)
	if err != nil {
		t.Fatalf("GetVoteScore: %v", err)
	}
	if score != 1 {
		t.Errorf("Vote score = %d, want 1 (2 up - 1 down)", score)
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()

	// Store a post
	store1, _ := NewContentStore(dir)
	post := &models.Post{
		CommunityID: "comm1",
		AuthorID:    "alice",
		Title:       "Persistent Post",
		Body:        "Should survive restart",
		CreatedAt:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	hash, _ := store1.StorePost(post)

	// Verify the JSON file exists on disk
	jsonPath := dir + "/posts/" + string(hash) + ".json"
	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		t.Fatal("Post JSON file was not written to disk")
	}

	// Create a new store from the same directory — should load the post
	store2, _ := NewContentStore(dir)
	got, err := store2.GetPost(hash)
	if err != nil {
		t.Fatalf("After reload, GetPost: %v", err)
	}
	if got.Title != "Persistent Post" {
		t.Errorf("After reload, title = %q, want %q", got.Title, "Persistent Post")
	}
}
