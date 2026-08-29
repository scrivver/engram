package api

import (
	"context"
	"net/http"

	"github.com/chunhou/engram/internal/config"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type dbQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type Server struct {
	db                 dbQuerier
	presignURLTemplate string
	cfg                *config.Config
}

type Option func(*Server)

func WithPresignURLTemplate(tmpl string) Option {
	return func(s *Server) { s.presignURLTemplate = tmpl }
}

func WithConfig(cfg *config.Config) Option {
	return func(s *Server) { s.cfg = cfg }
}

func NewServer(db *pgxpool.Pool, opts ...Option) *Server {
	s := &Server{db: db}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Middleware is an HTTP middleware applied to every API route except /api/health.
type Middleware func(http.Handler) http.Handler

func (s *Server) Routes(authMW Middleware) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", s.handleHealth)

	protected := http.NewServeMux()
	protected.HandleFunc("GET /api/files", s.handleListFiles)
	protected.HandleFunc("GET /api/files/{id}", s.handleGetFile)
	protected.HandleFunc("GET /api/folders", s.handleListFolders)
	protected.HandleFunc("GET /api/tags", s.handleListTags)
	protected.HandleFunc("GET /api/devices", s.handleListDevices)
	protected.HandleFunc("GET /api/stats", s.handleStats)
	protected.HandleFunc("GET /api/activity", s.handleActivity)

	var h http.Handler = protected
	if authMW != nil {
		h = authMW(protected)
	}
	mux.Handle("/api/", h)

	return mux
}
