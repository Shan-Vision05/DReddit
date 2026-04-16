package main

import (
	"flag"
	"log"

	"github.com/Shan-Vision05/Distributed-Reddit/internal/api"
	"github.com/Shan-Vision05/Distributed-Reddit/internal/models"
	"github.com/Shan-Vision05/Distributed-Reddit/internal/node"
)

func main() {
	// Parse command-line arguments so we can run multiple nodes on different ports
	nodeID := flag.String("id", "node1", "Unique ID for this node")
	bindAddr := flag.String("addr", ":8080", "Address to bind the HTTP API to")
	flag.Parse()

	log.Printf("Starting Distributed Reddit Node: %s", *nodeID)

	// 1. Boot up the Node Orchestrator (from Step 8)
	// This sets up the global DHT and Gossip network connections.
	dredditNode, err := node.NewNode(models.NodeID(*nodeID), *bindAddr)
	if err != nil {
		log.Fatalf("Failed to initialize node: %v", err)
	}

	// 2. Initialize the HTTP API Server (from Step 9)
	// We pass the orchestrator into the API so it knows where to route web requests.
	server := api.NewServer(dredditNode)

	// 3. Start the server and keep the application running
	log.Printf("API Server listening on %s", *bindAddr)
	if err := server.Start(*bindAddr); err != nil {
		log.Fatalf("Server crashed: %v", err)
	}
}