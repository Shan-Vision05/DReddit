// Package dht implements a simplified Distributed Hash Table for
// decentralized node discovery and community-to-node mapping.
//
// Inspired by Kademlia, this DHT maps community IDs to the set of
// replica nodes responsible for them.
package dht

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"sort"
	"sync"

	"github.com/shan/dreddit/internal/models"
)

// HashID is a 256-bit identifier used in the DHT.
type HashID string

// DHT implements a simplified distributed hash table for community lookup.
type DHT struct {
	mu   sync.RWMutex
	self models.NodeID

	// Node routing table
	nodes map[models.NodeID]*models.NodeInfo

	// Community -> replica set mapping
	communityMap map[models.CommunityID][]models.NodeID

	// Replication factor (how many nodes per community)
	replicationFactor int
}

// NewDHT creates a new DHT instance.
func NewDHT(selfID models.NodeID, replicationFactor int) *DHT {
	return &DHT{
		self:              selfID,
		nodes:             make(map[models.NodeID]*models.NodeInfo),
		communityMap:      make(map[models.CommunityID][]models.NodeID),
		replicationFactor: replicationFactor,
	}
}

// AddNode adds a node to the DHT routing table.
func (d *DHT) AddNode(info *models.NodeInfo) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.nodes[info.ID] = info
	log.Printf("[DHT %s] Added node %s", d.self, info.ID)
}

// RemoveNode removes a node from the DHT.
func (d *DHT) RemoveNode(nodeID models.NodeID) {
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.nodes, nodeID)

	// Remove from community mappings
	for cid, nodes := range d.communityMap {
		filtered := make([]models.NodeID, 0)
		for _, n := range nodes {
			if n != nodeID {
				filtered = append(filtered, n)
			}
		}
		d.communityMap[cid] = filtered
	}
}

// LookupCommunity returns the replica set for a given community.
func (d *DHT) LookupCommunity(communityID models.CommunityID) ([]models.NodeID, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	nodes, ok := d.communityMap[communityID]
	if !ok || len(nodes) == 0 {
		return nil, fmt.Errorf("community %s not found in DHT", communityID)
	}

	return nodes, nil
}

// AssignCommunity assigns a community to a replica set of nodes.
// Uses consistent hashing to determine which nodes should store the community.
func (d *DHT) AssignCommunity(communityID models.CommunityID) ([]models.NodeID, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.nodes) < d.replicationFactor {
		return nil, fmt.Errorf("not enough nodes (%d) for replication factor %d",
			len(d.nodes), d.replicationFactor)
	}

	// Hash the community ID
	communityHash := hashKey(string(communityID))

	// Get all node IDs sorted by distance to community hash
	type nodeDistance struct {
		nodeID   models.NodeID
		distance string
	}

	distances := make([]nodeDistance, 0, len(d.nodes))
	for nodeID := range d.nodes {
		nodeHash := hashKey(string(nodeID))
		dist := xorDistance(communityHash, nodeHash)
		distances = append(distances, nodeDistance{nodeID, dist})
	}

	// Sort by XOR distance (Kademlia-style)
	sort.Slice(distances, func(i, j int) bool {
		return distances[i].distance < distances[j].distance
	})

	// Select the closest N nodes
	replicaSet := make([]models.NodeID, 0, d.replicationFactor)
	for i := 0; i < d.replicationFactor && i < len(distances); i++ {
		replicaSet = append(replicaSet, distances[i].nodeID)
	}

	d.communityMap[communityID] = replicaSet
	log.Printf("[DHT %s] Assigned community %s to nodes: %v", d.self, communityID, replicaSet)

	return replicaSet, nil
}

// RegisterCommunity explicitly registers a community->nodes mapping.
func (d *DHT) RegisterCommunity(communityID models.CommunityID, nodes []models.NodeID) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.communityMap[communityID] = nodes
}

// GetAllNodes returns all known nodes.
func (d *DHT) GetAllNodes() []*models.NodeInfo {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make([]*models.NodeInfo, 0, len(d.nodes))
	for _, info := range d.nodes {
		result = append(result, info)
	}
	return result
}

// GetAllCommunities returns all community->node mappings.
func (d *DHT) GetAllCommunities() map[models.CommunityID][]models.NodeID {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make(map[models.CommunityID][]models.NodeID)
	for k, v := range d.communityMap {
		nodes := make([]models.NodeID, len(v))
		copy(nodes, v)
		result[k] = nodes
	}
	return result
}

// -------------------------------------------------------------------
// Helper functions
// -------------------------------------------------------------------

func hashKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:])
}

func xorDistance(a, b string) string {
	aBytes, _ := hex.DecodeString(a)
	bBytes, _ := hex.DecodeString(b)

	result := make([]byte, len(aBytes))
	for i := range aBytes {
		result[i] = aBytes[i] ^ bBytes[i]
	}
	return hex.EncodeToString(result)
}
