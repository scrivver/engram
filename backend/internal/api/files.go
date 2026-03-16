package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/chunhou/engram/internal/model"
)

func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status := q.Get("status")
	if status == "" {
		status = "ready"
	}
	device := q.Get("device")
	search := q.Get("q")
	tags := q["tag"]
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset, _ := strconv.Atoi(q.Get("offset"))

	query := `
		SELECT DISTINCT f.id, f.filename, f.size, f.hash, f.file_path, f.device_id, d.name,
		       f.status, f.storage_type, f.mime_type, f.page_count,
		       f.mtime, f.created_at, f.updated_at
		FROM files f
		JOIN devices d ON f.device_id = d.id`

	var conditions []string
	var args []any
	argIdx := 1

	conditions = append(conditions, "f.status = $"+strconv.Itoa(argIdx))
	args = append(args, status)
	argIdx++

	if device != "" {
		conditions = append(conditions, "d.name = $"+strconv.Itoa(argIdx))
		args = append(args, device)
		argIdx++
	}

	if search != "" {
		conditions = append(conditions, "f.filename ILIKE $"+strconv.Itoa(argIdx))
		args = append(args, "%"+search+"%")
		argIdx++
	}

	if len(tags) > 0 {
		query += " JOIN file_tags ft ON f.id = ft.file_id JOIN tags t ON ft.tag_id = t.id"
		placeholders := make([]string, len(tags))
		for i, tag := range tags {
			placeholders[i] = "$" + strconv.Itoa(argIdx)
			args = append(args, tag)
			argIdx++
		}
		conditions = append(conditions, "t.name IN ("+strings.Join(placeholders, ",")+")")
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	query += " ORDER BY f.created_at DESC"
	query += " LIMIT $" + strconv.Itoa(argIdx)
	args = append(args, limit)
	argIdx++
	query += " OFFSET $" + strconv.Itoa(argIdx)
	args = append(args, offset)

	rows, err := s.db.Query(r.Context(), query, args...)
	if err != nil {
		log.Printf("list files: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	files := make([]model.File, 0)
	for rows.Next() {
		var f model.File
		if err := rows.Scan(
			&f.ID, &f.Filename, &f.Size, &f.Hash, &f.FilePath, &f.DeviceID, &f.DeviceName,
			&f.Status, &f.StorageType, &f.MimeType, &f.PageCount,
			&f.Mtime, &f.CreatedAt, &f.UpdatedAt,
		); err != nil {
			log.Printf("scan file: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		files = append(files, f)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

func (s *Server) handleGetFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var f model.File
	err := s.db.QueryRow(r.Context(),
		`SELECT f.id, f.filename, f.size, f.hash, f.file_path, f.device_id, d.name,
		        f.status, f.storage_type, f.mime_type, f.page_count,
		        f.extracted_text, f.mtime, f.created_at, f.updated_at
		 FROM files f JOIN devices d ON f.device_id = d.id
		 WHERE f.id = $1`, id,
	).Scan(
		&f.ID, &f.Filename, &f.Size, &f.Hash, &f.FilePath, &f.DeviceID, &f.DeviceName,
		&f.Status, &f.StorageType, &f.MimeType, &f.PageCount,
		&f.ExtractedText, &f.Mtime, &f.CreatedAt, &f.UpdatedAt,
	)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}

	// Fetch tags
	tagRows, err := s.db.Query(r.Context(),
		`SELECT t.name FROM tags t JOIN file_tags ft ON t.id = ft.tag_id WHERE ft.file_id = $1`, id)
	if err == nil {
		defer tagRows.Close()
		for tagRows.Next() {
			var name string
			if tagRows.Scan(&name) == nil {
				f.Tags = append(f.Tags, name)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(f)
}

func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(),
		`SELECT t.id, t.name, COUNT(ft.file_id) as file_count
		 FROM tags t LEFT JOIN file_tags ft ON t.id = ft.tag_id
		 GROUP BY t.id, t.name ORDER BY file_count DESC`)
	if err != nil {
		log.Printf("list tags: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	tags := make([]model.Tag, 0)
	for rows.Next() {
		var t model.Tag
		if rows.Scan(&t.ID, &t.Name, &t.FileCount) == nil {
			tags = append(tags, t)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tags)
}

func (s *Server) handleListDevices(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(r.Context(),
		`SELECT id, name, created_at FROM devices ORDER BY name`)
	if err != nil {
		log.Printf("list devices: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	devices := make([]model.Device, 0)
	for rows.Next() {
		var d model.Device
		if rows.Scan(&d.ID, &d.Name, &d.CreatedAt) == nil {
			devices = append(devices, d)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(devices)
}
