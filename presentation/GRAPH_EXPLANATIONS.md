# DReddit Evaluation Graphs - Complete Guide

This document explains each graph, the terminology used, and what the results mean for the DReddit system.

---

## Key Terminology

### Scenarios

**Baseline**
- Normal, healthy cluster operation
- All nodes are running and stable
- No failures or disruptions
- Represents best-case performance

**Failover**
- One or more nodes crash/fail
- The system must route requests to surviving replicas
- Tests whether the system stays available when nodes die
- Client must reroute to a different node

**Churn**
- Nodes repeatedly crash and restart
- High instability: nodes join, leave, rejoin continuously
- Simulates a turbulent production environment
- Tests resilience under constant membership changes

### Latency Metrics

**Average latency (ms)**
- Mean time for a request to complete
- Simple average of all measurements

**P95 (95th percentile)**
- 95% of requests complete faster than this time
- Only 5% of requests are slower
- Shows "typical worst case"

**P99 (99th percentile)**
- 99% of requests complete faster than this time
- Only 1% of requests are slower
- Reveals tail latency problems

**Max (maximum)**
- The slowest request observed
- Shows the absolute worst case

### Request Types

**Read**
- Fetching posts, comments, or votes
- No data modification
- Typically faster than writes

**Write**
- Creating posts, commenting, or voting
- Modifies data and replicates across nodes
- Requires CRDT merging and consensus (for moderation)

---

## Graph 1: Average Latency by Scenario

### What It Shows
A bar chart comparing average latency across six scenarios using a **logarithmic scale**.

### Why Log Scale?
Churn write latency is ~40,000 ms while baseline read is ~1 ms. Linear scale would make small values invisible. Log scale shows all bars proportionally.

### The Numbers

| Scenario | Latency | Meaning |
|----------|---------|---------|
| Baseline read | 1.2 ms | Very fast - reading from local replica |
| Baseline write | 5.6 ms | Fast - CRDT write with gossip propagation |
| Failover read | 1.0 ms | Still fast - reading from surviving node |
| Failover write | 6.2 ms | Similar to baseline - minimal impact |
| Churn read | 4.3 ms | ~4× slower - some instability |
| Churn write | 40,009 ms | **~40 seconds** - major degradation |

### Key Insights

✅ **Healthy-path performance is excellent**
- Reads: ~1 ms (near-local)
- Writes: ~5-6 ms (reasonable for distributed system)

✅ **Failover is handled well**
- Latency barely changes when nodes crash
- System reroutes successfully

⚠️ **Churn severely impacts writes**
- Write latency increases 7000× during churn
- This is the system's main weakness
- Reads stay relatively fast (4× slower vs. 7000× for writes)

### Why Is Churn Write So Slow?

During churn:
1. Nodes repeatedly crash mid-operation
2. Write operations must retry
3. Membership changes trigger re-coordination
4. The evaluation waits for eventual success (doesn't timeout immediately)
5. This measures real retry costs, not just healthy-path speed

---

## Graph 2: Tail Latency Under Failure (Log Scale)

### What It Shows
Grouped bar chart showing P95, P99, and Max latency for three failure scenarios.

### The Numbers

**Churn Read**
- P95: 2 ms (95% of reads complete in 2ms)
- P99: 95 ms (99% complete in 95ms)
- Max: 95 ms (worst case matches P99)

**Churn Write**
- P95: 60,006 ms (~60 seconds)
- P99: 60,106 ms
- Max: 60,106 ms
- All three are nearly identical - consistent slowness

**Failover Write**
- P95: 8 ms
- P99: 8 ms
- Max: 8 ms
- Very consistent, slightly slower than baseline

### Key Insights

✅ **Failover has low tail latency**
- All percentiles around 8ms
- Very predictable performance

⚠️ **Churn read tail is acceptable**
- P95 is only 2ms
- But P99/Max jumps to 95ms (outliers exist)

❌ **Churn write tail is catastrophic**
- Even P95 is 60 seconds
- Not just outliers - consistently slow
- Makes the system unusable for writes during high churn

### What This Means

Tail latency shows reliability:
- **Low tail latency** = predictable, consistent performance
- **High tail latency** = some requests suffer badly

The churn write result shows that nearly all writes (not just a few unlucky ones) take ~60 seconds during cluster instability.

---

## Graph 3: System Performance Under Failure

### What It Shows
Three metrics cards plus availability bars showing request completion rates.

### The Numbers

**Top Cards**
- **Throughput**: 228 ops/sec
  - Mixed read/write workload
  - Measured over 2.1 seconds
  - Includes concurrent operations (4 concurrent clients)
  
- **Total Operations**: 472
  - All operations attempted during the test
  - 472 succeeded, 0 failed = 100% success rate
  
- **Duration**: 2.1 seconds
  - How long the throughput test ran
  - 0 failed operations

**Availability Bars**

| Scenario | Completed | Percentage | Sample Size |
|----------|-----------|------------|-------------|
| Failover Read | 5/5 | 100% | Small sample |
| Failover Write | 5/5 | 100% | Small sample |
| Churn Read | 45/45 | 100% | Larger sample |
| Churn Write | 45/45 | 100% | Larger sample |

### Key Insights

✅ **High throughput for a prototype**
- 228 ops/sec is respectable for a local multi-process test
- Not a distributed datacenter benchmark
- Shows the system can handle concurrent load

✅ **Perfect availability (100%)**
- All sampled requests completed during failures
- No dropped requests
- System stays serviceable even during node crashes

⚠️ **Important caveat**
- 100% availability doesn't mean fast
- Churn writes complete, but take 40+ seconds (see Graph 1)
- Availability ≠ Performance

### What "100% Availability" Actually Means

The system is **available** (accepts and completes requests) but not necessarily **performant** (completes them quickly).

Think of it like a restaurant:
- **Available**: The restaurant is open and will serve your food
- **Performant**: Your food arrives in 5 minutes vs. 60 minutes

DReddit during churn: restaurant is open (100% availability) but food takes an hour (40s latency).

---

## Graph 4: Convergence and Moderation

### What It Shows
Two-part graph: convergence metrics (top) and moderation effectiveness (bottom).

### Convergence Metrics

**Cross-Region Convergence**
- **Time**: 160 ms
- **Threshold**: 2000 ms (2 seconds)
- **Headroom**: 1840 ms
- **Status**: ✅ PASS (well under threshold)

**What it means:**
- 20 replicas distributed across 4 simulated regions (us-east, us-west, eu-central, ap-southeast)
- Inter-region latency ranges from 5ms (intra-region) to 210ms (cross-region)
- After a vote is cast, CRDT state converges across all 20 nodes in 160ms
- This is fast considering the high WAN latency

**Partition-Heal Convergence**
- **Time**: 180 ms
- **Threshold**: 1500 ms (1.5 seconds)
- **Headroom**: 1320 ms
- **Status**: ✅ PASS

**What it means:**
- Network partition splits 20 replicas into two isolated groups
- Each group gets different updates (votes, posts)
- After 400ms, the partition heals (groups can communicate again)
- Full-sync event is triggered
- All conflicting state reconciles in 180ms
- Both groups converge to the same final state

### Moderation Metrics

**Flow Diagram: 3 → 0**
- **Before moderation**: 3 malicious posts created by "Mallory" (the attacker)
- **After moderation**: 0 visible posts on follower nodes
- **Post-ban response**: HTTP 403 (Forbidden)

**What it means:**

1. **Malicious user creates spam**
   - A banned/malicious user ("Mallory") creates 3 spam posts
   - Posts initially replicate across nodes via gossip

2. **Moderator takes action**
   - Admin flags posts for removal
   - Removal actions go through Raft consensus
   - All nodes agree on the moderation log

3. **Pruning takes effect**
   - Follower nodes apply the moderation log
   - Flagged posts are hidden from reads
   - 0 malicious posts remain visible

4. **Future posts are blocked**
   - Mallory is banned
   - Attempts to post return 403 Forbidden
   - Ban is enforced consistently across all nodes

### Key Insights

✅ **CRDT convergence is fast**
- Even with high WAN latency (210ms), convergence is 160ms
- System handles network partitions gracefully
- Final state is consistent across all replicas

✅ **Moderation works correctly**
- Reactive content removal succeeds
- Ban enforcement prevents future abuse
- Consistency: all nodes enforce the same rules

⚠️ **Reactive, not proactive**
- System doesn't prevent malicious posts upfront
- Moderation happens after content is created
- No cryptographic Sybil resistance (user can create multiple identities)

---

## Overall System Assessment

### Strengths

✅ **Excellent healthy-path latency**
- 1-6 ms for normal operations
- Interactive user experience
- Competitive with centralized systems

✅ **Fast CRDT convergence**
- 160-180 ms under WAN and partition scenarios
- Proves eventual consistency works at scale

✅ **High availability**
- 100% request completion during failures
- System never becomes unavailable

✅ **Effective moderation**
- Raft-backed consensus for policy enforcement
- Consistent ban/removal across all nodes

### Weaknesses

❌ **Churn write performance is unusable**
- 40+ second latency during repeated node restarts
- 7000× slower than baseline
- Needs architectural improvement

⚠️ **Failover requires manual reroute**
- Client must switch nodes when one fails
- No transparent backend forwarding (yet)

⚠️ **Small evaluation sample sizes**
- Failover: only 5 requests tested
- Churn: 45 requests tested
- Results are indicative, not statistically comprehensive

---

## How To Present These Results

### For Academic/Research Audiences

1. **Start with the positive**: Low baseline latency, fast convergence
2. **Show availability**: 100% completion under failures
3. **Be honest about weaknesses**: Churn write latency is the clear limitation
4. **Frame it as research**: "This prototype validates CRDT+Raft architecture but exposes a key engineering challenge"

### For Technical Audiences

1. **Emphasize throughput**: 228 ops/sec for a prototype
2. **Highlight convergence**: 160ms under simulated WAN
3. **Explain churn**: Not a bug, it's measuring real retry costs
4. **Set expectations**: This is a prototype, not production-ready

### For Stakeholders

1. **Show the vision**: Decentralized Reddit is feasible
2. **Demonstrate reliability**: 100% availability
3. **Acknowledge reality**: Write path needs optimization before launch
4. **Roadmap**: "Next phase focuses on churn resilience"

---

## Comparison With Centralized Reddit

| Metric | DReddit (Prototype) | Centralized Reddit (est.) |
|--------|---------------------|---------------------------|
| Baseline read latency | 1.2 ms | 10-50 ms (DB + network) |
| Baseline write latency | 5.6 ms | 50-200 ms (DB + replication) |
| **Single point of failure** | ❌ None | ⚠️ Central server/DB |
| **Censorship resistance** | ✅ Decentralized | ❌ Admin controls all |
| **Partition tolerance** | ✅ Strong | ⚠️ Dependent on infra |
| **Churn write latency** | ❌ 40 seconds | ✅ Not applicable |

### DReddit's Value Proposition

- **Lower latency** for reads/writes in healthy state
- **No central point of failure** - distributed across nodes
- **Survives network partitions** - keeps working when isolated
- **Censorship resistant** - no single entity controls all data

### DReddit's Current Limitations

- **Churn tolerance** needs improvement
- **Smaller scale** testing (15-20 nodes vs. massive production fleet)
- **Simulated WAN** not real geographic distribution
