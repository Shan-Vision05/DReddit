package models

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

type NodeID string
type CommunityID string
type ContentID string
type UserID string
type ContentHash string

type Community struct {
	ID          CommunityID `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	CreatedAt   time.Time   `json:"created_at"`
	CreatedBy   UserID      `json:"created_by"`
	Moderators  []UserID    `json:"moderators"`
	ReplicaSet  []NodeID    `json:"replica_set"`
}

type Post struct {
	Hash        ContentHash `json:"hash"`
	CommunityID CommunityID `json:"community_id"`
	AuthorID    UserID      `json:"author_id"`
	Title       string      `json:"title"`
	Body        string      `json:"body"`
	URL         string      `json:"url,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
	IsRemoved   bool        `json:"is_removed"`
}

func (p *Post) ComputeHash() ContentHash {
	data := struct {
		CommunityID CommunityID `json:"community_id"`
		AuthorID    UserID      `json:"author_id"`
		Title       string      `json:"title"`
		Body        string      `json:"body"`
		URL         string      `json:"url,omitempty"`
		CreatedAt   time.Time   `json:"created_at"`
	}{p.CommunityID, p.AuthorID, p.Title, p.Body, p.URL, p.CreatedAt}

	jsonData, _ := json.Marshal(data)
	hash := sha256.Sum256(jsonData)
	return ContentHash(hex.EncodeToString(hash[:]))
}

type VoteType int

const (
	Upvote   VoteType = 1
	Downvote VoteType = -1
)

type Vote struct {
	TargetHash ContentHash `json:"target_hash"`
	UserID     UserID      `json:"user_id"`
	Value      VoteType    `json:"value"`
	Timestamp  time.Time   `json:"timestamp"`
}

type ModerationActionType string

const (
	ModRemovePost    ModerationActionType = "REMOVE_POST"
	ModRestorePost   ModerationActionType = "RESTORE_POST"
	ModRemoveComment ModerationActionType = "REMOVE_COMMENT"
	ModBanUser       ModerationActionType = "BAN_USER"
	ModUnbanUser     ModerationActionType = "UNBAN_USER"
)

type ModerationAction struct {
	ID          string               `json:"id"`
	CommunityID CommunityID          `json:"community_id"`
	ModeratorID UserID               `json:"moderator_id"`
	ActionType  ModerationActionType `json:"action_type"`
	TargetHash  ContentHash          `json:"target_hash,omitempty"`
	TargetUser  UserID               `json:"target_user_id,omitempty"`
	Reason      string               `json:"reason,omitempty"`
	Timestamp   time.Time            `json:"timestamp"`
	LogIndex    uint64               `json:"log_index"`
}

type NodeInfo struct {
	ID          NodeID        `json:"id"`
	Address     string        `json:"address"`
	RPCPort     int           `json:"rpc_port"`
	HTTPPort    int           `json:"http_port"`
	IsAlive     bool          `json:"is_alive"`
	LastSeen    time.Time     `json:"last_seen"`
	Communities []CommunityID `json:"communities"`
}
