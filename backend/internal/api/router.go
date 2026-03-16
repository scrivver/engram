package api

import (
	"net/http"

	"github.com/chunhou/engram/internal/queue"
	"github.com/chunhou/engram/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	db      *pgxpool.Pool
	store   storage.Store
	queue   *queue.Publisher
	bucket  string
}

func NewServer(db *pgxpool.Pool, store storage.Store, q *queue.Publisher, bucket string) *Server {
	return &Server{db: db, store: store, queue: q, bucket: bucket}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("POST /api/files", s.handleUpload)
	mux.HandleFunc("GET /api/files", s.handleListFiles)
	mux.HandleFunc("GET /api/files/{id}", s.handleGetFile)
	mux.HandleFunc("GET /api/files/{id}/download", s.handleDownload)
	mux.HandleFunc("GET /api/tags", s.handleListTags)
	mux.HandleFunc("GET /api/devices", s.handleListDevices)

	return mux
}
