package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/chunhou/engram/internal/model"
)

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "invalid multipart form", http.StatusBadRequest)
		return
	}

	metaStr := r.FormValue("metadata")
	if metaStr == "" {
		http.Error(w, "missing metadata field", http.StatusBadRequest)
		return
	}

	var meta model.UploadMeta
	if err := json.Unmarshal([]byte(metaStr), &meta); err != nil {
		http.Error(w, "invalid metadata JSON", http.StatusBadRequest)
		return
	}

	if meta.Filename == "" || meta.Hash == "" || meta.DeviceName == "" {
		http.Error(w, "metadata requires filename, hash, and device_name", http.StatusBadRequest)
		return
	}

	// Dedup check
	var existingID, existingStatus string
	err := s.db.QueryRow(r.Context(),
		"SELECT id, status FROM files WHERE hash = $1", meta.Hash,
	).Scan(&existingID, &existingStatus)
	if err == nil {
		if existingStatus == "ready" {
			// Return existing file
			s.writeFileJSON(w, r, existingID, http.StatusOK)
			return
		}
		http.Error(w, "file already being processed", http.StatusConflict)
		return
	}

	// Upsert device
	var deviceID string
	err = s.db.QueryRow(r.Context(),
		`INSERT INTO devices (name) VALUES ($1)
		 ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
		 RETURNING id`, meta.DeviceName,
	).Scan(&deviceID)
	if err != nil {
		log.Printf("upsert device: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Generate object key
	objectKey := fmt.Sprintf("%s/%s/%s",
		meta.DeviceName,
		time.Now().Format("2006-01"),
		meta.Filename,
	)

	// Upload file to storage
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	if err := s.store.Put(r.Context(), objectKey, file, header.Size); err != nil {
		log.Printf("store put: %v", err)
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}

	// Insert file record
	var fileID string
	err = s.db.QueryRow(r.Context(),
		`INSERT INTO files (filename, size, hash, original_path, device_id, status, object_key, storage_bucket, mtime, archival_timestamp)
		 VALUES ($1, $2, $3, $4, $5, 'pending', $6, $7, $8, now())
		 RETURNING id`,
		meta.Filename, meta.Size, meta.Hash, meta.Path, deviceID, objectKey, s.bucket, meta.Mtime,
	).Scan(&fileID)
	if err != nil {
		log.Printf("insert file: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Publish ingest job
	job := model.IngestJob{
		FileID:        fileID,
		ObjectKey:     objectKey,
		StorageBucket: s.bucket,
		Filename:      meta.Filename,
		Size:          meta.Size,
		Hash:          meta.Hash,
	}
	if err := s.queue.PublishIngestJob(r.Context(), job); err != nil {
		log.Printf("publish job: %v", err)
		// File is stored, just log the queue failure
	}

	s.writeFileJSON(w, r, fileID, http.StatusCreated)
}

func (s *Server) writeFileJSON(w http.ResponseWriter, r *http.Request, fileID string, status int) {
	var f model.File
	err := s.db.QueryRow(r.Context(),
		`SELECT f.id, f.filename, f.size, f.hash, f.original_path, f.device_id, d.name,
		        f.status, f.object_key, f.storage_bucket, f.mime_type, f.page_count,
		        f.mtime, f.archival_timestamp, f.created_at, f.updated_at
		 FROM files f JOIN devices d ON f.device_id = d.id
		 WHERE f.id = $1`, fileID,
	).Scan(
		&f.ID, &f.Filename, &f.Size, &f.Hash, &f.OriginalPath, &f.DeviceID, &f.DeviceName,
		&f.Status, &f.ObjectKey, &f.StorageBucket, &f.MimeType, &f.PageCount,
		&f.Mtime, &f.ArchivalTimestamp, &f.CreatedAt, &f.UpdatedAt,
	)
	if err != nil {
		log.Printf("fetch file: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(f)
}
