package api

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/chunhou/engram/internal/auth"
	"github.com/chunhou/engram/internal/model"
)

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	owner := auth.UsernameFromContext(r.Context())

	resp := model.StatsResponse{
		FilesByStatus: map[string]int{},
	}

	var total int
	err := s.db.QueryRow(r.Context(),
		"SELECT COUNT(*) FROM files WHERE owner = $1", owner).Scan(&total)
	if err != nil {
		log.Printf("stats total_files: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	resp.TotalFiles = total

	rows, err := s.db.Query(r.Context(),
		"SELECT status, COUNT(*) FROM files WHERE owner = $1 GROUP BY status", owner)
	if err != nil {
		log.Printf("stats files_by_status: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	resp.Status = "healthy"
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			log.Printf("stats scan: %v", err)
			continue
		}
		resp.FilesByStatus[status] = count
	}

	if resp.FilesByStatus["failed"] > 0 {
		resp.Status = "degraded"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
