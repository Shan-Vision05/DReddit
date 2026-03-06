# DReddit — Project Milestones

## Milestone Overview

| # | Milestone | Status | Description |
|---|-----------|--------|-------------|
| 1 | Core Data Models & Config | **DONE** | Type definitions, content hashing, configuration |
| 2 | CRDT Implementations + Tests | **DONE** | PNCounter, GSet, ORSet, LWWRegister, VoteState |
| 3 | Content-Addressed Storage | **DONE** | SHA-256 content store with persistence |
| 4 | Raft Consensus (Moderation) | **DONE** | Full Raft protocol: elections, log replication, commit |
| 5 | Network Layer (Gossip + Peers) | **DONE** | PeerManager, GossipProtocol, InMemoryRaftTransport |
| 6 | DHT (Community Discovery) | **DONE** | Kademlia-style XOR distance, community assignment |
| 7 | Community Manager | **DONE** | CRUD for communities, posts, comments, votes, moderation |
| 8 | Node Orchestrator | **DONE** | Ties all components together, lifecycle management |
| 9 | HTTP REST API | **DONE** | Full CRUD endpoints for all operations |
| 10 | CLI Entry Point | **DONE** | Flag parsing, config loading, graceful shutdown |

---

## Detailed Milestone Breakdown

### Milestone 1: Core Data Models & Config (DONE)
- `internal/models/models.go` — NodeID, CommunityID, ContentHash, UserID
- Community, Post, Comment, Vote, ModerationAction, NodeInfo types
- `ComputeHash()` for content-addressed identification
- `internal/config/config.go` — Config struct with defaults, JSON load/save

### Milestone 2: CRDT Implementations + Tests (DONE)
- `internal/crdt/crdt.go` — Four CRDT types + composite VoteState
  - **PNCounter**: positive-negative counter for vote tallying
  - **GSet**: grow-only set for banned users
  - **ORSet**: observed-remove set with add-wins for membership
  - **LWWRegister**: last-writer-wins for per-user vote tracking
  - **VoteState**: composite type tracking all votes on a content hash
- `internal/crdt/crdt_test.go` — 10 tests covering all CRDTs
  - Merge correctness, idempotency, concurrent add-wins semantics

### Milestone 3: Content-Addressed Storage (DONE)
- `internal/storage/content_store.go`
- In-memory maps with indices (community->posts, post->comments)
- SHA-256 hash-based identification
- JSON persistence to disk
- CRDT-based vote state per content hash

### Milestone 4: Raft Consensus (DONE)
- `internal/consensus/raft.go`
- Full Raft implementation: Follower, Candidate, Leader states
- RequestVote and AppendEntries RPCs
- Leader election with randomized timeouts
- Log replication and commit index tracking
- `Propose()` API for moderation actions
- Apply channel for committed entries

### Milestone 5: Network Layer (DONE)
- `internal/network/network.go`
- **PeerManager**: add/remove/alive tracking, connection state
- **GossipProtocol**: periodic anti-entropy rounds
- **InMemoryRaftTransport**: Raft RPC transport for testing

### Milestone 6: DHT (DONE)
- `internal/dht/dht.go`
- Kademlia-inspired XOR distance metric
- Consistent hashing for community-to-node assignment
- Community registration and lookup

### Milestone 7: Community Manager (DONE)
- `internal/community/community.go`
- Community lifecycle (create, list, get)
- Post/comment creation with ban checking
- Vote application via content store
- Moderation actions (remove post, ban user, etc.)

### Milestone 8: Node Orchestrator (DONE)
- `internal/node/node.go`
- Ties CommunityManager + PeerManager + DHT together
- Start/Stop lifecycle, bootstrap node connection
- CreateCommunity, JoinPeer high-level operations

### Milestone 9: HTTP REST API (DONE)
- `internal/api/server.go`
- 14 endpoints covering all CRUD operations
- Go 1.22+ method routing (`GET /path`, `POST /path`)
- JSON request/response with proper error handling

### Milestone 10: CLI Entry Point (DONE)
- `cmd/dreddit/main.go`
- Flag parsing (node-id, ports, data-dir, bootstrap, config)
- Config file support
- Graceful shutdown via SIGINT/SIGTERM

---

## What's Ready for Tomorrow's Demo

**Everything above (Milestones 1-10) compiles and the binary runs.** You can demonstrate:

1. **Build and run**: `go build -o dreddit ./cmd/dreddit && ./dreddit --node-id node1 --http-port 8001`
2. **Create a community**: `curl -X POST localhost:8001/communities -d '{"name":"test","description":"A test community","creator_id":"alice"}'`
3. **Create a post**: `curl -X POST localhost:8001/communities/<id>/posts -d '{"author_id":"alice","title":"Hello","body":"First post!"}'`
4. **Vote on it**: `curl -X POST localhost:8001/vote -d '{"user_id":"bob","target_hash":"<hash>","value":1}'`
5. **Run tests**: `go test ./... -v` (10 passing CRDT tests)
6. **Show architecture**: Walk through the README diagram

---

## Future Work (Post-Demo)

| Priority | Task | Description |
|----------|------|-------------|
| High | gRPC transport | Replace in-memory transport with real gRPC |
| High | Multi-node testing | Integration tests with 3-5 node cluster |
| High | Raft integration | Wire Raft into community moderation flow |
| Medium | CRDT gossip sync | Actual CRDT state exchange over gossip |
| Medium | Persistence for Raft | Persist Raft log + state to disk |
| Medium | Node failure recovery | Detect and handle node failures |
| Low | Frontend | Web UI for browsing communities/posts |
| Low | Authentication | User identity and signatures |
