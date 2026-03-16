package api

import (
	"io"
	"log"
	"net/http"
	"time"
)

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var objectKey, filename string
	err := s.db.QueryRow(r.Context(),
		"SELECT object_key, filename FROM files WHERE id = $1 AND status = 'ready'", id,
	).Scan(&objectKey, &filename)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}

	// Try presigned URL first (S3 backend)
	presigned, err := s.store.PresignedURL(r.Context(), objectKey, 15*time.Minute)
	if err == nil && presigned != "" {
		http.Redirect(w, r, presigned, http.StatusFound)
		return
	}

	// Filesystem backend: stream directly
	rc, err := s.store.Get(r.Context(), objectKey)
	if err != nil {
		log.Printf("download get: %v", err)
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	io.Copy(w, rc)
}
