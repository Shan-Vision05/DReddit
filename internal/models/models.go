// Package models defines the core data structures for DReddit.
// All content is content-addressed (identified by cryptographic hash).
package models

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// NodeID uniquely identifies a node in the network.
type NodeID string

// CommunityID uniquely identifies a community (analogous to a subreddit).
type CommunityID string

// ContentHash is a content-addressed identifier (SHA-256 of content).
type ContentHash string

// UserID uniquely identifies a user in the system.
type UserID string

// -------------------------------------------------------------------
// Community
// -------------------------------------------------------------------

// Community represents a discussion space (analogous to a subreddit).
// Each community is managed by a replica group of 3-5 nodes.
type Community struct {
	ID          CommunityID `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	CreatedAt   time.Time   `json:"created_at"`
	CreatedBy   UserID      `json:"created_by"`
	Moderators  []UserID    `json:"moderators"`
	ReplicaSet  []NodeID    `json:"replica_set"` // 3-5 nodes
}

// -------------------------------------------------------------------
// Post
// -------------------------------------------------------------------

// Post represents a user-submitted post within a community.
// Posts are content-addressed objects identified by their hash.
type Post struct {
	Hash        ContentHash `json:"hash"`
	CommunityID CommunityID `json:"community_id"`
	AuthorID    UserID      `json:"author_id"`
	Title       string      `json:"title"`
	Body        string      `json:"body"`
	URL         string      `json:"url,omitempty"` // link posts
	CreatedAt   time.Time   `json:"created_at"`
	IsRemoved   bool        `json:"is_removed"` // moderation flag
}

// ComputeHash generates the content-addressed hash for a post.
func (p *Post) ComputeHash() ContentHash {
	data := struct {
		CommunityID CommunityID
		AuthorID    UserID
		Title       string
		Body        string
		URL         string
		CreatedAt   time.Time
	}{p.CommunityID, p.AuthorID, p.Title, p.Body, p.URL, p.CreatedAt}

	b, _ := json.Marshal(data)
	h := sha256.Sum256(b)
	return ContentHash(hex.EncodeToString(h[:]))
}

// -------------------------------------------------------------------
// Comment
// -------------------------------------------------------------------

// Comment represents a comment in a discussion thread.
// Comments form a tree structure via ParentHash.
type Comment struct {
	Hash       ContentHash `json:"hash"`
	PostHash   ContentHash `json:"post_hash"`
	ParentHash ContentHash `json:"parent_hash,omitempty"` // empty = top-level
	AuthorID   UserID      `json:"author_id"`
	Body       string      `json:"body"`
	CreatedAt  time.Time   `json:"created_at"`
	IsRemoved  bool        `json:"is_removed"`
}

// ComputeHash generates the content-addressed hash for a comment.
func (c *Comment) ComputeHash() ContentHash {
	data := struct {
		PostHash   ContentHash
		ParentHash ContentHash
		AuthorID   UserID
		Body       string
		CreatedAt  time.Time
	}{c.PostHash, c.ParentHash, c.AuthorID, c.Body, c.CreatedAt}

	b, _ := json.Marshal(data)
	h := sha256.Sum256(b)
	return ContentHash(hex.EncodeToString(h[:]))
}

// -------------------------------------------------------------------
// Vote
// -------------------------------------------------------------------

// VoteType represents an upvote or downvote.
type VoteType int

const (
	Upvote   VoteType = 1
	Downvote VoteType = -1
)

// Vote represents a user's vote on a post or comment.
type Vote struct {
	TargetHash ContentHash `json:"target_hash"` // post or comment hash
	UserID     UserID      `json:"user_id"`
	Value      VoteType    `json:"value"`
	Timestamp  time.Time   `json:"timestamp"`
}

// -------------------------------------------------------------------
// Moderation
// -------------------------------------------------------------------

// ModActionType enumerates moderation action types.
type ModActionType string

const (
	ModRemovePost    ModActionType = "REMOVE_POST"
	ModRestorePost   ModActionType = "RESTORE_POST"
	ModRemoveComment ModActionType = "REMOVE_COMMENT"
	ModBanUser       ModActionType = "BAN_USER"
	ModUnbanUser     ModActionType = "UNBAN_USER"
)

// ModerationAction represents a moderation decision that requires
// consensus (strong consistency via Raft).
type ModerationAction struct {
	ID          string        `json:"id"`
	CommunityID CommunityID   `json:"community_id"`
	ModeratorID UserID        `json:"moderator_id"`
	ActionType  ModActionType `json:"action_type"`
	TargetHash  ContentHash   `json:"target_hash,omitempty"`
	TargetUser  UserID        `json:"target_user,omitempty"`
	Reason      string        `json:"reason"`
	Timestamp   time.Time     `json:"timestamp"`
	LogIndex    uint64        `json:"log_index"` // Raft log index
}

// -------------------------------------------------------------------
// Node Info
// -------------------------------------------------------------------

// NodeInfo represents metadata about a node in the network.
type NodeInfo struct {
	ID          NodeID        `json:"id"`
	Address     string        `json:"address"` // host:port
	RPCPort     int           `json:"rpc_port"`
	HTTPPort    int           `json:"http_port"`
	IsAlive     bool          `json:"is_alive"`
	LastSeen    time.Time     `json:"last_seen"`
	Communities []CommunityID `json:"communities"` // communities this node serves
}
