// Package consensus implements the Raft-based consensus layer for moderation.
//
// Moderation actions (post removals, user bans) require strong consistency.
// Each community replica group maintains a replicated log using Raft
// to serialize these actions in a single agreed-upon order.
package consensus

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/shan/dreddit/internal/models"
)

// -------------------------------------------------------------------
// Raft state
// -------------------------------------------------------------------

// RaftState represents the state of a Raft node.
type RaftState int

const (
	Follower RaftState = iota
	Candidate
	Leader
)

func (s RaftState) String() string {
	switch s {
	case Follower:
		return "Follower"
	case Candidate:
		return "Candidate"
	case Leader:
		return "Leader"
	default:
		return "Unknown"
	}
}

// -------------------------------------------------------------------
// Log entry
// -------------------------------------------------------------------

// LogEntry is a single entry in the Raft replicated log.
type LogEntry struct {
	Index   uint64                  `json:"index"`
	Term    uint64                  `json:"term"`
	Command models.ModerationAction `json:"command"`
}

// -------------------------------------------------------------------
// RPC types
// -------------------------------------------------------------------

// RequestVoteRequest is sent by candidates to request votes.
type RequestVoteRequest struct {
	Term         uint64        `json:"term"`
	CandidateID  models.NodeID `json:"candidate_id"`
	LastLogIndex uint64        `json:"last_log_index"`
	LastLogTerm  uint64        `json:"last_log_term"`
}

// RequestVoteResponse is the reply to a vote request.
type RequestVoteResponse struct {
	Term        uint64 `json:"term"`
	VoteGranted bool   `json:"vote_granted"`
}

// AppendEntriesRequest is used by the leader for log replication and heartbeats.
type AppendEntriesRequest struct {
	Term         uint64        `json:"term"`
	LeaderID     models.NodeID `json:"leader_id"`
	PrevLogIndex uint64        `json:"prev_log_index"`
	PrevLogTerm  uint64        `json:"prev_log_term"`
	Entries      []LogEntry    `json:"entries"`
	LeaderCommit uint64        `json:"leader_commit"`
}

// AppendEntriesResponse is the reply to an AppendEntries RPC.
type AppendEntriesResponse struct {
	Term    uint64 `json:"term"`
	Success bool   `json:"success"`
}

// -------------------------------------------------------------------
// Transport interface
// -------------------------------------------------------------------

// RaftTransport defines the interface for Raft RPCs between nodes.
type RaftTransport interface {
	RequestVote(peer models.NodeID, req *RequestVoteRequest) (*RequestVoteResponse, error)
	AppendEntries(peer models.NodeID, req *AppendEntriesRequest) (*AppendEntriesResponse, error)
}

// -------------------------------------------------------------------
// RaftNode
// -------------------------------------------------------------------

// RaftNode implements the Raft consensus protocol for a single community's
// moderation log.
type RaftNode struct {
	mu sync.RWMutex

	// Identity
	id          models.NodeID
	communityID models.CommunityID

	// Persistent state
	currentTerm uint64
	votedFor    models.NodeID
	log         []LogEntry

	// Volatile state
	commitIndex uint64
	lastApplied uint64
	state       RaftState

	// Leader-only volatile state
	nextIndex  map[models.NodeID]uint64
	matchIndex map[models.NodeID]uint64

	// Cluster membership
	peers []models.NodeID

	// Transport for RPCs
	transport RaftTransport

	// Channels
	applyCh       chan LogEntry
	stopCh        chan struct{}
	heartbeatCh   chan struct{}
	voteGrantedCh chan struct{}

	// Timing
	electionTimeout  time.Duration
	heartbeatTimeout time.Duration
}

// NewRaftNode creates a new Raft node for a community's moderation log.
func NewRaftNode(id models.NodeID, communityID models.CommunityID, peers []models.NodeID, transport RaftTransport) *RaftNode {
	return &RaftNode{
		id:               id,
		communityID:      communityID,
		currentTerm:      0,
		votedFor:         "",
		log:              make([]LogEntry, 0),
		commitIndex:      0,
		lastApplied:      0,
		state:            Follower,
		nextIndex:        make(map[models.NodeID]uint64),
		matchIndex:       make(map[models.NodeID]uint64),
		peers:            peers,
		transport:        transport,
		applyCh:          make(chan LogEntry, 100),
		stopCh:           make(chan struct{}),
		heartbeatCh:      make(chan struct{}, 1),
		voteGrantedCh:    make(chan struct{}, 1),
		electionTimeout:  time.Duration(150+rand.Intn(150)) * time.Millisecond,
		heartbeatTimeout: 50 * time.Millisecond,
	}
}

// Start starts the Raft node's main loop.
func (rn *RaftNode) Start() {
	go rn.run()
	log.Printf("[Raft %s/%s] Started as %s", rn.communityID, rn.id, rn.state)
}

// Stop stops the Raft node.
func (rn *RaftNode) Stop() {
	close(rn.stopCh)
}

// ApplyCh returns the channel that receives committed log entries.
func (rn *RaftNode) ApplyCh() <-chan LogEntry {
	return rn.applyCh
}

// GetState returns the current Raft state.
func (rn *RaftNode) GetState() (uint64, RaftState) {
	rn.mu.RLock()
	defer rn.mu.RUnlock()
	return rn.currentTerm, rn.state
}

// IsLeader returns true if this node is the leader.
func (rn *RaftNode) IsLeader() bool {
	rn.mu.RLock()
	defer rn.mu.RUnlock()
	return rn.state == Leader
}

// GetLeaderID returns the current leader ID (empty if unknown).
func (rn *RaftNode) GetLeaderID() models.NodeID {
	rn.mu.RLock()
	defer rn.mu.RUnlock()
	if rn.state == Leader {
		return rn.id
	}
	return ""
}

// -------------------------------------------------------------------
// Propose a new moderation action
// -------------------------------------------------------------------

// Propose proposes a new moderation action to be appended to the log.
// Only the leader can accept proposals.
func (rn *RaftNode) Propose(action models.ModerationAction) error {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	if rn.state != Leader {
		return fmt.Errorf("not the leader (current state: %s)", rn.state)
	}

	entry := LogEntry{
		Index:   rn.lastLogIndex() + 1,
		Term:    rn.currentTerm,
		Command: action,
	}

	rn.log = append(rn.log, entry)
	log.Printf("[Raft %s/%s] Proposed entry at index %d", rn.communityID, rn.id, entry.Index)

	// Trigger replication
	go rn.replicateToAll()

	return nil
}

// -------------------------------------------------------------------
// Main event loop
// -------------------------------------------------------------------

func (rn *RaftNode) run() {
	for {
		select {
		case <-rn.stopCh:
			return
		default:
		}

		rn.mu.RLock()
		state := rn.state
		rn.mu.RUnlock()

		switch state {
		case Follower:
			rn.runFollower()
		case Candidate:
			rn.runCandidate()
		case Leader:
			rn.runLeader()
		}
	}
}

func (rn *RaftNode) runFollower() {
	timeout := rn.randomElectionTimeout()
	select {
	case <-rn.stopCh:
		return
	case <-rn.heartbeatCh:
		// Received heartbeat, reset timer
	case <-time.After(timeout):
		// Election timeout, become candidate
		rn.mu.Lock()
		rn.state = Candidate
		rn.mu.Unlock()
		log.Printf("[Raft %s/%s] Election timeout, becoming Candidate", rn.communityID, rn.id)
	}
}

func (rn *RaftNode) runCandidate() {
	rn.mu.Lock()
	rn.currentTerm++
	rn.votedFor = rn.id
	term := rn.currentTerm
	lastLogIndex := rn.lastLogIndex()
	lastLogTerm := rn.lastLogTerm()
	rn.mu.Unlock()

	log.Printf("[Raft %s/%s] Starting election for term %d", rn.communityID, rn.id, term)

	votes := 1 // Vote for self
	voteCh := make(chan bool, len(rn.peers))

	for _, peer := range rn.peers {
		go func(p models.NodeID) {
			if rn.transport == nil {
				voteCh <- false
				return
			}
			resp, err := rn.transport.RequestVote(p, &RequestVoteRequest{
				Term:         term,
				CandidateID:  rn.id,
				LastLogIndex: lastLogIndex,
				LastLogTerm:  lastLogTerm,
			})
			if err != nil {
				voteCh <- false
				return
			}
			if resp.Term > term {
				rn.mu.Lock()
				rn.currentTerm = resp.Term
				rn.state = Follower
				rn.votedFor = ""
				rn.mu.Unlock()
				voteCh <- false
				return
			}
			voteCh <- resp.VoteGranted
		}(peer)
	}

	timeout := rn.randomElectionTimeout()
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	majority := (len(rn.peers)+1)/2 + 1

	for i := 0; i < len(rn.peers); i++ {
		select {
		case <-rn.stopCh:
			return
		case granted := <-voteCh:
			if granted {
				votes++
			}
			if votes >= majority {
				rn.mu.Lock()
				if rn.state == Candidate && rn.currentTerm == term {
					rn.state = Leader
					// Initialize leader state
					for _, p := range rn.peers {
						rn.nextIndex[p] = rn.lastLogIndex() + 1
						rn.matchIndex[p] = 0
					}
					log.Printf("[Raft %s/%s] Won election for term %d", rn.communityID, rn.id, term)
				}
				rn.mu.Unlock()
				return
			}
		case <-timer.C:
			// Election timeout, start new election
			return
		case <-rn.heartbeatCh:
			// Received heartbeat from leader, step down
			rn.mu.Lock()
			rn.state = Follower
			rn.mu.Unlock()
			return
		}
	}
}

func (rn *RaftNode) runLeader() {
	// Send initial heartbeat
	rn.replicateToAll()

	ticker := time.NewTicker(rn.heartbeatTimeout)
	defer ticker.Stop()

	select {
	case <-rn.stopCh:
		return
	case <-ticker.C:
		rn.replicateToAll()
	}
}

// -------------------------------------------------------------------
// Log replication
// -------------------------------------------------------------------

func (rn *RaftNode) replicateToAll() {
	rn.mu.RLock()
	if rn.state != Leader {
		rn.mu.RUnlock()
		return
	}
	peers := make([]models.NodeID, len(rn.peers))
	copy(peers, rn.peers)
	rn.mu.RUnlock()

	for _, peer := range peers {
		go rn.replicateTo(peer)
	}
}

func (rn *RaftNode) replicateTo(peer models.NodeID) {
	if rn.transport == nil {
		return
	}

	rn.mu.RLock()
	nextIdx := rn.nextIndex[peer]
	prevLogIndex := uint64(0)
	prevLogTerm := uint64(0)
	if nextIdx > 1 && int(nextIdx-1) <= len(rn.log) {
		prevEntry := rn.log[nextIdx-2]
		prevLogIndex = prevEntry.Index
		prevLogTerm = prevEntry.Term
	}

	var entries []LogEntry
	if int(nextIdx-1) < len(rn.log) {
		entries = rn.log[nextIdx-1:]
	}

	req := &AppendEntriesRequest{
		Term:         rn.currentTerm,
		LeaderID:     rn.id,
		PrevLogIndex: prevLogIndex,
		PrevLogTerm:  prevLogTerm,
		Entries:      entries,
		LeaderCommit: rn.commitIndex,
	}
	rn.mu.RUnlock()

	resp, err := rn.transport.AppendEntries(peer, req)
	if err != nil {
		return
	}

	rn.mu.Lock()
	defer rn.mu.Unlock()

	if resp.Term > rn.currentTerm {
		rn.currentTerm = resp.Term
		rn.state = Follower
		rn.votedFor = ""
		return
	}

	if resp.Success {
		if len(entries) > 0 {
			rn.nextIndex[peer] = entries[len(entries)-1].Index + 1
			rn.matchIndex[peer] = entries[len(entries)-1].Index
		}
		rn.updateCommitIndex()
	} else {
		if rn.nextIndex[peer] > 1 {
			rn.nextIndex[peer]--
		}
	}
}

func (rn *RaftNode) updateCommitIndex() {
	for n := rn.commitIndex + 1; n <= rn.lastLogIndex(); n++ {
		if int(n) > len(rn.log) {
			break
		}
		entry := rn.log[n-1]
		if entry.Term != rn.currentTerm {
			continue
		}

		replicaCount := 1 // self
		for _, peer := range rn.peers {
			if rn.matchIndex[peer] >= n {
				replicaCount++
			}
		}

		majority := (len(rn.peers)+1)/2 + 1
		if replicaCount >= majority {
			rn.commitIndex = n
			rn.applyCommitted()
		}
	}
}

func (rn *RaftNode) applyCommitted() {
	for rn.lastApplied < rn.commitIndex {
		rn.lastApplied++
		if int(rn.lastApplied) <= len(rn.log) {
			entry := rn.log[rn.lastApplied-1]
			select {
			case rn.applyCh <- entry:
			default:
				log.Printf("[Raft %s/%s] Apply channel full, dropping entry %d",
					rn.communityID, rn.id, entry.Index)
			}
		}
	}
}

// -------------------------------------------------------------------
// Handle incoming RPCs
// -------------------------------------------------------------------

// HandleRequestVote processes a RequestVote RPC.
func (rn *RaftNode) HandleRequestVote(req *RequestVoteRequest) *RequestVoteResponse {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	resp := &RequestVoteResponse{
		Term:        rn.currentTerm,
		VoteGranted: false,
	}

	if req.Term < rn.currentTerm {
		return resp
	}

	if req.Term > rn.currentTerm {
		rn.currentTerm = req.Term
		rn.state = Follower
		rn.votedFor = ""
	}

	// Grant vote if we haven't voted yet and candidate's log is up-to-date
	if (rn.votedFor == "" || rn.votedFor == req.CandidateID) &&
		rn.isLogUpToDate(req.LastLogIndex, req.LastLogTerm) {
		rn.votedFor = req.CandidateID
		resp.VoteGranted = true
		resp.Term = rn.currentTerm

		// Reset election timer
		select {
		case rn.voteGrantedCh <- struct{}{}:
		default:
		}
	}

	return resp
}

// HandleAppendEntries processes an AppendEntries RPC.
func (rn *RaftNode) HandleAppendEntries(req *AppendEntriesRequest) *AppendEntriesResponse {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	resp := &AppendEntriesResponse{
		Term:    rn.currentTerm,
		Success: false,
	}

	if req.Term < rn.currentTerm {
		return resp
	}

	if req.Term >= rn.currentTerm {
		rn.currentTerm = req.Term
		rn.state = Follower
		rn.votedFor = ""
	}

	// Signal heartbeat received
	select {
	case rn.heartbeatCh <- struct{}{}:
	default:
	}

	// Check prev log consistency
	if req.PrevLogIndex > 0 {
		if int(req.PrevLogIndex) > len(rn.log) {
			return resp
		}
		if rn.log[req.PrevLogIndex-1].Term != req.PrevLogTerm {
			// Delete conflicting entry and all after
			rn.log = rn.log[:req.PrevLogIndex-1]
			return resp
		}
	}

	// Append new entries
	for _, entry := range req.Entries {
		if int(entry.Index) <= len(rn.log) {
			if rn.log[entry.Index-1].Term != entry.Term {
				rn.log = rn.log[:entry.Index-1]
				rn.log = append(rn.log, entry)
			}
		} else {
			rn.log = append(rn.log, entry)
		}
	}

	// Update commit index
	if req.LeaderCommit > rn.commitIndex {
		lastNew := rn.lastLogIndex()
		if req.LeaderCommit < lastNew {
			rn.commitIndex = req.LeaderCommit
		} else {
			rn.commitIndex = lastNew
		}
		rn.applyCommitted()
	}

	resp.Success = true
	return resp
}

// -------------------------------------------------------------------
// Helpers
// -------------------------------------------------------------------

func (rn *RaftNode) lastLogIndex() uint64 {
	if len(rn.log) == 0 {
		return 0
	}
	return rn.log[len(rn.log)-1].Index
}

func (rn *RaftNode) lastLogTerm() uint64 {
	if len(rn.log) == 0 {
		return 0
	}
	return rn.log[len(rn.log)-1].Term
}

func (rn *RaftNode) isLogUpToDate(lastIndex, lastTerm uint64) bool {
	myLastTerm := rn.lastLogTerm()
	myLastIndex := rn.lastLogIndex()

	if lastTerm != myLastTerm {
		return lastTerm >= myLastTerm
	}
	return lastIndex >= myLastIndex
}

func (rn *RaftNode) randomElectionTimeout() time.Duration {
	return time.Duration(150+rand.Intn(150)) * time.Millisecond
}

// GetLog returns a copy of the Raft log (for debugging/inspection).
func (rn *RaftNode) GetLog() []LogEntry {
	rn.mu.RLock()
	defer rn.mu.RUnlock()

	result := make([]LogEntry, len(rn.log))
	copy(result, rn.log)
	return result
}

// GetLogJSON returns the log as pretty-printed JSON.
func (rn *RaftNode) GetLogJSON() string {
	entries := rn.GetLog()
	b, _ := json.MarshalIndent(entries, "", "  ")
	return string(b)
}
