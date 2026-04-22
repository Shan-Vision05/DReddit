package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/Shan-Vision05/Distributed-Reddit/internal/models"
	"github.com/Shan-Vision05/Distributed-Reddit/internal/node"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func waitForCondition(t *testing.T, desc string, fn func() bool, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for: %s", desc)
}

// makeTestNode creates a node with a unique ID and wires cleanup.
// makeTestNode creates a node with a unique ID, isolated temp data dir, and wires cleanup.
func makeTestNode(t *testing.T, id string) *node.Node {
	t.Helper()
	tmpDir := t.TempDir()
	cfg := node.NodeConfig{DataDir: tmpDir}
	n, err := node.NewNodeWithConfig(models.NodeID(id), "127.0.0.1", cfg)
	if err != nil {
		t.Fatalf("NewNode(%s): %v", id, err)
	}
	t.Cleanup(func() {
		n.Gossip.Shutdown()
	})
	return n
}

func communityOwnedBy(t *testing.T, owner *node.Node, prefix string) models.CommunityID {
	t.Helper()
	for index := 0; index < 2048; index++ {
		communityID := models.CommunityID(fmt.Sprintf("%s-%d", prefix, index))
		primary, ok := owner.DHT.GetPrimaryNode(communityID)
		if ok && primary == owner.NodeID {
			return communityID
		}
	}
	t.Fatalf("unable to find community with primary owner %s", owner.NodeID)
	return ""
}

func joinCommunityOnPrimaryAndFollower(t *testing.T, primary, follower *node.Node, communityID models.CommunityID) {
	t.Helper()
	if err := primary.JoinCommunity(communityID); err != nil {
		t.Fatalf("primary.JoinCommunity: %v", err)
	}
	waitForCondition(t, fmt.Sprintf("%s becomes Raft leader", primary.NodeID),
		func() bool { return primary.IsRaftLeader(communityID) }, 5*time.Second)

	waitForCondition(t, fmt.Sprintf("%s learns primary announcement", follower.NodeID),
		func() bool { return follower.HasKnownPrimaryForCommunity(communityID) }, 5*time.Second)

	if err := follower.JoinCommunityAsFollower(communityID); err != nil {
		t.Fatalf("follower.JoinCommunityAsFollower: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Bug 2: Gossip and community manager share the same ContentStore
// ---------------------------------------------------------------------------

// TestBug2_GossipAndCommunityManagerShareStore verifies that a post received
// via gossip on a node is visible through that node's community manager.
//
// Before the fix: gossip wrote to a "dead" store that the community manager
// never read from. After the fix, both use the same shared ContentStore.
func TestBug2_GossipAndCommunityManagerShareStore(t *testing.T) {
	nodeA := makeTestNode(t, fmt.Sprintf("bug2-A-%d", time.Now().UnixNano()))
	nodeB := makeTestNode(t, fmt.Sprintf("bug2-B-%d", time.Now().UnixNano()))

	// Form a 2-node gossip cluster.
	addrA := nodeA.Gossip.LocalNode().Address()
	if _, err := nodeB.Gossip.Join([]string{addrA}); err != nil {
		t.Fatalf("nodeB.Join: %v", err)
	}
	waitForCondition(t, "2 gossip members on nodeA",
		func() bool { return nodeA.Gossip.NumMembers() >= 2 }, 5*time.Second)

	communityID := communityOwnedBy(t, nodeA, "shared-store-test")
	joinCommunityOnPrimaryAndFollower(t, nodeA, nodeB, communityID)

	managerA, err := nodeA.GetCommunity(communityID)
	if err != nil {
		t.Fatal(err)
	}
	managerB, err := nodeB.GetCommunity(communityID)
	if err != nil {
		t.Fatal(err)
	}

	// Node A creates a post — this gossip-broadcasts it to node B.
	post := &models.Post{
		CommunityID: communityID,
		AuthorID:    "user1",
		Title:       "Hello from A",
		Body:        "Propagated via gossip",
		CreatedAt:   time.Now(),
	}
	post.Hash = post.ComputeHash()
	if _, err := managerA.CreatePost(post); err != nil {
		t.Fatalf("CreatePost: %v", err)
	}

	// The post should reach node B via gossip and be visible through the
	// community manager (Bug 2 fix: gossip and manager share the same store).
	waitForCondition(t, "post visible on nodeB via community manager",
		func() bool {
			posts, _ := managerB.GetPosts()
			return len(posts) > 0
		}, 5*time.Second)

	posts, _ := managerB.GetPosts()
	if len(posts) != 1 {
		t.Fatalf("Bug 2 not fixed: expected 1 post on nodeB, got %d", len(posts))
	}
	if posts[0].Title != "Hello from A" {
		t.Errorf("unexpected post title: %q", posts[0].Title)
	}
}

// ---------------------------------------------------------------------------
// Bug 5: Community membership propagates via gossip
// ---------------------------------------------------------------------------

// TestBug5_CommunityAnnouncePropagatesToPeer verifies that when a node joins
// a community it immediately broadcasts a CommunityAnnounce message over
// gossip, and the receiving node updates its DHT assignment table.
//
// Before the fix: DHT.Announce was a no-op log print.
// After the fix: BroadcastCommunityAnnounce sends a real gossip message.
func TestBug5_CommunityAnnouncePropagatesToPeer(t *testing.T) {
	nodeA := makeTestNode(t, fmt.Sprintf("bug5-A-%d", time.Now().UnixNano()))
	nodeB := makeTestNode(t, fmt.Sprintf("bug5-B-%d", time.Now().UnixNano()))

	// Form a 2-node gossip cluster.
	addrA := nodeA.Gossip.LocalNode().Address()
	if _, err := nodeB.Gossip.Join([]string{addrA}); err != nil {
		t.Fatalf("nodeB.Join: %v", err)
	}
	waitForCondition(t, "2 gossip members",
		func() bool { return nodeA.Gossip.NumMembers() >= 2 }, 5*time.Second)

	communityID := communityOwnedBy(t, nodeA, "announce-test")

	// Node A joins the community and broadcasts the announce.
	if err := nodeA.JoinCommunity(communityID); err != nil {
		t.Fatalf("nodeA.JoinCommunity: %v", err)
	}

	// Wait for node B to learn that node A is the primary host via announce.
	waitForCondition(t, "nodeB knows the primary host announcement",
		func() bool { return nodeB.HasKnownPrimaryForCommunity(communityID) }, 5*time.Second)

	if !nodeB.HasKnownPrimaryForCommunity(communityID) {
		t.Errorf("Bug 5 not fixed: nodeB did not record nodeA as the known primary host for %s", communityID)
	}
}

// ---------------------------------------------------------------------------
// Bug 1: Multi-node Raft cluster formation
// ---------------------------------------------------------------------------

// TestBug1_MultiNodeRaftCluster verifies that a two-node Raft cluster can be
// formed, that the follower replicates the moderation log, and that proposal
// on the leader is visible on the follower.
//
// Before the fix: every node bootstrapped its own single-node Raft cluster.
// After the fix: JoinCommunityAsFollower + AddRaftPeer form a real 2-node cluster.
func TestBug1_MultiNodeRaftCluster(t *testing.T) {
	nodeA := makeTestNode(t, fmt.Sprintf("bug1-A-%d", time.Now().UnixNano()))
	nodeB := makeTestNode(t, fmt.Sprintf("bug1-B-%d", time.Now().UnixNano()))

	// Form a gossip cluster so peer-join callbacks run.
	addrA := nodeA.Gossip.LocalNode().Address()
	if _, err := nodeB.Gossip.Join([]string{addrA}); err != nil {
		t.Fatalf("nodeB.Join: %v", err)
	}
	waitForCondition(t, "2 gossip members", func() bool {
		return nodeA.Gossip.NumMembers() >= 2 && nodeB.Gossip.NumMembers() >= 2
	}, 5*time.Second)

	communityID := communityOwnedBy(t, nodeA, "raft-cluster-test")
	joinCommunityOnPrimaryAndFollower(t, nodeA, nodeB, communityID)

	// Get B's Raft address and have A add B as a voter.
	raftAddrB, err := nodeB.GetRaftAddr(communityID)
	if err != nil {
		t.Fatalf("nodeB.GetRaftAddr: %v", err)
	}
	if err := nodeA.AddRaftPeer(communityID, nodeB.NodeID, raftAddrB); err != nil {
		t.Fatalf("nodeA.AddRaftPeer: %v", err)
	}

	// Wait for B to become part of the cluster (non-Leader, but known state).
	waitForCondition(t, "nodeB has a Raft state (Follower/Candidate)",
		func() bool {
			raftNodeB, _ := nodeB.GetRaftAddr(communityID)
			return raftNodeB != "" // address is set means raft node exists
		}, 5*time.Second)

	// Propose a moderation action on the leader (node A).
	managerA, err := nodeA.GetCommunity(communityID)
	if err != nil {
		t.Fatal(err)
	}
	action := models.ModerationAction{
		ID:          "test-action-1",
		CommunityID: communityID,
		ModeratorID: "admin",
		ActionType:  models.ModRemovePost,
		TargetHash:  "somehash123",
		Reason:      "test",
		Timestamp:   time.Now(),
	}
	if err := managerA.Moderate(action); err != nil {
		t.Fatalf("managerA.Moderate: %v", err)
	}

	// Wait for the log entry to replicate to node B.
	managerB, err := nodeB.GetCommunity(communityID)
	if err != nil {
		t.Fatal(err)
	}
	waitForCondition(t, "moderation log replicated to nodeB",
		func() bool { return len(managerB.GetModerationLog()) >= 1 },
		5*time.Second)

	logB := managerB.GetModerationLog()
	if len(logB) != 1 {
		t.Fatalf("Bug 1 not fixed: expected 1 log entry on nodeB, got %d", len(logB))
	}
	if logB[0].ActionType != models.ModRemovePost {
		t.Errorf("replicated action type mismatch: got %q, want %q",
			logB[0].ActionType, models.ModRemovePost)
	}
	if logB[0].TargetHash != "somehash123" {
		t.Errorf("replicated TargetHash mismatch: got %q, want %q",
			logB[0].TargetHash, "somehash123")
	}
	t.Logf("SUCCESS: Raft log entry replicated from nodeA to nodeB in a 2-node cluster")
}
