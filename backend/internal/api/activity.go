package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/chunhou/engram/internal/auth"
	"github.com/chunhou/engram/internal/model"
)

func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	owner := auth.UsernameFromContext(r.Context())

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}

	var total int
	err := s.db.QueryRow(r.Context(),
		"SELECT COUNT(*) FROM files WHERE owner = $1", owner).Scan(&total)
	if err != nil {
		log.Printf("activity count: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	rows, err := s.db.Query(r.Context(),
		`SELECT f.id, f.filename, f.status, f.updated_at
		 FROM files f
		 WHERE f.owner = $1
		 ORDER BY f.updated_at DESC
		 LIMIT $2 OFFSET $3`,
		owner, limit, offset)
	if err != nil {
		log.Printf("activity query: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	entries := make([]model.ActivityEntry, 0, limit)
	for rows.Next() {
		var id, filename, status string
		var updatedAt time.Time
		if err := rows.Scan(&id, &filename, &status, &updatedAt); err != nil {
			log.Printf("activity scan: %v", err)
			continue
		}
		entries = append(entries, model.ActivityEntry{
			ID:          id,
			Icon:        iconForStatus(status),
			Description: descriptionForStatus(status, filename),
			Timestamp:   updatedAt.UTC().Format(time.RFC3339),
		})
	}

	resp := model.ActivityResponse{
		Entries: entries,
		Total:   total,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func iconForStatus(status string) string {
	switch status {
	case "pending":
		return "add_circle"
	case "processing":
		return "cloud_sync"
	case "ready":
		return "task_alt"
	case "failed":
		return "error"
	default:
		return "auto_fix_high"
	}
}

func descriptionForStatus(status, filename string) string {
	switch status {
	case "pending":
		return "New engram entry created: " + filename
	case "processing":
		return "Processing started for " + filename
	case "ready":
		return "Metadata extraction complete for " + filename
	case "failed":
		return "Processing failed for " + filename
	default:
		return "Neural mapping optimized for " + filename
	}
}
