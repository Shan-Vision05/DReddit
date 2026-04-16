package api

import (
	"encoding/json"
	"net/http"

	"github.com/Shan-Vision05/Distributed-Reddit/internal/models"
	"github.com/Shan-Vision05/Distributed-Reddit/internal/node"
)

// Server represents the HTTP API layer that exposes our distributed node to the outside world.
type Server struct {
	node *node.Node
}

// NewServer creates a new API server attached to the given node.
func NewServer(n *node.Node) *Server {
	return &Server{node: n}
}

// Start boots up the HTTP server on the given address (e.g., ":8080").
func (s *Server) Start(addr string) error {
	mux := http.NewServeMux()

	mux.Handle("/", http.FileServer(http.Dir("./ui")))

	// Register our API endpoints
	mux.HandleFunc("/api/join", s.handleJoinCommunity)
	mux.HandleFunc("/api/post", s.handleCreatePost)
	mux.HandleFunc("/api/vote", s.handleVote)

	return http.ListenAndServe(addr, mux)
}

// handleJoinCommunity allows the node to join a new community via an HTTP POST request.
func (s *Server) handleJoinCommunity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		CommunityID string `json:"community_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	// Route the request down to the Node Orchestrator (Step 8)
	err := s.node.JoinCommunity(models.CommunityID(req.CommunityID))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Successfully joined community: " + req.CommunityID))
}

// handleCreatePost creates a post in a specific community.
func (s *Server) handleCreatePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var post models.Post
	if err := json.NewDecoder(r.Body).Decode(&post); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	// 1. Ask the Node to find the right Community Manager
	manager, err := s.node.GetCommunity(post.CommunityID)
	if err != nil {
		http.Error(w, "Not a member of this community", http.StatusNotFound)
		return
	}

	// 2. Ask the Community Manager to handle the actual creation (Step 7)
	hash, err := manager.CreatePost(&post)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 3. Return the unique hash of the new post back to the user
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"hash": string(hash)})
}

// handleVote registers an upvote or downvote from the user.
func (s *Server) handleVote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		CommunityID string      `json:"community_id"`
		Vote        models.Vote `json:"vote"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	manager, err := s.node.GetCommunity(models.CommunityID(req.CommunityID))
	if err != nil {
		http.Error(w, "Not a member of this community", http.StatusNotFound)
		return
	}

	if err := manager.Vote(req.Vote); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Vote recorded successfully"))
}