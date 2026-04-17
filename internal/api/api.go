package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/Shan-Vision05/Distributed-Reddit/internal/models"
	"github.com/Shan-Vision05/Distributed-Reddit/internal/node"
)

type Server struct {
	node *node.Node
}

func NewServer(n *node.Node) *Server {
	return &Server{node: n}
}

func (s *Server) Start(addr string) error {
	mux := http.NewServeMux()

	mux.Handle("/", http.FileServer(http.Dir("./ui")))

	mux.HandleFunc("/api/communities", s.handleGetCommunities)
	mux.HandleFunc("/api/join", s.handleJoinCommunity)
	mux.HandleFunc("/api/posts", s.handleGetPosts)
	mux.HandleFunc("/api/post", s.handleCreatePost)
	mux.HandleFunc("/api/comments", s.handleGetComments)
	mux.HandleFunc("/api/comment", s.handleCreateComment)
	mux.HandleFunc("/api/vote", s.handleVote)

	return http.ListenAndServe(addr, mux)
}

func (s *Server) handleGetCommunities(w http.ResponseWriter, r *http.Request) {
	comms := s.node.GetJoinedCommunities()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(comms)
}

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
		http.Error(w, "Not a member of this community", http.StatusNotFound)
		return
	}

	posts, scores := manager.GetPosts()
	
	type PostResponse struct {
		*models.Post
		Score int64 `json:"score"`
	}
	
	var res []PostResponse
	for _, p := range posts {
		res = append(res, PostResponse{Post: p, Score: scores[p.Hash]})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

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
	
	post.CreatedAt = time.Now()
	post.Hash = post.ComputeHash()

	manager, err := s.node.GetCommunity(post.CommunityID)
	if err != nil {
		http.Error(w, "Not a member of this community", http.StatusNotFound)
		return
	}

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
		http.Error(w, "Not a member of this community", http.StatusNotFound)
		return
	}

	comments, scores := manager.GetComments(models.ContentHash(postHash))
	
	type CommentResponse struct {
		*models.Comment
		Score int64 `json:"score"`
	}

	var res []CommentResponse
	for _, c := range comments {
		res = append(res, CommentResponse{Comment: c, Score: scores[c.Hash]})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func (s *Server) handleCreateComment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		CommunityID string         `json:"community_id"`
		Comment     models.Comment `json:"comment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	req.Comment.CreatedAt = time.Now()
	req.Comment.Hash = req.Comment.ComputeHash()

	manager, err := s.node.GetCommunity(models.CommunityID(req.CommunityID))
	if err != nil {
		http.Error(w, "Not a member of this community", http.StatusNotFound)
		return
	}

	hash, err := manager.CreateComment(&req.Comment)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"hash": string(hash)})
}

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

	req.Vote.Timestamp = time.Now()

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
}