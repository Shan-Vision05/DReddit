#!/usr/bin/env bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BINARY="${REPO_ROOT}/dreddit"
TEST_DIR="/tmp/dreddit_cluster_full_$$"
PASS=0
FAIL=0
PIDS=()

N1_HTTP=8091; N1_GOSSIP=11101; N1_DIR="${TEST_DIR}/node1"
N2_HTTP=8092; N2_GOSSIP=11102; N2_DIR="${TEST_DIR}/node2"
N3_HTTP=8093; N3_GOSSIP=11103; N3_DIR="${TEST_DIR}/node3"
N4_HTTP=8094; N4_GOSSIP=11104; N4_DIR="${TEST_DIR}/node4"
N5_HTTP=8095; N5_GOSSIP=11105; N5_DIR="${TEST_DIR}/node5"

pass() { echo -e "  \033[32m✓ PASS\033[0m  $1"; PASS=$((PASS + 1)); }
fail() { echo -e "  \033[31m✗ FAIL\033[0m  $1"; FAIL=$((FAIL + 1)); }

cleanup() {
    echo
    echo "── Stopping nodes ──────────────────────────────────────────────────────────"
    for pid in "${PIDS[@]:-}"; do
        kill "$pid" 2>/dev/null || true
    done
    wait 2>/dev/null || true
    rm -rf "$TEST_DIR"
    echo "── Cleanup done ────────────────────────────────────────────────────────────"
}
trap cleanup EXIT

wait_for() {
    local url=$1 max=${2:-30} count=0
    until curl -sf "$url" >/dev/null 2>&1; do
        sleep 0.5
        count=$((count + 1))
        if [ "$count" -gt $((max * 2)) ]; then
            echo "Timed out waiting for $url" >&2
            return 1
        fi
    done
}

cj() {
    local method=$1 url=$2 body="${3:-}"
    shift 3 || true
    if [[ -n "$body" ]]; then
        curl -sf -X "$method" -H "Content-Type: application/json" "$@" -d "$body" "$url"
    else
        curl -sf -X "$method" -H "Content-Type: application/json" "$@" "$url"
    fi
}

json_get() {
    python3 -c 'import json, sys; key = sys.argv[1]; data = json.load(sys.stdin); val = data.get(key, ""); print(json.dumps(val) if isinstance(val, (list, dict)) else val)' "$1"
}

json_len() {
    python3 -c 'import json, sys; data = json.load(sys.stdin); data = [] if data is None else data; print(len(data if isinstance(data, list) else []))'
}

json_find_score() {
    python3 -c 'import json, sys; target = sys.argv[1]; items = json.load(sys.stdin); items = [] if items is None else items; items = items if isinstance(items, list) else []; print(next((item.get("score", 0) for item in items if item.get("hash") == target), 0))' "$1"
}

json_count_author() {
    python3 -c 'import json, sys; target = sys.argv[1]; items = json.load(sys.stdin); items = [] if items is None else items; items = items if isinstance(items, list) else []; print(sum(1 for item in items if item.get("author_id") == target))' "$1"
}

json_has_hash() {
    python3 -c 'import json, sys; target = sys.argv[1]; items = json.load(sys.stdin); items = [] if items is None else items; items = items if isinstance(items, list) else []; print("true" if any(item.get("hash") == target for item in items) else "false")' "$1"
}

json_has_substring() {
    local body=$1 needle=$2
    if echo "$body" | grep -q "$needle"; then
        return 0
    fi
    return 1
}

http_code() {
    local method=$1 url=$2 body="${3:-}"
    shift 3 || true
    if [[ -n "$body" ]]; then
        curl -s -o /dev/null -w "%{http_code}" -X "$method" -H "Content-Type: application/json" "$@" -d "$body" "$url"
    else
        curl -s -o /dev/null -w "%{http_code}" -X "$method" -H "Content-Type: application/json" "$@" "$url"
    fi
}

wait_for_hash_in_posts() {
    local port=$1 token=$2 community=$3 hash=$4 max=${5:-30} count=0
    until (curl -sf "http://127.0.0.1:${port}/api/posts?community_id=${community}" -H "Authorization: Bearer ${token}" 2>/dev/null || echo '[]') | json_has_hash "$hash" | grep -q true; do
        sleep 0.5
        count=$((count + 1))
        if [ "$count" -gt $((max * 2)) ]; then
            return 1
        fi
    done
}

wait_for_hash_in_comments() {
    local port=$1 token=$2 community=$3 post_hash=$4 hash=$5 max=${6:-30} count=0
    until (curl -sf "http://127.0.0.1:${port}/api/comments?community_id=${community}&post_hash=${post_hash}" -H "Authorization: Bearer ${token}" 2>/dev/null || echo '[]') | json_has_hash "$hash" | grep -q true; do
        sleep 0.5
        count=$((count + 1))
        if [ "$count" -gt $((max * 2)) ]; then
            return 1
        fi
    done
}

start_node() {
    local node_id=$1 http_port=$2 gossip_port=$3 data_dir=$4 peers=$5 log_file=$6
    if [[ -n "$peers" ]]; then
        "$BINARY" -id "$node_id" -addr ":${http_port}" -gossip-port "$gossip_port" -peers "$peers" -data-dir "$data_dir" > "$log_file" 2>&1 &
    else
        "$BINARY" -id "$node_id" -addr ":${http_port}" -gossip-port "$gossip_port" -data-dir "$data_dir" > "$log_file" 2>&1 &
    fi
    PIDS+=("$!")
    wait_for "http://127.0.0.1:${http_port}/api/status"
}

mkdir -p "$N1_DIR" "$N2_DIR" "$N3_DIR" "$N4_DIR" "$N5_DIR"

if [[ ! -x "$BINARY" ]]; then
    (cd "$REPO_ROOT" && go build -o dreddit ./cmd/dreddit/)
fi

echo "════════════════════════════════════════════════════════════════════════"
echo " DReddit Full 5-node Distributed Test"
echo "════════════════════════════════════════════════════════════════════════"

echo
echo "── [1] Starting 5 separate processes ───────────────────────────────────"
start_node node1 "$N1_HTTP" "$N1_GOSSIP" "$N1_DIR" "" "${TEST_DIR}/node1.log"
pass "node1 up (PID=${PIDS[0]})"
start_node node2 "$N2_HTTP" "$N2_GOSSIP" "$N2_DIR" "127.0.0.1:${N1_GOSSIP}" "${TEST_DIR}/node2.log"
pass "node2 up (PID=${PIDS[1]})"
start_node node3 "$N3_HTTP" "$N3_GOSSIP" "$N3_DIR" "127.0.0.1:${N1_GOSSIP},127.0.0.1:${N2_GOSSIP}" "${TEST_DIR}/node3.log"
pass "node3 up (PID=${PIDS[2]})"
start_node node4 "$N4_HTTP" "$N4_GOSSIP" "$N4_DIR" "127.0.0.1:${N1_GOSSIP},127.0.0.1:${N2_GOSSIP},127.0.0.1:${N3_GOSSIP}" "${TEST_DIR}/node4.log"
pass "node4 up (PID=${PIDS[3]})"
start_node node5 "$N5_HTTP" "$N5_GOSSIP" "$N5_DIR" "127.0.0.1:${N1_GOSSIP},127.0.0.1:${N2_GOSSIP},127.0.0.1:${N3_GOSSIP},127.0.0.1:${N4_GOSSIP}" "${TEST_DIR}/node5.log"
pass "node5 up (PID=${PIDS[4]})"

sleep 3
for port in "$N1_HTTP" "$N2_HTTP" "$N3_HTTP" "$N4_HTTP" "$N5_HTTP"; do
    MEMBERS=$(curl -sf "http://127.0.0.1:${port}/api/status" | json_get member_count)
    if [[ "$MEMBERS" =~ ^[0-9]+$ ]] && [ "$MEMBERS" -ge 5 ]; then
        pass "node on :${port} sees $MEMBERS gossip members"
    else
        fail "node on :${port} sees $MEMBERS gossip members (want 5)"
    fi
done

echo
echo "── [2] Auth and user setup across replicas ─────────────────────────────"
TOK_ADMIN=$(cj POST "http://127.0.0.1:${N1_HTTP}/api/signup" '{"username":"admin","password":"adminpw"}' | json_get token)
TOK_ALICE=$(cj POST "http://127.0.0.1:${N1_HTTP}/api/signup" '{"username":"alice","password":"pw1"}' | json_get token)
TOK_BOB=$(cj POST "http://127.0.0.1:${N2_HTTP}/api/signup" '{"username":"bob","password":"pw2"}' | json_get token)
TOK_CAROL=$(cj POST "http://127.0.0.1:${N3_HTTP}/api/signup" '{"username":"carol","password":"pw3"}' | json_get token)
TOK_DAVE=$(cj POST "http://127.0.0.1:${N4_HTTP}/api/signup" '{"username":"dave","password":"pw4"}' | json_get token)
TOK_MALLORY=$(cj POST "http://127.0.0.1:${N5_HTTP}/api/signup" '{"username":"mallory","password":"pw5"}' | json_get token)
TOK_ERIN=$(cj POST "http://127.0.0.1:${N5_HTTP}/api/signup" '{"username":"erin","password":"pw6"}' | json_get token)
for pair in "$TOK_ADMIN admin" "$TOK_ALICE alice" "$TOK_BOB bob" "$TOK_CAROL carol" "$TOK_DAVE dave" "$TOK_MALLORY mallory" "$TOK_ERIN erin"; do
    set -- $pair
    if [ ${#1} -ge 20 ]; then
        pass "$2 signup returned token"
    else
        fail "$2 signup failed"
    fi
done
SC=$(http_code POST "http://127.0.0.1:${N3_HTTP}/api/join" '{"community_id":"golang"}')
if [ "$SC" = "401" ]; then pass "protected endpoint rejects missing token"; else fail "missing token join returned $SC"; fi

echo
echo "── [3] 3 communities, 5-node community membership, Raft groups ─────────"
for community in golang distributed webdev; do
    SC=$(http_code POST "http://127.0.0.1:${N1_HTTP}/api/join" "{\"community_id\":\"$community\"}" -H "Authorization: Bearer $TOK_ADMIN")
    if [ "$SC" = "200" ]; then pass "node1 bootstrapped $community"; else fail "node1 bootstrap $community -> $SC"; fi
done
sleep 3

for join in \
    "$N2_HTTP $TOK_BOB golang" \
    "$N3_HTTP $TOK_CAROL golang" \
    "$N4_HTTP $TOK_DAVE golang" \
    "$N5_HTTP $TOK_ERIN golang" \
    "$N2_HTTP $TOK_BOB distributed" \
    "$N3_HTTP $TOK_CAROL distributed" \
    "$N4_HTTP $TOK_DAVE distributed" \
    "$N5_HTTP $TOK_ERIN distributed" \
    "$N2_HTTP $TOK_BOB webdev" \
    "$N3_HTTP $TOK_CAROL webdev" \
    "$N4_HTTP $TOK_DAVE webdev" \
    "$N5_HTTP $TOK_ERIN webdev"; do
    set -- $join
    SC=$(http_code POST "http://127.0.0.1:$1/api/join" "{\"community_id\":\"$3\"}" -H "Authorization: Bearer $2")
    if [ "$SC" = "200" ] || [ "$SC" = "201" ]; then
        pass "node on :$1 joined $3"
    else
        fail "node on :$1 join $3 -> $SC"
    fi
done
sleep 4

for port in "$N1_HTTP" "$N2_HTTP" "$N3_HTTP" "$N4_HTTP" "$N5_HTTP"; do
    COMMS=$(curl -sf "http://127.0.0.1:${port}/api/status" | json_get communities)
    if json_has_substring "$COMMS" golang && json_has_substring "$COMMS" distributed && json_has_substring "$COMMS" webdev; then
        pass "node on :${port} lists all 3 communities"
    else
        fail "node on :${port} communities missing: $COMMS"
    fi
done

SC=$(http_code POST "http://127.0.0.1:${N4_HTTP}/api/join" '{"community_id":"golang"}' -H "Authorization: Bearer $TOK_DAVE")
if [ "$SC" = "200" ]; then pass "duplicate join remains idempotent"; else fail "duplicate join -> $SC"; fi

echo
echo "── [4] Posts, comments, votes, cross-node replication ──────────────────"
POST_G1=$(cj POST "http://127.0.0.1:${N1_HTTP}/api/post" '{"community_id":"golang","title":"Go Consensus","body":"Raft details"}' -H "Authorization: Bearer $TOK_ALICE")
HASH_G1=$(echo "$POST_G1" | json_get hash)
POST_G2=$(cj POST "http://127.0.0.1:${N2_HTTP}/api/post" '{"community_id":"golang","title":"SWIM Notes","body":"Gossip details"}' -H "Authorization: Bearer $TOK_BOB")
HASH_G2=$(echo "$POST_G2" | json_get hash)
POST_D1=$(cj POST "http://127.0.0.1:${N3_HTTP}/api/post" '{"community_id":"distributed","title":"CAP Tradeoffs","body":"Partition handling"}' -H "Authorization: Bearer $TOK_CAROL")
HASH_D1=$(echo "$POST_D1" | json_get hash)
POST_W1=$(cj POST "http://127.0.0.1:${N4_HTTP}/api/post" '{"community_id":"webdev","title":"HTTP Caching","body":"ETag and cache-control"}' -H "Authorization: Bearer $TOK_DAVE")
HASH_W1=$(echo "$POST_W1" | json_get hash)
for hash_desc in "$HASH_G1 golang post 1" "$HASH_G2 golang post 2" "$HASH_D1 distributed post" "$HASH_W1 webdev post"; do
    set -- $hash_desc
    if [ -n "$1" ]; then pass "$2 $3 created"; else fail "$2 $3 creation failed"; fi
done

COMMENT1=$(cj POST "http://127.0.0.1:${N3_HTTP}/api/comment" "{\"community_id\":\"golang\",\"comment\":{\"post_hash\":\"$HASH_G1\",\"body\":\"useful post\"}}" -H "Authorization: Bearer $TOK_CAROL")
HASH_C1=$(echo "$COMMENT1" | json_get hash)
COMMENT2=$(cj POST "http://127.0.0.1:${N4_HTTP}/api/comment" "{\"community_id\":\"golang\",\"comment\":{\"post_hash\":\"$HASH_G1\",\"parent_hash\":\"$HASH_C1\",\"body\":\"nested reply\"}}" -H "Authorization: Bearer $TOK_DAVE")
HASH_C2=$(echo "$COMMENT2" | json_get hash)
if [ -n "$HASH_C1" ]; then pass "top-level comment created"; else fail "top-level comment creation failed"; fi
if [ -n "$HASH_C2" ]; then pass "nested comment created"; else fail "nested comment creation failed"; fi

for wait_spec in \
    "$N2_HTTP $TOK_BOB golang $HASH_G1" \
    "$N3_HTTP $TOK_CAROL golang $HASH_G1" \
    "$N4_HTTP $TOK_DAVE golang $HASH_G1" \
    "$N5_HTTP $TOK_ERIN golang $HASH_G1" \
    "$N1_HTTP $TOK_ALICE golang $HASH_G2" \
    "$N5_HTTP $TOK_ERIN golang $HASH_G2"; do
    set -- $wait_spec
    if wait_for_hash_in_posts "$1" "$2" "$3" "$4" 20; then
        pass "replicated post ${4:0:8} reached node on :$1"
    else
        fail "replicated post ${4:0:8} did not reach node on :$1"
    fi
done

for wait_spec in \
    "$N2_HTTP $TOK_BOB golang $HASH_G1 $HASH_C1" \
    "$N5_HTTP $TOK_ERIN golang $HASH_G1 $HASH_C1" \
    "$N5_HTTP $TOK_ERIN golang $HASH_G1 $HASH_C2"; do
    set -- $wait_spec
    if wait_for_hash_in_comments "$1" "$2" "$3" "$4" "$5" 20; then
        pass "replicated comment ${5:0:8} reached node on :$1"
    else
        fail "replicated comment ${5:0:8} did not reach node on :$1"
    fi
done

for vote in \
    "$N1_HTTP $TOK_ALICE $HASH_G1 1" \
    "$N2_HTTP $TOK_BOB $HASH_G1 1" \
    "$N3_HTTP $TOK_CAROL $HASH_G1 1" \
    "$N4_HTTP $TOK_DAVE $HASH_G1 -1" \
    "$N5_HTTP $TOK_ERIN $HASH_G1 1" \
    "$N5_HTTP $TOK_ERIN $HASH_C1 1"; do
    set -- $vote
    SC=$(http_code POST "http://127.0.0.1:$1/api/vote" "{\"community_id\":\"golang\",\"vote\":{\"target_hash\":\"$3\",\"value\":$4}}" -H "Authorization: Bearer $2")
    if [ "$SC" = "200" ]; then pass "vote applied on :$1 for ${3:0:8}"; else fail "vote on :$1 for $3 -> $SC"; fi
done

sleep 4
for port in "$N1_HTTP" "$N2_HTTP" "$N3_HTTP" "$N4_HTTP" "$N5_HTTP"; do
    POSTS=$(cj GET "http://127.0.0.1:${port}/api/posts?community_id=golang" "" -H "Authorization: Bearer $TOK_ALICE")
    COUNT=$(echo "$POSTS" | json_len 2>/dev/null || echo 0)
    SCORE=$(echo "$POSTS" | json_find_score "$HASH_G1" 2>/dev/null || echo 0)
    if [ "$COUNT" -ge 2 ]; then pass "node on :$port sees replicated golang posts"; else fail "node on :$port sees only $COUNT golang posts"; fi
    if [ "$SCORE" = "3" ]; then pass "node on :$port sees converged score 3 for golang post"; else fail "node on :$port sees score $SCORE for golang post (want 3)"; fi
done

COMMENTS=$(cj GET "http://127.0.0.1:${N5_HTTP}/api/comments?community_id=golang&post_hash=${HASH_G1}" "" -H "Authorization: Bearer $TOK_ERIN")
CCOUNT=$(echo "$COMMENTS" | json_len 2>/dev/null || echo 0)
CSCORE=$(echo "$COMMENTS" | json_find_score "$HASH_C1" 2>/dev/null || echo 0)
if [ "$CCOUNT" -ge 2 ]; then pass "node5 sees replicated comments"; else fail "node5 sees only $CCOUNT comments"; fi
if [ "$CSCORE" = "1" ]; then pass "comment vote replicated"; else fail "comment vote score is $CSCORE (want 1)"; fi

DIST_POSTS=$(cj GET "http://127.0.0.1:${N2_HTTP}/api/posts?community_id=distributed" "" -H "Authorization: Bearer $TOK_BOB")
WEB_POSTS=$(cj GET "http://127.0.0.1:${N2_HTTP}/api/posts?community_id=webdev" "" -H "Authorization: Bearer $TOK_BOB")
if [ "$(echo "$DIST_POSTS" | json_len 2>/dev/null || echo 0)" = "1" ]; then pass "distributed feed isolated"; else fail "distributed feed isolation broken"; fi
if [ "$(echo "$WEB_POSTS" | json_len 2>/dev/null || echo 0)" = "1" ]; then pass "webdev feed isolated"; else fail "webdev feed isolation broken"; fi

echo
echo "── [5] Moderation auth, delete/restore, ban/unban, raft isolation ─────"
SC=$(http_code POST "http://127.0.0.1:${N2_HTTP}/api/moderate" "{\"community_id\":\"golang\",\"action_type\":\"BAN_USER\",\"target\":\"alice\"}" -H "Authorization: Bearer $TOK_BOB")
if [ "$SC" = "403" ]; then pass "non-admin cannot ban users"; else fail "non-admin BAN_USER -> $SC (want 403)"; fi

TOK_FRANK=$(cj POST "http://127.0.0.1:${N1_HTTP}/api/signup" '{"username":"frank","password":"pw7"}' | json_get token)
SC=$(http_code POST "http://127.0.0.1:${N1_HTTP}/api/join" '{"community_id":"golang"}' -H "Authorization: Bearer $TOK_FRANK")
if [ "$SC" = "200" ]; then pass "leader-local non-admin user joined golang"; else fail "leader-local join for frank -> $SC"; fi
OWN_POST=$(cj POST "http://127.0.0.1:${N1_HTTP}/api/post" '{"community_id":"golang","title":"Temp Own Post","body":"delete me"}' -H "Authorization: Bearer $TOK_FRANK")
OWN_HASH=$(echo "$OWN_POST" | json_get hash)
SC=$(http_code POST "http://127.0.0.1:${N1_HTTP}/api/moderate" "{\"community_id\":\"golang\",\"action_type\":\"DELETE_POST\",\"target\":\"$OWN_HASH\"}" -H "Authorization: Bearer $TOK_FRANK")
if [ "$SC" = "200" ]; then pass "non-admin can delete own post"; else fail "own DELETE_POST -> $SC"; fi

SC=$(http_code POST "http://127.0.0.1:${N1_HTTP}/api/moderate" "{\"community_id\":\"golang\",\"action_type\":\"DELETE_POST\",\"target\":\"$HASH_G2\"}" -H "Authorization: Bearer $TOK_ADMIN")
if [ "$SC" = "200" ]; then pass "admin deleted bob post"; else fail "admin DELETE_POST -> $SC"; fi
sleep 2
POSTS=$(cj GET "http://127.0.0.1:${N5_HTTP}/api/posts?community_id=golang" "" -H "Authorization: Bearer $TOK_ERIN")
VISIBLE=$(echo "$POSTS" | json_has_hash "$HASH_G2")
if [ "$VISIBLE" = "false" ]; then pass "deleted post hidden on follower node"; else fail "deleted post still visible on follower"; fi

SC=$(http_code POST "http://127.0.0.1:${N1_HTTP}/api/moderate" "{\"community_id\":\"golang\",\"action_type\":\"RESTORE_POST\",\"target\":\"$HASH_G2\"}" -H "Authorization: Bearer $TOK_ADMIN")
if [ "$SC" = "200" ]; then pass "admin restored bob post"; else fail "RESTORE_POST -> $SC"; fi
sleep 2
POSTS=$(cj GET "http://127.0.0.1:${N3_HTTP}/api/posts?community_id=golang" "" -H "Authorization: Bearer $TOK_CAROL")
VISIBLE=$(echo "$POSTS" | json_has_hash "$HASH_G2")
if [ "$VISIBLE" = "true" ]; then pass "restored post visible again across node"; else fail "restored post still hidden"; fi

SC=$(http_code POST "http://127.0.0.1:${N1_HTTP}/api/moderate" "{\"community_id\":\"golang\",\"action_type\":\"DELETE_COMMENT\",\"target\":\"$HASH_C1\"}" -H "Authorization: Bearer $TOK_ADMIN")
if [ "$SC" = "200" ]; then pass "DELETE_COMMENT action accepted by Raft API"; else fail "DELETE_COMMENT -> $SC"; fi
sleep 2
COMMENTS_AFTER_DEL=$(cj GET "http://127.0.0.1:${N2_HTTP}/api/comments?community_id=golang&post_hash=${HASH_G1}" "" -H "Authorization: Bearer $TOK_BOB")
VISIBLE_COMMENT=$(echo "$COMMENTS_AFTER_DEL" | json_has_hash "$HASH_C1")
if [ "$VISIBLE_COMMENT" = "false" ]; then
    pass "deleted comment hidden from API"
else
    fail "deleted comment still visible from API (moderation log exists, but comment filtering is missing)"
fi

SC=$(http_code POST "http://127.0.0.1:${N1_HTTP}/api/moderate" "{\"community_id\":\"golang\",\"action_type\":\"BAN_USER\",\"target\":\"mallory\"}" -H "Authorization: Bearer $TOK_ADMIN")
if [ "$SC" = "200" ]; then pass "admin banned mallory in golang"; else fail "BAN_USER -> $SC"; fi
sleep 2
SC=$(http_code POST "http://127.0.0.1:${N5_HTTP}/api/join" '{"community_id":"golang"}' -H "Authorization: Bearer $TOK_MALLORY")
if [ "$SC" = "403" ]; then pass "banned user cannot join golang again"; else fail "banned user join golang -> $SC (want 403)"; fi
SC=$(http_code POST "http://127.0.0.1:${N5_HTTP}/api/post" '{"community_id":"golang","title":"Should Fail","body":"blocked"}' -H "Authorization: Bearer $TOK_MALLORY")
if [ "$SC" = "403" ]; then pass "banned user cannot post in golang"; else fail "banned user post golang -> $SC (want 403)"; fi

SC=$(http_code POST "http://127.0.0.1:${N5_HTTP}/api/join" '{"community_id":"distributed"}' -H "Authorization: Bearer $TOK_MALLORY")
if [ "$SC" = "200" ]; then pass "ban is scoped to golang, not distributed"; else fail "mallory join distributed -> $SC"; fi
SC=$(http_code POST "http://127.0.0.1:${N5_HTTP}/api/post" '{"community_id":"distributed","title":"Allowed Elsewhere","body":"ban should be scoped"}' -H "Authorization: Bearer $TOK_MALLORY")
if [ "$SC" = "201" ]; then pass "banned user can still post in distributed"; else fail "mallory post distributed -> $SC"; fi

SC=$(http_code POST "http://127.0.0.1:${N1_HTTP}/api/moderate" "{\"community_id\":\"golang\",\"action_type\":\"UNBAN_USER\",\"target\":\"mallory\"}" -H "Authorization: Bearer $TOK_ADMIN")
if [ "$SC" = "200" ]; then pass "admin unbanned mallory in golang"; else fail "UNBAN_USER -> $SC"; fi
sleep 2
SC=$(http_code POST "http://127.0.0.1:${N5_HTTP}/api/post" '{"community_id":"golang","title":"I am back","body":"unban works"}' -H "Authorization: Bearer $TOK_MALLORY")
if [ "$SC" = "201" ]; then pass "unbanned user can post again in golang"; else fail "unbanned user post golang -> $SC"; fi

echo
echo "── [6] Persistence with multi-node restart ───────────────────────────────"
BEFORE1=$(cj GET "http://127.0.0.1:${N1_HTTP}/api/posts?community_id=golang" "" -H "Authorization: Bearer $TOK_ALICE" | json_len)
BEFORE3=$(cj GET "http://127.0.0.1:${N3_HTTP}/api/posts?community_id=golang" "" -H "Authorization: Bearer $TOK_CAROL" | json_len)
kill "${PIDS[0]}" 2>/dev/null || true
wait "${PIDS[0]}" 2>/dev/null || true
kill "${PIDS[2]}" 2>/dev/null || true
wait "${PIDS[2]}" 2>/dev/null || true
pass "node1 and node3 stopped"

"$BINARY" -id node1 -addr ":${N1_HTTP}" -gossip-port "$N1_GOSSIP" -peers "127.0.0.1:${N2_GOSSIP},127.0.0.1:${N4_GOSSIP}" -data-dir "$N1_DIR" > "${TEST_DIR}/node1-restart.log" 2>&1 &
PIDS[0]="$!"
"$BINARY" -id node3 -addr ":${N3_HTTP}" -gossip-port "$N3_GOSSIP" -peers "127.0.0.1:${N2_GOSSIP},127.0.0.1:${N4_GOSSIP}" -data-dir "$N3_DIR" > "${TEST_DIR}/node3-restart.log" 2>&1 &
PIDS[2]="$!"
wait_for "http://127.0.0.1:${N1_HTTP}/api/status" 40
wait_for "http://127.0.0.1:${N3_HTTP}/api/status" 40
sleep 4
pass "node1 and node3 restarted with persisted data"

AFTER1=$(cj GET "http://127.0.0.1:${N1_HTTP}/api/posts?community_id=golang" "" -H "Authorization: Bearer $TOK_ALICE" | json_len)
AFTER3=$(cj GET "http://127.0.0.1:${N3_HTTP}/api/posts?community_id=golang" "" -H "Authorization: Bearer $TOK_CAROL" | json_len)
if [ "$AFTER1" -ge "$BEFORE1" ]; then pass "node1 retained golang posts after restart"; else fail "node1 lost golang posts ($BEFORE1 -> $AFTER1)"; fi
if [ "$AFTER3" -ge "$BEFORE3" ]; then pass "node3 retained golang posts after restart"; else fail "node3 lost golang posts ($BEFORE3 -> $AFTER3)"; fi

SC=$(http_code POST "http://127.0.0.1:${N1_HTTP}/api/post" '{"community_id":"golang","title":"Post Restart Old Session","body":"cluster still writable"}' -H "Authorization: Bearer $TOK_ALICE")
if [ "$SC" = "201" ]; then
    pass "pre-restart session token still valid after restart"
else
    fail "session token lost after node restart (post returned $SC)"
fi
TOK_ALICE_R=$(cj POST "http://127.0.0.1:${N1_HTTP}/api/login" '{"username":"alice","password":"pw1"}' | json_get token)
SC=$(http_code POST "http://127.0.0.1:${N1_HTTP}/api/post" '{"community_id":"golang","title":"Post Restart","body":"cluster still writable"}' -H "Authorization: Bearer $TOK_ALICE_R")
if [ "$SC" = "201" ]; then pass "cluster writable after restart with fresh login"; else fail "fresh login post after restart -> $SC"; fi

for port in "$N1_HTTP" "$N2_HTTP" "$N3_HTTP" "$N4_HTTP" "$N5_HTTP"; do
    MEMBERS=$(curl -sf "http://127.0.0.1:${port}/api/status" | json_get member_count)
    if [[ "$MEMBERS" =~ ^[0-9]+$ ]] && [ "$MEMBERS" -ge 5 ]; then
        pass "node on :$port re-converged to 5-member gossip cluster"
    else
        fail "node on :$port member_count after restart is $MEMBERS"
    fi
done

echo
echo "── [7] Final summary ─────────────────────────────────────────────────────"
echo "════════════════════════════════════════════════════════════════════════"
printf "  Results: \033[32m%d passed\033[0m, \033[31m%d failed\033[0m\n" "$PASS" "$FAIL"
echo "════════════════════════════════════════════════════════════════════════"

if (( FAIL > 0 )); then
    echo
    echo "  Node logs:"
    echo "    ${TEST_DIR}/node1.log"
    echo "    ${TEST_DIR}/node2.log"
    echo "    ${TEST_DIR}/node3.log"
    echo "    ${TEST_DIR}/node4.log"
    echo "    ${TEST_DIR}/node5.log"
    exit 1
fi

exit 0