# DReddit: Decentralized, Fault-Tolerant Link Aggregation and Discussion System

A Go implementation of DReddit — a decentralized, Reddit-like platform where communities (subreddits) are replicated across small groups of nodes, using **CRDTs** for eventual consistency on discussion data and **Raft consensus** for strongly consistent moderation actions.

Based on the paper by Saiteja Poluka, Shanmukha Vamshi Kuruba, and Jayanth Vunnam (CU Boulder).

---

## Architecture Overview

```
┌───────────────────────────────────────────────────────────────────┐
│                        DReddit Node                               │
│                                                                   │
│  ┌──────────┐  ┌──────────────┐  ┌─────────────┐  ┌───────────┐ │
│  │ HTTP API │  │  Community   │  │   Network   │  │    DHT    │ │
│  │ (REST)   │  │   Manager    │  │  (Gossip)   │  │ (Lookup)  │ │
│  └────┬─────┘  └──────┬───────┘  └──────┬──────┘  └─────┬─────┘ │
│       │               │                 │                │       │
│       │       ┌───────┴────────┐        │                │       │
│       │       │  Per-Community │        │                │       │
│       │       │     State      │        │                │       │
│       │       │                │        │                │       │
│       │       │  ┌──────────┐  │        │                │       │
│       │       │  │  CRDTs   │◄─┼────────┘                │       │
│       │       │  │(PNCounter│  │  gossip sync            │       │
│       │       │  │ ORSet,   │  │                         │       │
│       │       │  │ GSet,    │  │                         │       │
│       │       │  │ LWW-Reg) │  │                         │       │
│       │       │  └──────────┘  │                         │       │
│       │       │  ┌──────────┐  │                         │       │
│       │       │  │   Raft   │  │  strong consistency     │       │
│       │       │  │(Mod Log) │  │  for moderation         │       │
│       │       │  └──────────┘  │                         │       │
│       │       │  ┌──────────┐  │                         │       │
│       │       │  │ Content  │  │                         │       │
│       │       │  │ Store    │  │                         │       │
│       │       │  │(SHA-256) │  │                         │       │
│       │       │  └──────────┘  │                         │       │
│       │       └────────────────┘                         │       │
│       └──────────────────────────────────────────────────┘       │
└───────────────────────────────────────────────────────────────────┘
```

### Key Design Decisions

| Concern | Approach | Consistency |
|---------|----------|-------------|
| Posts, Comments | Content-addressed storage (SHA-256) | Eventual (CRDTs) |
| Votes | PN-Counter CRDT | Eventual |
| User membership | OR-Set CRDT (add-wins) | Eventual |
| Moderation (bans, removals) | Raft consensus log | Strong |
| Community discovery | DHT (Kademlia-style XOR) | Eventual |
| State synchronization | Gossip protocol (anti-entropy) | Eventual |
| Replication | 3-5 node replica groups per community | Configurable |

---

## Project Structure

```
DReddit/
├── cmd/
│   └── dreddit/
│       └── main.go              # CLI entry point
├── internal/
│   ├── models/
│   │   └── models.go            # Core data types (Post, Comment, Vote, etc.)
│   ├── config/
│   │   └── config.go            # Node configuration
│   ├── crdt/
│   │   ├── crdt.go              # CRDT implementations (PNCounter, GSet, ORSet, LWW)
│   │   └── crdt_test.go         # CRDT unit tests (10 tests)
│   ├── storage/
│   │   └── content_store.go     # Content-addressed storage with persistence
│   ├── consensus/
│   │   └── raft.go              # Raft consensus for moderation log
│   ├── network/
│   │   └── network.go           # Peer management, gossip protocol, transports
│   ├── dht/
│   │   └── dht.go               # Distributed hash table for community lookup
│   ├── community/
│   │   └── community.go         # Community lifecycle and operations
│   ├── node/
│   │   └── node.go              # Main node orchestrator
│   └── api/
│       └── server.go            # HTTP REST API (all endpoints)
├── go.mod
├── LICENSE
└── README.md
```

---

## Running

```bash
# Build
go build -o dreddit ./cmd/dreddit

# Run a single node
./dreddit --node-id node1 --http-port 8001 --rpc-port 7001

# Run with a config file
./dreddit --config config.json

# Run with a bootstrap node
./dreddit --node-id node2 --http-port 8002 --bootstrap localhost:7001
```

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| GET | `/info` | Node information |
| GET | `/communities` | List communities |
| POST | `/communities` | Create community |
| GET | `/communities/{id}` | Get community details |
| POST | `/communities/{id}/posts` | Create post |
| GET | `/communities/{id}/posts` | List posts (with scores) |
| GET | `/posts/{hash}` | Get post with comments |
| POST | `/posts/{hash}/comments` | Add comment |
| GET | `/posts/{hash}/comments` | Get comments |
| POST | `/vote` | Vote on post/comment |
| POST | `/moderate` | Moderation action |
| GET | `/cluster/nodes` | List cluster nodes |
| POST | `/cluster/join` | Join cluster |

---

## Testing

```bash
# Run all tests
go test ./... -v

# Run CRDT tests only
go test ./internal/crdt/... -v
```

Currently 10 CRDT tests covering:
- PNCounter: increment, decrement, merge, idempotency
- GSet: add, contains, merge (union)
- ORSet: add, remove, concurrent add-wins semantics
- LWWRegister: last-writer-wins merge
- VoteState: apply votes, change votes

---

## CRDT Summary

| CRDT | Usage | Merge Strategy |
|------|-------|----------------|
| **PNCounter** | Vote scores | max(positive[node]), max(negative[node]) |
| **GSet** | Banned users, permanent sets | Union |
| **ORSet** | Community membership | Add-wins with unique tags |
| **LWWRegister** | Per-user vote tracking | Latest timestamp wins |

---

## Authors

- Saiteja Poluka
- Shanmukha Vamshi Kuruba
- Jayanth Vunnam
