package api

import (
	"encoding/json"
	"errors"
	"fmt"
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

// badRequestError is a client-supplied-parameter error whose message is safe to
// return to the caller verbatim.
type badRequestError struct{ msg string }

func (e badRequestError) Error() string { return e.msg }

// fileFilters is the shared WHERE-clause construction for the file list and the
// folder list. Both queries build their predicates here so folder counts can
// never disagree with the files a folder actually contains.
type fileFilters struct {
	joins      string
	conditions []string
	args       []any
}

// placeholder records arg and returns its positional placeholder.
func (f *fileFilters) placeholder(arg any) string {
	f.args = append(f.args, arg)
	return "$" + strconv.Itoa(len(f.args))
}

// where appends a condition, substituting %s in format with the placeholder for arg.
func (f *fileFilters) where(format string, arg any) {
	f.conditions = append(f.conditions, fmt.Sprintf(format, f.placeholder(arg)))
}

// buildFileFilters translates the shared query parameters into SQL conditions
// and positional arguments. Argument order is significant: callers append their
// own trailing arguments (LIMIT, OFFSET) after these.
func buildFileFilters(q url.Values, username string) (fileFilters, error) {
	var f fileFilters

	status := q.Get("status")
	if status == "" {
		status = "ready"
	}

	fileType := q.Get("type")
	if fileType != "" {
		if _, ok := fileTypeFilter[fileType]; !ok {
			return fileFilters{}, badRequestError{"invalid type"}
		}
	}

	f.where("f.status = %s", status)
	f.where("f.owner = %s", username)

	if device := q.Get("device"); device != "" {
		f.where("d.name = %s", device)
	}

	if search := q.Get("q"); search != "" {
		// Two predicates, OR'd, because neither covers the search box alone.
		//
		// Full-text match against the generated tsv column (see migration 006,
		// which is what makes the words inside a filename addressable at all).
		// websearch_to_tsquery supports Google-style syntax: quoted phrases,
		// `-term` exclusions, implicit AND across terms.
		//
		// A tsquery only matches whole lexemes, though, and the client filters
		// as the user types — so `presenta` would find nothing on its way to
		// `presentation`. The substring match on the filename covers the
		// half-typed term (and any infix, which a prefix query would miss);
		// migration 006's trigram index keeps it off a sequential scan.
		fts := f.placeholder(search)
		like := f.placeholder(strings.ToLower(escapeLikePattern(search)))
		f.conditions = append(f.conditions, fmt.Sprintf(
			"(f.tsv @@ websearch_to_tsquery('simple', %s)"+
				" OR lower(f.filename) LIKE '%%' || %s || '%%' ESCAPE '\\')",
			fts, like))
	}

	if fileType != "" {
		f.conditions = append(f.conditions, fileTypeFilter[fileType])
	}

	if fromStr := q.Get("from"); fromStr != "" {
		t, ok := parseDate(fromStr)
		if !ok {
			return fileFilters{}, badRequestError{"invalid 'from' date"}
		}
		f.where("f.mtime >= %s", t)
	}

	if toStr := q.Get("to"); toStr != "" {
		t, ok := parseDate(toStr)
		if !ok {
			return fileFilters{}, badRequestError{"invalid 'to' date"}
		}
		f.where("f.mtime <= %s", t)
	}

	if tags := q["tag"]; len(tags) > 0 {
		f.joins = " JOIN file_tags ft ON f.id = ft.file_id JOIN tags t ON ft.tag_id = t.id"
		placeholders := make([]string, len(tags))
		for i, tag := range tags {
			placeholders[i] = f.placeholder(tag)
		}
		f.conditions = append(f.conditions, "t.name IN ("+strings.Join(placeholders, ",")+")")
	}

	return f, nil
}

// folderPrefix turns a normalized display path into the prefix its children
// share. The root is the empty prefix, which every display path starts with.
func folderPrefix(path string) string {
	if path == "" {
		return ""
	}
	return path + "/"
}

// bindPrefix records prefix as a positional argument and, when it is not the
// root, constrains filename to it. The returned placeholder holds the raw
// (unescaped) prefix so callers can reuse it inside substring/length, where
// character offsets matter and LIKE escaping would corrupt them.
func (f *fileFilters) bindPrefix(prefix string) string {
	raw := f.placeholder(prefix)
	if prefix != "" {
		f.conditions = append(f.conditions,
			fmt.Sprintf("f.filename LIKE %s || '%%' ESCAPE '\\'", f.placeholder(escapeLikePattern(prefix))))
	}
	return raw
}

// scopeToFolder restricts to files sitting directly in path: the prefix
// matches and what follows holds no further separator.
func (f *fileFilters) scopeToFolder(path string) {
	raw := f.bindPrefix(folderPrefix(path))
	f.conditions = append(f.conditions,
		fmt.Sprintf("position('/' in substring(f.filename from length(%s) + 1)) = 0", raw))
}

// scopeToDescendants restricts to files nested anywhere below path, and returns
// the placeholder holding the raw prefix so the caller can project the next
// path segment from it.
func (f *fileFilters) scopeToDescendants(path string) string {
	raw := f.bindPrefix(folderPrefix(path))
	f.conditions = append(f.conditions,
		fmt.Sprintf("position('/' in substring(f.filename from length(%s) + 1)) > 0", raw))
	return raw
}

// normalizeDisplayPath cleans a user-supplied display path. It mirrors
// GalleryFolderPath._normalizePath in the Flutter client, so both sides agree
// on what a folder path means: backslashes become separators, and empty, `.`,
// and `..` segments are dropped rather than rejected.
func normalizeDisplayPath(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	parts := strings.Split(value, "/")
	kept := parts[:0]
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." || part == ".." {
			continue
		}
		kept = append(kept, part)
	}
	return strings.Join(kept, "/")
}

// escapeLikePattern escapes the LIKE metacharacters so a folder named
// `my_docs` matches literally instead of behaving as a wildcard. Callers must
// pair this with an explicit ESCAPE clause.
func escapeLikePattern(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r == '\\' || r == '%' || r == '_' {
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	username := auth.UsernameFromContext(r.Context())

	q := r.URL.Query()

	// Filter validation runs before sort validation, matching the original
	// precedence: a request carrying both an invalid type and an invalid sort
	// still reports the type.
	filters, err := buildFileFilters(q, username)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	scope := q.Get("scope")
	if scope == "" {
		scope = "all"
	}
	if scope != "all" && scope != "folder" {
		http.Error(w, "invalid scope", http.StatusBadRequest)
		return
	}
	if scope == "folder" {
		filters.scopeToFolder(normalizeDisplayPath(q.Get("path")))
	}

	sortKey := q.Get("sort")
	if sortKey == "" {
		sortKey = "created_desc"
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

	args := filters.args
	query += filters.joins
	query += " WHERE " + strings.Join(filters.conditions, " AND ")
	query += " ORDER BY " + orderBy
	query += " LIMIT $" + strconv.Itoa(len(args)+1)
	args = append(args, limit)
	query += " OFFSET $" + strconv.Itoa(len(args)+1)
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

// handleListFolders returns every immediate subfolder of `path` for the owner.
// The response is complete rather than paginated: a directory level holds far
// fewer folders than files, and a partial folder list is the defect this
// endpoint exists to remove.
func (s *Server) handleListFolders(w http.ResponseWriter, r *http.Request) {
	username := auth.UsernameFromContext(r.Context())
	q := r.URL.Query()

	filters, err := buildFileFilters(q, username)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	path := normalizeDisplayPath(q.Get("path"))
	prefix := filters.scopeToDescendants(path)

	// The segment projection happens in a subquery so ORDER BY can name it
	// unambiguously: `name` also exists on the joined devices table, and
	// ordering by that silently instead would be near-invisible.
	// COUNT(DISTINCT id) rather than COUNT(*) because the tag filter joins
	// file_tags and would otherwise count a file once per matching tag.
	query := `
		SELECT sub.name, COUNT(DISTINCT sub.id) AS file_count
		FROM (
			SELECT split_part(substring(f.filename from length(` + prefix + `) + 1), '/', 1) AS name,
			       f.id
			FROM files f
			JOIN devices d ON f.device_id = d.id` + filters.joins + `
			WHERE ` + strings.Join(filters.conditions, " AND ") + `
		) sub
		GROUP BY sub.name
		ORDER BY lower(sub.name)`

	rows, err := s.db.Query(r.Context(), query, filters.args...)
	if err != nil {
		log.Printf("list folders: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	folders := make([]model.Folder, 0)
	for rows.Next() {
		var f model.Folder
		if err := rows.Scan(&f.Name, &f.FileCount); err != nil {
			log.Printf("scan folder: %v", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		f.Path = f.Name
		if path != "" {
			f.Path = path + "/" + f.Name
		}
		folders = append(folders, f)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(folders)
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
