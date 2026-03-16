package api

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	db *pgxpool.Pool
}

func NewServer(db *pgxpool.Pool) *Server {
	return &Server{db: db}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/files", s.handleListFiles)
	mux.HandleFunc("GET /api/files/{id}", s.handleGetFile)
	mux.HandleFunc("GET /api/tags", s.handleListTags)
	mux.HandleFunc("GET /api/devices", s.handleListDevices)

	return mux
}
