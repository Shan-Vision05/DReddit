package consensus

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/hashicorp/raft"

	"github.com/Shan-Vision05/Distributed-Reddit/internal/models"
)

type ModerationFSM struct {
	mu  sync.RWMutex
	log []models.ModerationAction
}

func NewModerationFSM() *ModerationFSM {
	return &ModerationFSM{
		log: make([]models.ModerationAction, 0),
	}
}

func (f *ModerationFSM) Apply(l *raft.Log) interface{} {
	var action models.ModerationAction
	if err := json.Unmarshal(l.Data, &action); err != nil {
		return fmt.Errorf("failed to unmarshal moderation action: %w", err)
	}
	
	// Track the Raft log index if it exists in your model
	action.LogIndex = l.Index

	f.mu.Lock()
	f.log = append(f.log, action)
	f.mu.Unlock()

	return nil
}

func (f *ModerationFSM) Snapshot() (raft.FSMSnapshot, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	
	// Create a deep copy of the logs so Raft can safely write it to the hard drive in the background
	clone := make([]models.ModerationAction, len(f.log))
	copy(clone, f.log)
	
	return &fsmSnapshot{actions: clone}, nil
}

func (f *ModerationFSM) Restore(rc io.ReadCloser) error {
	defer rc.Close()
	
	// When the server boots up, Raft will stream the saved JSON file back into this function
	var actions []models.ModerationAction
	if err := json.NewDecoder(rc).Decode(&actions); err != nil {
		return err
	}
	
	f.mu.Lock()
	f.log = actions
	f.mu.Unlock()
	
	return nil
}

func (f *ModerationFSM) GetLog() []models.ModerationAction {
	f.mu.RLock()
	defer f.mu.RUnlock()
	
	result := make([]models.ModerationAction, len(f.log))
	copy(result, f.log)
	
	return result
}

// --- Snapshot Implementation ---

type fsmSnapshot struct {
	actions []models.ModerationAction
}

func (s *fsmSnapshot) Persist(sink raft.SnapshotSink) error {
	data, err := json.Marshal(s.actions)
	if err != nil {
		sink.Cancel()
		return err
	}
	
	if _, err := sink.Write(data); err != nil {
		sink.Cancel()
		return err
	}
	
	return sink.Close()
}

func (s *fsmSnapshot) Release() {}