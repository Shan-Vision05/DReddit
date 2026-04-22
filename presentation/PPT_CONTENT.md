# DReddit Presentation Content

This file provides slide-ready content for a project presentation grounded in the evaluation data from `EVALUATION_RESULTS.md` and the system description in `README.md`.

## Slide 1 - Title

**Title:** DReddit: A Distributed Reddit Prototype With CRDT Replication And Raft Moderation

**Subtitle:** Evaluation-driven presentation of architecture, resilience, and measured tradeoffs

**Presenter notes:**

- Position DReddit as a fully decentralized Reddit-style platform implemented in Go.
- Frame the presentation around one design idea: eventual consistency for content, strong consistency for moderation.
- State that the results are from both a live multi-process cluster and deterministic WAN/partition evaluation.

## Slide 2 - Problem And Goal

**Title:** Why Build A Distributed Reddit?

**Slide content:**

- Traditional social platforms centralize control, moderation, and failure domains.
- A decentralized forum needs low-latency reads and writes, but also reliable moderation and recovery.
- DReddit explores whether we can combine conflict-free replication with explicit consensus only where it is necessary.

**Talk track:**

We are not trying to make every operation strongly consistent. The goal is to use the lightest coordination model that still preserves the user-facing behavior we care about.

## Slide 3 - System Design

**Title:** Two Consistency Models In One Node

**Slide content:**

- Posts, comments, and votes use CRDT-based eventual consistency.
- Ban, remove, restore, and unban actions use Raft-backed ordered consensus.
- Communities are isolated so moderation decisions stay scoped to a single subreddit-like space.
- A DHT maps communities to nodes, while gossip propagates replicated state.

**Talk track:**

The architecture splits high-volume collaborative state from low-volume but policy-sensitive state. That keeps the write path simple for content while preserving strong agreement for moderation.

## Slide 4 - Evaluation Methodology

**Title:** How The Prototype Was Evaluated

**Slide content:**

- Live 15-node cluster evaluation for latency, throughput, failover, churn, and moderation behavior.
- Deterministic 20-replica WAN simulation for cross-region delay and partition-heal convergence.
- Metrics collected: average latency, tail latency, availability, throughput, convergence time, and moderation outcomes.

**Talk track:**

This evaluation intentionally separates what was measured on real local processes from what was simulated in a controlled WAN model. That distinction matters for credibility.

## Slide 5 - Healthy-Path Performance

**Title:** Healthy Cluster Latency Is Low

**Slide content:**

- Baseline read latency averaged **1.2 ms**.
- Baseline write latency averaged **5.6 ms**.
- Failover reads still averaged **1.0 ms** and failover writes averaged **6.2 ms**.
- These numbers reflect a local prototype where loopback networking dominates cost.

**Visual:** Use `presentation/graphs/latency_summary.svg`

**Talk track:**

The healthy path looks strong. Reads are essentially near-local and even failover stays close to baseline when clients reroute to a surviving replica.

## Slide 6 - Failure Tradeoff

**Title:** Availability Holds, But Churn Write Latency Explodes

**Slide content:**

- Churn availability stayed at **100%** for both reads and writes in the sampled run.
- Churn read latency averaged **4.3 ms**, but churn write latency averaged **40,009.2 ms**.
- Tail latency reached **60,106 ms** at max for churn writes.
- This is the clearest weakness in the current prototype.

**Visual:** Use `presentation/graphs/tail_latency.svg`

**Talk track:**

The system remains serviceable, but the cost of repeated membership changes shows up directly in the write tail. This is the key negative result and it should be presented honestly.

## Slide 7 - Throughput And Service Continuity

**Title:** The System Remains Responsive During Faults

**Slide content:**

- Mixed workload throughput reached **228.33 ops/s**.
- Across sampled failover and churn scenarios, request completion remained **100%**.
- The current failover result reflects client-side reroute, not backend-transparent forwarding.

**Visual:** Use `presentation/graphs/throughput_availability.svg`

**Talk track:**

This is an availability story more than a raw throughput story. The important takeaway is that the service remains usable while nodes fail and recover, but only because clients can move to a healthy replica.

## Slide 8 - WAN And Partition Results

**Title:** Convergence Stays Fast Across Regions And After Healing

**Slide content:**

- Cross-region convergence completed in **160 ms** with a **2000 ms** threshold.
- Partition-heal convergence completed in **180 ms** with a **1500 ms** threshold.
- Both results passed comfortably in the 20-replica deterministic evaluation.
- These results support the CRDT design for asynchronous merge-heavy workloads.

**Visual:** Use `presentation/graphs/convergence_security.svg`

**Talk track:**

Even under high-latency simulated links, the replicated vote state converges quickly. After a partition heals, the system also recovers quickly once a full-sync event is triggered.

## Slide 9 - Security And Moderation

**Title:** Reactive Moderation Works, Proactive Sybil Defense Is Out Of Scope

**Slide content:**

- A malicious user created **3** spam posts.
- After pruning replicated to a follower, visible malicious posts dropped to **0**.
- The banned user was blocked from posting again with HTTP **403**.
- The prototype demonstrates reactive moderation, not cryptographic identity enforcement.

**Visual:** Reuse `presentation/graphs/convergence_security.svg` and crop the moderation panel if desired.

**Talk track:**

This is a moderation effectiveness claim, not a full security claim. The system can remove abusive content and enforce bans, but it does not solve identity creation or Sybil resistance.

## Slide 10 - What Worked Well

**Title:** Main Strengths

**Slide content:**

- Low healthy-path latency for interactive use.
- Full sampled availability during failover and restart churn.
- Fast CRDT convergence in WAN and post-partition scenarios.
- Moderation actions replicate correctly and preserve policy consistency.

**Talk track:**

The architecture does what it was intended to do: keep collaborative data highly available while restricting coordination to moderation.

## Slide 11 - Limitations And Honest Caveats

**Title:** What The Evaluation Does Not Claim

**Slide content:**

- Failover uses client reroute rather than transparent backend forwarding.
- WAN behavior is simulated, not measured over geographically distributed infrastructure.
- Partition healing requires an explicit full-sync event in the evaluation.
- Write latency under churn is currently too high for a production-quality user experience.

**Talk track:**

These caveats do not invalidate the project, but they define the boundary of the claims we can responsibly make.

## Slide 12 - Closing

**Title:** Conclusion

**Slide content:**

- DReddit shows that a Reddit-like system can mix CRDT replication and Raft moderation in one practical prototype.
- The strongest evidence is low healthy-path latency, high sampled availability, and fast convergence.
- The primary next engineering target is the churn write path.

**Talk track:**

The project is successful as a systems prototype because it validates the core architecture and also exposes the exact area that needs deeper engineering next.

## Optional Slide 13 - Future Work

**Title:** Next Steps

**Slide content:**

- Add internal request forwarding so failover is transparent to clients.
- Add periodic anti-entropy after partition heal.
- Reduce churn write tail latency with better retry and membership-handling logic.
- Expand evaluation to real multi-host or cloud-based WAN deployments.