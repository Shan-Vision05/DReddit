package crdt

import (
	"testing"

	"github.com/Shan-Vision05/DReddit/internal/models"
)

// =============================================================
// PNCounter tests
// =============================================================

func TestPNCounter_IncrementDecrement(t *testing.T) {
	c := NewPNCounter()
	c.Increment("node1")
	c.Increment("node1")
	c.Increment("node1")
	c.Decrement("node1")

	if got := c.Value(); got != 2 {
		t.Errorf("PNCounter.Value() = %d, want 2", got)
	}
}

func TestPNCounter_MultiNode(t *testing.T) {
	c := NewPNCounter()
	c.Increment("node1")
	c.Increment("node2")
	c.Increment("node2")
	c.Decrement("node3")

	// 1 + 2 - 1 = 2
	if got := c.Value(); got != 2 {
		t.Errorf("PNCounter.Value() = %d, want 2", got)
	}
}

func TestPNCounter_Merge(t *testing.T) {
	c1 := NewPNCounter()
	c2 := NewPNCounter()

	c1.Increment("node1")
	c1.Increment("node1")
	c2.Increment("node2")
	c2.Increment("node2")
	c2.Increment("node2")

	c1.Merge(c2)

	// node1=2, node2=3 → 5
	if got := c1.Value(); got != 5 {
		t.Errorf("After merge, PNCounter.Value() = %d, want 5", got)
	}
}

func TestPNCounter_MergeIdempotent(t *testing.T) {
	c1 := NewPNCounter()
	c2 := NewPNCounter()

	c1.Increment("node1")
	c2.Increment("node1") // same node, same count

	c1.Merge(c2)

	// max(1,1) = 1, not 1+1
	if got := c1.Value(); got != 1 {
		t.Errorf("Idempotent merge: PNCounter.Value() = %d, want 1", got)
	}
}

// =============================================================
// GSet tests
// =============================================================

func TestGSet_AddContains(t *testing.T) {
	s := NewGSet()
	s.Add("post1")
	s.Add("post2")

	if !s.Contains("post1") {
		t.Error("GSet should contain 'post1'")
	}
	if s.Contains("post3") {
		t.Error("GSet should not contain 'post3'")
	}
}

func TestGSet_Merge(t *testing.T) {
	s1 := NewGSet()
	s2 := NewGSet()

	s1.Add("a")
	s1.Add("b")
	s2.Add("b")
	s2.Add("c")

	s1.Merge(s2)

	if len(s1.List()) != 3 {
		t.Errorf("After merge, GSet should have 3 elements, got %d", len(s1.List()))
	}
}

// =============================================================
// ORSet tests
// =============================================================

func TestORSet_AddRemove(t *testing.T) {
	s := NewORSet()
	s.Add("user1", "tag1")
	s.Add("user2", "tag2")

	if !s.Contains("user1") {
		t.Error("ORSet should contain 'user1'")
	}

	s.Remove("user1")

	if s.Contains("user1") {
		t.Error("ORSet should not contain 'user1' after remove")
	}
	if !s.Contains("user2") {
		t.Error("ORSet should still contain 'user2'")
	}
}

func TestORSet_ConcurrentAddWins(t *testing.T) {
	// Two replicas: s1 removes, s2 adds concurrently → add wins
	s1 := NewORSet()
	s2 := NewORSet()

	s1.Add("user1", "tag1")
	s2.Add("user1", "tag2") // concurrent add with different tag

	s1.Remove("user1") // removes tag1 only

	s1.Merge(s2) // tag2 from s2 should survive

	if !s1.Contains("user1") {
		t.Error("After concurrent add+remove merge, ORSet should contain 'user1' (add-wins)")
	}
}

// =============================================================
// LWWRegister tests
// =============================================================

func TestLWWRegister_LastWriterWins(t *testing.T) {
	r1 := NewLWWRegister("value1", "node1")
	r2 := NewLWWRegister("value2", "node2")
	// r2 created after r1 → r2 wins

	r1.Merge(r2)

	if r1.Get() != "value2" {
		t.Errorf("LWWRegister should have value2 after merge, got %v", r1.Get())
	}
}

// =============================================================
// VoteState tests
// =============================================================

func TestVoteState_ApplyVote(t *testing.T) {
	vs := NewVoteState("post123")

	vs.ApplyVote(models.Vote{TargetHash: "post123", UserID: "user1", Value: models.Upvote}, "node1")
	vs.ApplyVote(models.Vote{TargetHash: "post123", UserID: "user2", Value: models.Upvote}, "node1")
	vs.ApplyVote(models.Vote{TargetHash: "post123", UserID: "user3", Value: models.Downvote}, "node1")

	// 2 up - 1 down = 1
	if got := vs.GetScore(); got != 1 {
		t.Errorf("VoteState score = %d, want 1", got)
	}
}

func TestVoteState_ChangeVote(t *testing.T) {
	vs := NewVoteState("post123")

	vs.ApplyVote(models.Vote{TargetHash: "post123", UserID: "user1", Value: models.Upvote}, "node1")
	if got := vs.GetScore(); got != 1 {
		t.Errorf("After upvote, score = %d, want 1", got)
	}

	// Same user changes to downvote
	vs.ApplyVote(models.Vote{TargetHash: "post123", UserID: "user1", Value: models.Downvote}, "node1")
	if got := vs.GetScore(); got != -1 {
		t.Errorf("After changing to downvote, score = %d, want -1", got)
	}
}
