package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/Shan-Vision05/Distributed-Reddit/internal/models"
	"github.com/Shan-Vision05/Distributed-Reddit/internal/node"
)

type Server struct {
	node     *node.Node
	mu       sync.RWMutex
	users    map[string]string // Maps username -> hashed password
	authFile string
}

func NewServer(n *node.Node) *Server {
	s := &Server{
		node:     n,
		users:    make(map[string]string),
		authFile: "users.json",
	}
	s.loadUsers()
	return s
}

// --- User Account Management ---

func (s *Server) loadUsers() {
	data, err := os.ReadFile(s.authFile)
	if err == nil {
		json.Unmarshal(data, &s.users)
	}
}

func (s *Server) saveUsers() {
	data, _ := json.MarshalIndent(s.users, "", "  ")
	_ = os.WriteFile(s.authFile, data, 0644)
}

func hashPassword(password string) string {
	h := sha256.Sum256([]byte(password))
	return hex.EncodeToString(h[:])
}

func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.users[req.Username]; exists {
		http.Error(w, "Username is already taken", http.StatusConflict)
		return
	}

	s.users[req.Username] = hashPassword(req.Password)
	s.saveUsers()
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	expectedHash, exists := s.users[req.Username]
	if !exists || expectedHash != hashPassword(req.Password) {
		http.Error(w, "Invalid username or password", http.StatusUnauthorized)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// --- Moderation Helpers ---

func isBanned(logs []models.ModerationAction, userID string) bool {
	banned := false
	for _, log := range logs {
		// Convert TargetHash and ActionType to standard strings for comparison
		if string(log.TargetHash) == userID {
			if string(log.ActionType) == "BAN_USER" {
				banned = true
			} else if string(log.ActionType) == "UNBAN_USER" {
				banned = false
			}
		}
	}
	return banned
}

// --- Main Server Setup ---

func (s *Server) Start(addr string) error {
	mux := http.NewServeMux()

	mux.Handle("/", http.FileServer(http.Dir("./ui")))

	// Auth Endpoints
	mux.HandleFunc("/api/signup", s.handleSignup)
	mux.HandleFunc("/api/login", s.handleLogin)

	// App Endpoints
	mux.HandleFunc("/api/communities", s.handleGetCommunities)
	mux.HandleFunc("/api/join", s.handleJoinCommunity)
	mux.HandleFunc("/api/posts", s.handleGetPosts)
	mux.HandleFunc("/api/post", s.handleCreatePost)
	mux.HandleFunc("/api/comments", s.handleGetComments)
	mux.HandleFunc("/api/comment", s.handleCreateComment)
	mux.HandleFunc("/api/vote", s.handleVote)
	mux.HandleFunc("/api/moderate", s.handleModerate)

	return http.ListenAndServe(addr, mux)
}

func (s *Server) handleGetCommunities(w http.ResponseWriter, r *http.Request) {
	comms := s.node.GetJoinedCommunities()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(comms)
}

func (s *Server) handleJoinCommunity(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CommunityID string `json:"community_id"`
		UserID      string `json:"user_id"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if req.UserID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	manager, _ := s.node.GetCommunity(models.CommunityID(req.CommunityID))
	if manager != nil && isBanned(manager.GetModerationLog(), req.UserID) {
		http.Error(w, "You are banned from this community", http.StatusForbidden)
		return
	}

	if err := s.node.JoinCommunity(models.CommunityID(req.CommunityID)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleGetPosts(w http.ResponseWriter, r *http.Request) {
	commID := r.URL.Query().Get("community_id")
	manager, err := s.node.GetCommunity(models.CommunityID(commID))
	if err != nil {
		http.Error(w, "Not a member", http.StatusNotFound)
		return
	}

	// NEW: Scan the moderation log once to build a map of banned users and deleted posts
	deletedPosts := make(map[models.ContentHash]bool)
	bannedUsers := make(map[string]bool)

	for _, log := range manager.GetModerationLog() {
		if string(log.ActionType) == "DELETE_POST" {
			deletedPosts[log.TargetHash] = true
		} else if string(log.ActionType) == "BAN_USER" {
			bannedUsers[string(log.TargetHash)] = true
		} else if string(log.ActionType) == "UNBAN_USER" {
			bannedUsers[string(log.TargetHash)] = false
		}
	}

	posts, scores := manager.GetPosts()
	type PostResponse struct {
		*models.Post
		Score int64 `json:"score"`
	}
	
	var res []PostResponse
	for _, p := range posts {
		// NEW: Only add the post to the feed if it is NOT deleted AND the author is NOT banned
		if !deletedPosts[p.Hash] && !bannedUsers[string(p.AuthorID)] {
			res = append(res, PostResponse{Post: p, Score: scores[p.Hash]})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func (s *Server) handleCreatePost(w http.ResponseWriter, r *http.Request) {
	var post models.Post
	json.NewDecoder(r.Body).Decode(&post)
	
	if string(post.AuthorID) == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	manager, err := s.node.GetCommunity(post.CommunityID)
	if err != nil {
		http.Error(w, "Not a member", http.StatusNotFound)
		return
	}

	// Cast AuthorID to string for the isBanned function
	if isBanned(manager.GetModerationLog(), string(post.AuthorID)) {
		http.Error(w, "You are banned from posting in this community", http.StatusForbidden)
		return
	}

	post.CreatedAt = time.Now()
	post.Hash = post.ComputeHash()

	hash, err := manager.CreatePost(&post)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"hash": string(hash)})
}

func (s *Server) handleGetComments(w http.ResponseWriter, r *http.Request) {
	commID := r.URL.Query().Get("community_id")
	postHash := r.URL.Query().Get("post_hash")
	
	manager, err := s.node.GetCommunity(models.CommunityID(commID))
	if err != nil {
		http.Error(w, "Not a member", http.StatusNotFound)
		return
	}

	// NEW: Scan the moderation log to build a map of banned users for comments
	bannedUsers := make(map[string]bool)
	for _, log := range manager.GetModerationLog() {
		if string(log.ActionType) == "BAN_USER" {
			bannedUsers[string(log.TargetHash)] = true
		} else if string(log.ActionType) == "UNBAN_USER" {
			bannedUsers[string(log.TargetHash)] = false
		}
	}

	comments, scores := manager.GetComments(models.ContentHash(postHash))
	type CommentResponse struct {
		*models.Comment
		Score int64 `json:"score"`
	}

	var res []CommentResponse
	for _, c := range comments {
		// NEW: Only show the comment if the author is NOT banned
		if !bannedUsers[string(c.AuthorID)] {
			res = append(res, CommentResponse{Comment: c, Score: scores[c.Hash]})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func (s *Server) handleCreateComment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CommunityID string         `json:"community_id"`
		Comment     models.Comment `json:"comment"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if string(req.Comment.AuthorID) == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	manager, err := s.node.GetCommunity(models.CommunityID(req.CommunityID))
	if err != nil {
		http.Error(w, "Not a member", http.StatusNotFound)
		return
	}

	// Cast AuthorID to string for the isBanned function
	if isBanned(manager.GetModerationLog(), string(req.Comment.AuthorID)) {
		http.Error(w, "You are banned from commenting in this community", http.StatusForbidden)
		return
	}

	req.Comment.CreatedAt = time.Now()
	req.Comment.Hash = req.Comment.ComputeHash()

	hash, err := manager.CreateComment(&req.Comment)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"hash": string(hash)})
}

func (s *Server) handleVote(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CommunityID string      `json:"community_id"`
		Vote        models.Vote `json:"vote"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if string(req.Vote.UserID) == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	manager, err := s.node.GetCommunity(models.CommunityID(req.CommunityID))
	if err != nil {
		http.Error(w, "Not a member", http.StatusNotFound)
		return
	}

	// Cast UserID to string for the isBanned function
	if isBanned(manager.GetModerationLog(), string(req.Vote.UserID)) {
		http.Error(w, "You are banned from voting in this community", http.StatusForbidden)
		return
	}

	req.Vote.Timestamp = time.Now()

	if err := manager.Vote(req.Vote); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleModerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		CommunityID string `json:"community_id"`
		ActionType  string `json:"action_type"`
		Target      string `json:"target"`
		UserID      string `json:"user_id"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	manager, err := s.node.GetCommunity(models.CommunityID(req.CommunityID))
	if err != nil {
		http.Error(w, "Not a member", http.StatusNotFound)
		return
	}

	// Authorization Guard
	if req.UserID != "admin" {
		if req.ActionType == "BAN_USER" {
			http.Error(w, "Forbidden: Only admins can ban users", http.StatusForbidden)
			return
		}
		if req.ActionType == "DELETE_POST" {
			// Users can only delete their own posts
			posts, _ := manager.GetPosts()
			isAuthor := false
			for _, p := range posts {
				// Cast p.AuthorID to string to compare with req.UserID
				if string(p.Hash) == req.Target && string(p.AuthorID) == req.UserID {
					isAuthor = true
					break
				}
			}
			if !isAuthor {
				http.Error(w, "Forbidden: You can only delete your own posts", http.StatusForbidden)
				return
			}
		}
	}

	// Cast ActionType to your specific models.ModerationActionType
	action := models.ModerationAction{
		CommunityID: models.CommunityID(req.CommunityID),
		TargetHash:  models.ContentHash(req.Target),
		ActionType:  models.ModerationActionType(req.ActionType),
		Reason:      "Moderated via API",
	}

	// Propose to Raft Consensus
	if err := manager.Moderate(action); err != nil {
		http.Error(w, "Raft consensus failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}