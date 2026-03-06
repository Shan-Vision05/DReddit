// DReddit node - main entry point.
//
// Usage:
//
//	dreddit --config config.json
//	dreddit --node-id node1 --rpc-port 7001 --http-port 8001
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/shan/dreddit/internal/api"
	"github.com/shan/dreddit/internal/config"
	"github.com/shan/dreddit/internal/node"
)

func main() {
	// Parse flags
	configPath := flag.String("config", "", "Path to config file")
	nodeID := flag.String("node-id", "", "Node ID")
	rpcPort := flag.Int("rpc-port", 7000, "RPC port")
	httpPort := flag.Int("http-port", 8000, "HTTP API port")
	dataDir := flag.String("data-dir", "./data", "Data directory")
	bootstrap := flag.String("bootstrap", "", "Bootstrap node address")
	flag.Parse()

	// Load or create config
	var cfg *config.Config
	if *configPath != "" {
		var err error
		cfg, err = config.LoadConfig(*configPath)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
	} else {
		cfg = config.DefaultConfig()
		cfg.NodeID = *nodeID
		cfg.RPCPort = *rpcPort
		cfg.HTTPPort = *httpPort
		cfg.DataDir = *dataDir
		if *bootstrap != "" {
			cfg.BootstrapNodes = []string{*bootstrap}
		}
	}

	// Create and start node
	n, err := node.NewNode(cfg)
	if err != nil {
		log.Fatalf("Failed to create node: %v", err)
	}

	if err := n.Start(); err != nil {
		log.Fatalf("Failed to start node: %v", err)
	}

	// Start HTTP API server
	apiServer := api.NewServer(n)
	addr := fmt.Sprintf("%s:%d", cfg.Address, cfg.HTTPPort)
	go func() {
		if err := apiServer.Start(addr); err != nil {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	log.Printf("DReddit node %s running (HTTP: %s)", n.ID, addr)

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down...")
	n.Stop()
}
