package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/chunhou/engram/internal/auth"
	"github.com/chunhou/engram/internal/model"
)

// File-type allowlist: maps a user-facing category to a SQL fragment that
// filters `f.mime_type`. Keeping the map here (rather than prefix-matching on
// whatever string the client sends) is deliberate — unknown categories are
// rejected, and `other` gets explicit semantics instead of a fragile NOT LIKE
// chain built client-side.
var fileTypeFilter = map[string]string{
	"image": "f.mime_type LIKE 'image/%'",
	"video": "f.mime_type LIKE 'video/%'",
	"audio": "f.mime_type LIKE 'audio/%'",
	"pdf":   "f.mime_type = 'application/pdf'",
	"other": "(f.mime_type IS NULL OR (" +
		"f.mime_type NOT LIKE 'image/%' AND " +
		"f.mime_type NOT LIKE 'video/%' AND " +
		"f.mime_type NOT LIKE 'audio/%' AND " +
		"f.mime_type <> 'application/pdf'))",
}

// Sort allowlist: user-facing key → ORDER BY clause.
var sortOrder = map[string]string{
	"created_desc": "f.created_at DESC",
	"mtime_desc":   "f.mtime DESC",
	"size_desc":    "f.size DESC",
	"size_asc":     "f.size ASC",
}

// parseDate accepts RFC 3339 or a bare YYYY-MM-DD.
func parseDate(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	username := auth.UsernameFromContext(r.Context())

	q := r.URL.Query()
	status := q.Get("status")
	if status == "" {
		status = "ready"
	}
	device := q.Get("device")
	search := q.Get("q")
	tags := q["tag"]
	fileType := q.Get("type")
	fromStr := q.Get("from")
	toStr := q.Get("to")
	sortKey := q.Get("sort")
	if sortKey == "" {
		sortKey = "created_desc"
	}

	if fileType != "" {
		if _, ok := fileTypeFilter[fileType]; !ok {
			http.Error(w, "invalid type", http.StatusBadRequest)
			return
		}
	}
	orderBy, ok := sortOrder[sortKey]
	if !ok {
		http.Error(w, "invalid sort", http.StatusBadRequest)
		return
	}

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

	conditions = append(conditions, "f.owner = $"+strconv.Itoa(argIdx))
	args = append(args, username)
	argIdx++

	if device != "" {
		conditions = append(conditions, "d.name = $"+strconv.Itoa(argIdx))
		args = append(args, device)
		argIdx++
	}

	if search != "" {
		// Full-text match against the generated tsv column (see migration 003).
		// websearch_to_tsquery supports Google-style syntax: quoted phrases,
		// `-term` exclusions, implicit AND across terms.
		conditions = append(conditions,
			"f.tsv @@ websearch_to_tsquery('simple', $"+strconv.Itoa(argIdx)+")")
		args = append(args, search)
		argIdx++
	}

	if fileType != "" {
		conditions = append(conditions, fileTypeFilter[fileType])
	}

	if fromStr != "" {
		t, ok := parseDate(fromStr)
		if !ok {
			http.Error(w, "invalid 'from' date", http.StatusBadRequest)
			return
		}
		conditions = append(conditions, "f.mtime >= $"+strconv.Itoa(argIdx))
		args = append(args, t)
		argIdx++
	}
	if toStr != "" {
		t, ok := parseDate(toStr)
		if !ok {
			http.Error(w, "invalid 'to' date", http.StatusBadRequest)
			return
		}
		conditions = append(conditions, "f.mtime <= $"+strconv.Itoa(argIdx))
		args = append(args, t)
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

	query += " WHERE " + strings.Join(conditions, " AND ")
	query += " ORDER BY " + orderBy
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
		f.DownloadURL = s.renderDownloadURL(f.StorageType, f.FilePath)
		files = append(files, f)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

func (s *Server) handleGetFile(w http.ResponseWriter, r *http.Request) {
	username := auth.UsernameFromContext(r.Context())
	id := r.PathValue("id")

	var f model.File
	err := s.db.QueryRow(r.Context(),
		`SELECT f.id, f.filename, f.size, f.hash, f.file_path, f.device_id, d.name,
		        f.status, f.storage_type, f.mime_type, f.page_count,
		        f.extracted_text, f.mtime, f.created_at, f.updated_at
		 FROM files f JOIN devices d ON f.device_id = d.id
		 WHERE f.id = $1 AND f.owner = $2`, id, username,
	).Scan(
		&f.ID, &f.Filename, &f.Size, &f.Hash, &f.FilePath, &f.DeviceID, &f.DeviceName,
		&f.Status, &f.StorageType, &f.MimeType, &f.PageCount,
		&f.ExtractedText, &f.Mtime, &f.CreatedAt, &f.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "file not found", http.StatusNotFound)
			return
		}
		log.Printf("get file: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	f.DownloadURL = s.renderDownloadURL(f.StorageType, f.FilePath)

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
	username := auth.UsernameFromContext(r.Context())

	rows, err := s.db.Query(r.Context(),
		`SELECT t.id, t.name, COUNT(DISTINCT ft.file_id) as file_count
		 FROM tags t
		 JOIN file_tags ft ON t.id = ft.tag_id
		 JOIN files f ON f.id = ft.file_id
		 WHERE f.owner = $1
		 GROUP BY t.id, t.name
		 ORDER BY file_count DESC`, username)
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
	username := auth.UsernameFromContext(r.Context())

	rows, err := s.db.Query(r.Context(),
		`SELECT DISTINCT d.id, d.name, d.created_at
		 FROM devices d
		 JOIN files f ON f.device_id = d.id
		 WHERE f.owner = $1
		 ORDER BY d.name`, username)
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

// renderDownloadURL returns a URL for downloading a file, derived from
// PresignURLTemplate. Only populated for s3-backed files when a template is set.
func (s *Server) renderDownloadURL(storageType, filePath string) string {
	if storageType != "s3" || s.presignURLTemplate == "" {
		return ""
	}
	return strings.ReplaceAll(s.presignURLTemplate, "{file_path}", url.QueryEscape(filePath))
}
