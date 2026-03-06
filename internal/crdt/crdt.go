// Package crdt implements Conflict-Free Replicated Data Types for DReddit.
//
// CRDTs allow replicas to process updates concurrently and guarantee
// convergence without coordination. Used for:
//   - Vote counts (PN-Counter)
//   - Comment/post membership sets (OR-Set / Add-Wins Set)
//   - Timestamps/ordering (LWW-Register)
package crdt

import (
	"sync"
	"time"

	"github.com/shan/dreddit/internal/models"
)

// -------------------------------------------------------------------
// PNCounter: Positive-Negative Counter (for vote counts)
// -------------------------------------------------------------------

// PNCounter is a CRDT counter that supports both increments and decrements.
// Each node maintains its own positive and negative counters.
// The value is sum(positive) - sum(negative).
type PNCounter struct {
	mu       sync.RWMutex
	Positive map[models.NodeID]int64 `json:"positive"` // per-node increment counts
	Negative map[models.NodeID]int64 `json:"negative"` // per-node decrement counts
}

// NewPNCounter creates a new PN-Counter.
func NewPNCounter() *PNCounter {
	return &PNCounter{
		Positive: make(map[models.NodeID]int64),
		Negative: make(map[models.NodeID]int64),
	}
}

// Increment adds 1 to the counter for the given node.
func (c *PNCounter) Increment(nodeID models.NodeID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Positive[nodeID]++
}

// Decrement subtracts 1 from the counter for the given node.
func (c *PNCounter) Decrement(nodeID models.NodeID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Negative[nodeID]++
}

// Value returns the current counter value: sum(P) - sum(N).
func (c *PNCounter) Value() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var pos, neg int64
	for _, v := range c.Positive {
		pos += v
	}
	for _, v := range c.Negative {
		neg += v
	}
	return pos - neg
}

// Merge merges another PNCounter into this one (takes max per node).
func (c *PNCounter) Merge(other *PNCounter) {
	c.mu.Lock()
	defer c.mu.Unlock()

	other.mu.RLock()
	defer other.mu.RUnlock()

	for nodeID, val := range other.Positive {
		if val > c.Positive[nodeID] {
			c.Positive[nodeID] = val
		}
	}
	for nodeID, val := range other.Negative {
		if val > c.Negative[nodeID] {
			c.Negative[nodeID] = val
		}
	}
}

// -------------------------------------------------------------------
// GSet: Grow-Only Set (elements can only be added, never removed)
// -------------------------------------------------------------------

// GSet is a grow-only set CRDT. Used for tracking which posts/comments
// belong to a community (add-only membership).
type GSet struct {
	mu       sync.RWMutex
	Elements map[string]struct{} `json:"elements"`
}

// NewGSet creates a new grow-only set.
func NewGSet() *GSet {
	return &GSet{
		Elements: make(map[string]struct{}),
	}
}

// Add inserts an element into the set.
func (s *GSet) Add(elem string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Elements[elem] = struct{}{}
}

// Contains checks if an element exists in the set.
func (s *GSet) Contains(elem string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.Elements[elem]
	return ok
}

// List returns all elements in the set.
func (s *GSet) List() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]string, 0, len(s.Elements))
	for elem := range s.Elements {
		result = append(result, elem)
	}
	return result
}

// Merge merges another GSet into this one (union).
func (s *GSet) Merge(other *GSet) {
	s.mu.Lock()
	defer s.mu.Unlock()

	other.mu.RLock()
	defer other.mu.RUnlock()

	for elem := range other.Elements {
		s.Elements[elem] = struct{}{}
	}
}

// -------------------------------------------------------------------
// ORSet: Observed-Remove Set (supports add and remove)
// -------------------------------------------------------------------

// ORSetElement is an element with a unique tag for add-wins semantics.
type ORSetElement struct {
	Value string `json:"value"`
	Tag   string `json:"tag"` // unique identifier for this add operation
}

// ORSet is an Observed-Remove Set CRDT. Supports both add and remove.
// Uses add-wins semantics: concurrent add and remove keeps the element.
type ORSet struct {
	mu         sync.RWMutex
	Elements   map[string]map[string]struct{} `json:"elements"`   // value -> set of tags
	Tombstones map[string]map[string]struct{} `json:"tombstones"` // removed tags
}

// NewORSet creates a new OR-Set.
func NewORSet() *ORSet {
	return &ORSet{
		Elements:   make(map[string]map[string]struct{}),
		Tombstones: make(map[string]map[string]struct{}),
	}
}

// Add inserts an element with a unique tag.
func (s *ORSet) Add(value, tag string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.Elements[value]; !ok {
		s.Elements[value] = make(map[string]struct{})
	}
	s.Elements[value][tag] = struct{}{}
}

// Remove removes all current tags for a value (observed remove).
func (s *ORSet) Remove(value string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tags, ok := s.Elements[value]
	if !ok {
		return
	}

	if _, ok := s.Tombstones[value]; !ok {
		s.Tombstones[value] = make(map[string]struct{})
	}

	// Move all current tags to tombstones
	for tag := range tags {
		s.Tombstones[value][tag] = struct{}{}
	}
	delete(s.Elements, value)
}

// Contains checks if a value exists in the set.
func (s *ORSet) Contains(value string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tags, ok := s.Elements[value]
	return ok && len(tags) > 0
}

// List returns all values currently in the set.
func (s *ORSet) List() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]string, 0, len(s.Elements))
	for value, tags := range s.Elements {
		if len(tags) > 0 {
			result = append(result, value)
		}
	}
	return result
}

// Merge merges another ORSet into this one.
func (s *ORSet) Merge(other *ORSet) {
	s.mu.Lock()
	defer s.mu.Unlock()

	other.mu.RLock()
	defer other.mu.RUnlock()

	// Merge tombstones first
	for value, tags := range other.Tombstones {
		if _, ok := s.Tombstones[value]; !ok {
			s.Tombstones[value] = make(map[string]struct{})
		}
		for tag := range tags {
			s.Tombstones[value][tag] = struct{}{}
		}
	}

	// Merge elements (excluding tombstoned tags)
	for value, tags := range other.Elements {
		if _, ok := s.Elements[value]; !ok {
			s.Elements[value] = make(map[string]struct{})
		}
		for tag := range tags {
			// Only add if not tombstoned
			if tombTags, ok := s.Tombstones[value]; ok {
				if _, tombstoned := tombTags[tag]; tombstoned {
					continue
				}
			}
			s.Elements[value][tag] = struct{}{}
		}
	}

	// Clean up own elements that are now tombstoned
	for value, tombTags := range s.Tombstones {
		if elemTags, ok := s.Elements[value]; ok {
			for tag := range tombTags {
				delete(elemTags, tag)
			}
			if len(elemTags) == 0 {
				delete(s.Elements, value)
			}
		}
	}
}

// -------------------------------------------------------------------
// LWWRegister: Last-Writer-Wins Register
// -------------------------------------------------------------------

// LWWRegister is a Last-Writer-Wins register CRDT.
// The value with the latest timestamp wins during merge.
type LWWRegister struct {
	mu        sync.RWMutex
	Value     interface{}   `json:"value"`
	Timestamp time.Time     `json:"timestamp"`
	NodeID    models.NodeID `json:"node_id"`
}

// NewLWWRegister creates a new LWW Register.
func NewLWWRegister(value interface{}, nodeID models.NodeID) *LWWRegister {
	return &LWWRegister{
		Value:     value,
		Timestamp: time.Now(),
		NodeID:    nodeID,
	}
}

// Set updates the register value with current timestamp.
func (r *LWWRegister) Set(value interface{}, nodeID models.NodeID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Value = value
	r.Timestamp = time.Now()
	r.NodeID = nodeID
}

// Get returns the current register value.
func (r *LWWRegister) Get() interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Value
}

// Merge merges another LWWRegister into this one.
// The value with the later timestamp wins; ties broken by NodeID.
func (r *LWWRegister) Merge(other *LWWRegister) {
	r.mu.Lock()
	defer r.mu.Unlock()

	other.mu.RLock()
	defer other.mu.RUnlock()

	if other.Timestamp.After(r.Timestamp) ||
		(other.Timestamp.Equal(r.Timestamp) && other.NodeID > r.NodeID) {
		r.Value = other.Value
		r.Timestamp = other.Timestamp
		r.NodeID = other.NodeID
	}
}

// -------------------------------------------------------------------
// VoteState: Per-content vote tracking using CRDTs
// -------------------------------------------------------------------

// VoteState tracks all votes for a specific post or comment.
// Uses a PNCounter for the aggregate score and a map for per-user votes.
type VoteState struct {
	mu         sync.RWMutex
	TargetHash models.ContentHash             `json:"target_hash"`
	Score      *PNCounter                     `json:"score"`
	UserVotes  map[models.UserID]*LWWRegister `json:"user_votes"` // per-user last vote
}

// NewVoteState creates a new vote state for a content hash.
func NewVoteState(targetHash models.ContentHash) *VoteState {
	return &VoteState{
		TargetHash: targetHash,
		Score:      NewPNCounter(),
		UserVotes:  make(map[models.UserID]*LWWRegister),
	}
}

// ApplyVote applies a user's vote.
func (vs *VoteState) ApplyVote(vote models.Vote, nodeID models.NodeID) {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	// Check if user already voted
	if existing, ok := vs.UserVotes[vote.UserID]; ok {
		oldVal := existing.Get()
		if oldVal != nil {
			oldVote := oldVal.(models.VoteType)
			// Undo previous vote
			if oldVote == models.Upvote {
				vs.Score.Decrement(nodeID)
			} else {
				vs.Score.Increment(nodeID)
			}
		}
	}

	// Apply new vote
	if vote.Value == models.Upvote {
		vs.Score.Increment(nodeID)
	} else {
		vs.Score.Decrement(nodeID)
	}

	// Record user's vote
	if _, ok := vs.UserVotes[vote.UserID]; !ok {
		vs.UserVotes[vote.UserID] = NewLWWRegister(vote.Value, nodeID)
	} else {
		vs.UserVotes[vote.UserID].Set(vote.Value, nodeID)
	}
}

// GetScore returns the current vote score.
func (vs *VoteState) GetScore() int64 {
	return vs.Score.Value()
}
