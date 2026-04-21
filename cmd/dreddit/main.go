package main

import (
	"flag"
	"log"
	"strings"

	"github.com/Shan-Vision05/Distributed-Reddit/internal/api"
	"github.com/Shan-Vision05/Distributed-Reddit/internal/models"
	"github.com/Shan-Vision05/Distributed-Reddit/internal/node"
)

func main() {
	// Parse command-line arguments so we can run multiple nodes on different ports
	nodeID := flag.String("id", "node1", "Unique ID for this node")
	bindAddr := flag.String("addr", ":8080", "Address to bind the HTTP API to")
	gossipPort := flag.Int("gossip-port", 0, "Gossip bind port (0 = random in 10000-19999)")
	peers := flag.String("peers", "", "Comma-separated gossip seed addresses, e.g. 127.0.0.1:11001,127.0.0.1:11002")
	dataDir := flag.String("data-dir", "", "Root directory for Raft logs, content store and snapshots (default: cwd)")
	flag.Parse()

	log.Printf("Starting Distributed Reddit Node: %s", *nodeID)

	// Parse peer list
	var peerList []string
	if *peers != "" {
		for _, p := range strings.Split(*peers, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				peerList = append(peerList, p)
			}
		}
	}

	// 1. Boot up the Node Orchestrator
	cfg := node.NodeConfig{
		GossipPort:  *gossipPort,
		GossipPeers: peerList,
		DataDir:     *dataDir,
	}
	dredditNode, err := node.NewNodeWithConfig(models.NodeID(*nodeID), *bindAddr, cfg)
	if err != nil {
		log.Fatalf("Failed to initialize node: %v", err)
	}

	// 2. Initialize the HTTP API Server
	server := api.NewServer(dredditNode, *dataDir)

	// 3. Start the server and keep the application running
	log.Printf("API Server listening on %s", *bindAddr)
	if err := server.Start(*bindAddr); err != nil {
		log.Fatalf("Server crashed: %v", err)
	}
}
