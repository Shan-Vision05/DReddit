// Package api provides the HTTP REST API for DReddit clients.
package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/shan/dreddit/internal/models"
	"github.com/shan/dreddit/internal/node"
)

// Server is the HTTP API server for DReddit.
type Server struct {
	node *node.Node
	mux  *http.ServeMux
}

// NewServer creates a new API server.
func NewServer(n *node.Node) *Server {
	s := &Server{
		node: n,
		mux:  http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	// Health & info
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /info", s.handleNodeInfo)

	// Communities
	s.mux.HandleFunc("GET /communities", s.handleListCommunities)
	s.mux.HandleFunc("POST /communities", s.handleCreateCommunity)
	s.mux.HandleFunc("GET /communities/{id}", s.handleGetCommunity)

	// Posts
	s.mux.HandleFunc("POST /communities/{id}/posts", s.handleCreatePost)
	s.mux.HandleFunc("GET /communities/{id}/posts", s.handleListPosts)
	s.mux.HandleFunc("GET /posts/{hash}", s.handleGetPost)

	// Comments
	s.mux.HandleFunc("POST /posts/{hash}/comments", s.handleCreateComment)
	s.mux.HandleFunc("GET /posts/{hash}/comments", s.handleGetComments)

	// Votes
	s.mux.HandleFunc("POST /vote", s.handleVote)

	// Moderation
	s.mux.HandleFunc("POST /moderate", s.handleModerate)

	// DHT / Cluster
	s.mux.HandleFunc("GET /cluster/nodes", s.handleListNodes)
	s.mux.HandleFunc("POST /cluster/join", s.handleJoinCluster)
}

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// Start starts the HTTP server.
func (s *Server) Start(addr string) error {
	log.Printf("[API] Starting HTTP server on %s", addr)
	return http.ListenAndServe(addr, s.mux)
}

// -------------------------------------------------------------------
// Handlers
// -------------------------------------------------------------------

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"node_id": s.node.ID,
		"running": s.node.IsRunning(),
	})
}

func (s *Server) handleNodeInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.node.GetNodeInfo())
}

func (s *Server) handleListCommunities(w http.ResponseWriter, r *http.Request) {
	communities := s.node.CommunityManager.ListCommunities()
	writeJSON(w, http.StatusOK, communities)
}

func (s *Server) handleCreateCommunity(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		CreatorID   string `json:"creator_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	comm, err := s.node.CreateCommunity(req.Name, req.Description, models.UserID(req.CreatorID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, comm)
}

func (s *Server) handleGetCommunity(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	state, err := s.node.CommunityManager.GetCommunity(models.CommunityID(id))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, state.Community)
}

func (s *Server) handleCreatePost(w http.ResponseWriter, r *http.Request) {
	communityID := r.PathValue("id")
	state, err := s.node.CommunityManager.GetCommunity(models.CommunityID(communityID))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var req struct {
		AuthorID string `json:"author_id"`
		Title    string `json:"title"`
		Body     string `json:"body"`
		URL      string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	post, err := state.CreatePost(models.UserID(req.AuthorID), req.Title, req.Body, req.URL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, post)
}

func (s *Server) handleListPosts(w http.ResponseWriter, r *http.Request) {
	communityID := r.PathValue("id")
	state, err := s.node.CommunityManager.GetCommunity(models.CommunityID(communityID))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	posts, err := state.GetPosts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Enrich with vote scores
	type PostWithScore struct {
		*models.Post
		Score int64 `json:"score"`
	}

	result := make([]PostWithScore, 0, len(posts))
	for _, post := range posts {
		score, _ := state.Store.GetVoteScore(post.Hash)
		result = append(result, PostWithScore{Post: post, Score: score})
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleGetPost(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")

	// Search all communities for the post
	for _, comm := range s.node.CommunityManager.ListCommunities() {
		state, err := s.node.CommunityManager.GetCommunity(comm.ID)
		if err != nil {
			continue
		}
		post, comments, err := state.GetPostWithComments(models.ContentHash(hash))
		if err != nil {
			continue
		}

		score, _ := state.Store.GetVoteScore(post.Hash)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"post":     post,
			"comments": comments,
			"score":    score,
		})
		return
	}

	writeError(w, http.StatusNotFound, "post not found")
}

func (s *Server) handleCreateComment(w http.ResponseWriter, r *http.Request) {
	postHash := r.PathValue("hash")

	var req struct {
		AuthorID   string `json:"author_id"`
		Body       string `json:"body"`
		ParentHash string `json:"parent_hash"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Find the community containing this post
	for _, comm := range s.node.CommunityManager.ListCommunities() {
		state, err := s.node.CommunityManager.GetCommunity(comm.ID)
		if err != nil {
			continue
		}
		_, err = state.Store.GetPost(models.ContentHash(postHash))
		if err != nil {
			continue
		}

		comment, err := state.CreateComment(
			models.UserID(req.AuthorID),
			models.ContentHash(postHash),
			models.ContentHash(req.ParentHash),
			req.Body,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusCreated, comment)
		return
	}

	writeError(w, http.StatusNotFound, "post not found")
}

func (s *Server) handleGetComments(w http.ResponseWriter, r *http.Request) {
	postHash := r.PathValue("hash")

	for _, comm := range s.node.CommunityManager.ListCommunities() {
		state, err := s.node.CommunityManager.GetCommunity(comm.ID)
		if err != nil {
			continue
		}
		_, comments, err := state.GetPostWithComments(models.ContentHash(postHash))
		if err != nil {
			continue
		}
		writeJSON(w, http.StatusOK, comments)
		return
	}

	writeError(w, http.StatusNotFound, "post not found")
}

func (s *Server) handleVote(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID     string `json:"user_id"`
		TargetHash string `json:"target_hash"`
		Value      int    `json:"value"` // 1 = upvote, -1 = downvote
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	voteType := models.Upvote
	if req.Value < 0 {
		voteType = models.Downvote
	}

	// Find community containing the target
	for _, comm := range s.node.CommunityManager.ListCommunities() {
		state, err := s.node.CommunityManager.GetCommunity(comm.ID)
		if err != nil {
			continue
		}

		err = state.Vote(models.UserID(req.UserID), models.ContentHash(req.TargetHash), voteType, s.node.ID)
		if err != nil {
			continue
		}

		score, _ := state.Store.GetVoteScore(models.ContentHash(req.TargetHash))
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"target_hash": req.TargetHash,
			"new_score":   score,
		})
		return
	}

	writeError(w, http.StatusNotFound, "target not found")
}

func (s *Server) handleModerate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ModeratorID string `json:"moderator_id"`
		CommunityID string `json:"community_id"`
		ActionType  string `json:"action_type"`
		TargetHash  string `json:"target_hash"`
		TargetUser  string `json:"target_user"`
		Reason      string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	state, err := s.node.CommunityManager.GetCommunity(models.CommunityID(req.CommunityID))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// Verify moderator
	if !state.IsModerator(models.UserID(req.ModeratorID)) {
		writeError(w, http.StatusForbidden, "user is not a moderator")
		return
	}

	action := models.ModerationAction{
		CommunityID: models.CommunityID(req.CommunityID),
		ModeratorID: models.UserID(req.ModeratorID),
		ActionType:  models.ModActionType(req.ActionType),
		TargetHash:  models.ContentHash(req.TargetHash),
		TargetUser:  models.UserID(req.TargetUser),
		Reason:      req.Reason,
	}

	// In full implementation, this goes through Raft consensus
	// For now, apply directly
	if err := state.ApplyModeration(action); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "moderation action applied",
	})
}

func (s *Server) handleListNodes(w http.ResponseWriter, r *http.Request) {
	nodes := s.node.DHT.GetAllNodes()
	writeJSON(w, http.StatusOK, nodes)
}

func (s *Server) handleJoinCluster(w http.ResponseWriter, r *http.Request) {
	var req models.NodeInfo
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	s.node.JoinPeer(req)
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "peer added",
	})
}

// -------------------------------------------------------------------
// Helpers
// -------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{
		"error": message,
	})
}

// FormatAddr returns the formatted address string.
func FormatAddr(host string, port int) string {
	return fmt.Sprintf("%s:%d", host, port)
}
