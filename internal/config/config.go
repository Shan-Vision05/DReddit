// Package config provides configuration for DReddit nodes.
package config

import (
	"encoding/json"
	"os"
	"time"
)

// Config holds all configuration for a DReddit node.
type Config struct {
	// Node identity
	NodeID   string `json:"node_id"`
	Address  string `json:"address"`   // bind address
	RPCPort  int    `json:"rpc_port"`  // gRPC port
	HTTPPort int    `json:"http_port"` // REST API port

	// Cluster
	BootstrapNodes []string `json:"bootstrap_nodes"` // seed node addresses
	ReplicaCount   int      `json:"replica_count"`   // replicas per community (3-5)

	// Storage
	DataDir string `json:"data_dir"` // directory for persistent data

	// Consensus (Raft)
	RaftDir             string        `json:"raft_dir"`
	RaftElectionTimeout time.Duration `json:"raft_election_timeout"`
	RaftHeartbeat       time.Duration `json:"raft_heartbeat"`

	// CRDT
	CRDTSyncInterval time.Duration `json:"crdt_sync_interval"` // gossip interval

	// DHT
	DHTReplicationFactor int `json:"dht_replication_factor"`

	// Timeouts
	RPCTimeout     time.Duration `json:"rpc_timeout"`
	GossipInterval time.Duration `json:"gossip_interval"`
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		NodeID:               "",
		Address:              "0.0.0.0",
		RPCPort:              7000,
		HTTPPort:             8000,
		BootstrapNodes:       []string{},
		ReplicaCount:         3,
		DataDir:              "./data",
		RaftDir:              "./data/raft",
		RaftElectionTimeout:  1000 * time.Millisecond,
		RaftHeartbeat:        150 * time.Millisecond,
		CRDTSyncInterval:     5 * time.Second,
		DHTReplicationFactor: 3,
		RPCTimeout:           5 * time.Second,
		GossipInterval:       2 * time.Second,
	}
}

// LoadConfig reads a config from a JSON file.
func LoadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	cfg := DefaultConfig()
	if err := json.NewDecoder(f).Decode(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// SaveConfig writes config to a JSON file.
func (c *Config) SaveConfig(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(c)
}
