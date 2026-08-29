package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type fakeDB struct {
	rows pgx.Rows
	row  pgx.Row
	// queries, when set, records every SQL string the handler issues so tests
	// can assert on query assembly. Without it the SQL is discarded and only
	// scanning behavior is covered.
	queries *[]string
}

func (db fakeDB) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	if db.queries != nil {
		*db.queries = append(*db.queries, sql)
	}
	return db.rows, nil
}

func (db fakeDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return db.row
}

type fakeRows struct {
	values [][]any
	index  int
}

func (r *fakeRows) Close()                                       {}
func (r *fakeRows) Err() error                                   { return nil }
func (r *fakeRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *fakeRows) Values() ([]any, error)                       { return r.values[r.index-1], nil }
func (r *fakeRows) RawValues() [][]byte                          { return nil }
func (r *fakeRows) Conn() *pgx.Conn                              { return nil }

func (r *fakeRows) Next() bool {
	if r.index >= len(r.values) {
		return false
	}
	r.index++
	return true
}

func (r *fakeRows) Scan(dest ...any) error {
	return scanValues(r.values[r.index-1], dest...)
}

type fakeRow struct {
	values []any
}

func (r fakeRow) Scan(dest ...any) error {
	return scanValues(r.values, dest...)
}

func scanValues(values []any, dest ...any) error {
	for i, value := range values {
		switch d := dest[i].(type) {
		case *string:
			*d = value.(string)
		case *int64:
			*d = value.(int64)
		case *int:
			*d = value.(int)
		case **string:
			if value == nil {
				*d = nil
			} else {
				v := value.(string)
				*d = &v
			}
		case **int:
			if value == nil {
				*d = nil
			} else {
				v := value.(int)
				*d = &v
			}
		case *time.Time:
			*d = value.(time.Time)
		default:
			panic("unsupported scan destination")
		}
	}
	return nil
}

func TestListFilesReturnsDisplayFilenameAndStoragePath(t *testing.T) {
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	rows := &fakeRows{values: [][]any{{
		"file-id",
		"docs/myfile.pdf",
		int64(204800),
		"sha256:abcdef123456",
		"files/alice/2026/07/docs/myfile.pdf",
		"device-id",
		"reliquary",
		"ready",
		"s3",
		nil,
		nil,
		now,
		now,
		now,
	}}}
	server := NewServer(nil)
	server.db = fakeDB{rows: rows}

	req := httptest.NewRequest(http.MethodGet, "/api/files", nil)
	res := httptest.NewRecorder()

	server.handleListFiles(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	var got []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got[0]["filename"] != "docs/myfile.pdf" {
		t.Fatalf("filename=%q, want display path", got[0]["filename"])
	}
	if got[0]["file_path"] != "files/alice/2026/07/docs/myfile.pdf" {
		t.Fatalf("file_path=%q, want storage identity", got[0]["file_path"])
	}
}

func TestGetFileReturnsDisplayFilenameAndStoragePath(t *testing.T) {
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	row := fakeRow{values: []any{
		"file-id",
		"docs/myfile.pdf",
		int64(204800),
		"sha256:abcdef123456",
		"files/alice/2026/07/docs/myfile.pdf",
		"device-id",
		"reliquary",
		"ready",
		"s3",
		nil,
		nil,
		nil,
		now,
		now,
		now,
	}}
	server := NewServer(nil)
	server.db = fakeDB{rows: &fakeRows{}, row: row}

	req := httptest.NewRequest(http.MethodGet, "/api/files/file-id", nil)
	req.SetPathValue("id", "file-id")
	res := httptest.NewRecorder()

	server.handleGetFile(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	var got map[string]any
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got["filename"] != "docs/myfile.pdf" {
		t.Fatalf("filename=%q, want display path", got["filename"])
	}
	if got["file_path"] != "files/alice/2026/07/docs/myfile.pdf" {
		t.Fatalf("file_path=%q, want storage identity", got["file_path"])
	}
}

func TestNormalizeDisplayPath(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"plain segment", "docs", "docs"},
		{"nested", "docs/notes", "docs/notes"},
		{"leading slash", "/docs/notes", "docs/notes"},
		{"trailing slash", "docs/notes/", "docs/notes"},
		{"duplicate separators", "docs//notes", "docs/notes"},
		{"backslash separators", `docs\notes`, "docs/notes"},
		{"dot segments", "docs/./notes", "docs/notes"},
		{"parent segments", "docs/../../notes", "docs/notes"},
		{"only traversal", "../..", ""},
		{"surrounding space", " docs / notes ", "docs/notes"},
		{"preserves inner space", "my docs/notes", "my docs/notes"},
		{"preserves underscore", "my_docs", "my_docs"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeDisplayPath(tc.in); got != tc.want {
				t.Fatalf("normalizeDisplayPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestEscapeLikePattern(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "docs", "docs"},
		{"underscore", "my_docs", `my\_docs`},
		{"percent", "100%", `100\%`},
		{"backslash", `a\b`, `a\\b`},
		{"combined", `a_b%c\d`, `a\_b\%c\\d`},
		{"path separator untouched", "docs/notes", "docs/notes"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := escapeLikePattern(tc.in); got != tc.want {
				t.Fatalf("escapeLikePattern(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestBuildFileFiltersDefaults(t *testing.T) {
	filters, err := buildFileFilters(url.Values{}, "alice")
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"f.status = $1", "f.owner = $2"}
	if !reflect.DeepEqual(filters.conditions, want) {
		t.Fatalf("conditions = %v, want %v", filters.conditions, want)
	}
	if !reflect.DeepEqual(filters.args, []any{"ready", "alice"}) {
		t.Fatalf("args = %v, want [ready alice]", filters.args)
	}
	if filters.joins != "" {
		t.Fatalf("joins = %q, want empty", filters.joins)
	}
}

// Argument order is load-bearing: callers append LIMIT and OFFSET after these,
// so a reordering here would silently misbind every query.
func TestBuildFileFiltersArgumentOrder(t *testing.T) {
	q := url.Values{
		"status": {"processing"},
		"device": {"reliquary"},
		"q":      {"invoice"},
		"type":   {"pdf"},
		"from":   {"2026-06-01"},
		"to":     {"2026-07-01"},
		"tag":    {"work", "urgent"},
	}

	filters, err := buildFileFilters(q, "alice")
	if err != nil {
		t.Fatal(err)
	}

	wantConditions := []string{
		"f.status = $1",
		"f.owner = $2",
		"d.name = $3",
		"f.tsv @@ websearch_to_tsquery('simple', $4)",
		fileTypeFilter["pdf"],
		"f.mtime >= $5",
		"f.mtime <= $6",
		"t.name IN ($7,$8)",
	}
	if !reflect.DeepEqual(filters.conditions, wantConditions) {
		t.Fatalf("conditions =\n%v\nwant\n%v", filters.conditions, wantConditions)
	}

	if len(filters.args) != 8 {
		t.Fatalf("len(args) = %d, want 8", len(filters.args))
	}
	for i, want := range []any{"processing", "alice", "reliquary", "invoice"} {
		if filters.args[i] != want {
			t.Fatalf("args[%d] = %v, want %v", i, filters.args[i], want)
		}
	}
	if filters.args[6] != "work" || filters.args[7] != "urgent" {
		t.Fatalf("tag args = %v, %v, want work, urgent", filters.args[6], filters.args[7])
	}
	if filters.joins == "" {
		t.Fatal("joins should include the tag join when tags are present")
	}
}

// The type filter is a literal SQL fragment and must not consume a placeholder,
// otherwise every later condition binds to the wrong argument.
func TestBuildFileFiltersTypeConsumesNoArgument(t *testing.T) {
	filters, err := buildFileFilters(url.Values{"type": {"image"}, "device": {"reliquary"}}, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(filters.args) != 3 {
		t.Fatalf("len(args) = %d, want 3", len(filters.args))
	}
}

func TestBuildFileFiltersRejectsBadInput(t *testing.T) {
	cases := []struct {
		name string
		q    url.Values
		want string
	}{
		{"unknown type", url.Values{"type": {"spreadsheet"}}, "invalid type"},
		{"bad from date", url.Values{"from": {"yesterday"}}, "invalid 'from' date"},
		{"bad to date", url.Values{"to": {"31-12-2026"}}, "invalid 'to' date"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := buildFileFilters(tc.q, "alice")
			if err == nil || err.Error() != tc.want {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

// Regression: type is validated before sort, so a request carrying both faults
// reports the type, as it did before the filter builder was extracted.
func TestListFilesReportsInvalidTypeBeforeInvalidSort(t *testing.T) {
	server := NewServer(nil)
	server.db = fakeDB{rows: &fakeRows{}}

	req := httptest.NewRequest(http.MethodGet, "/api/files?type=nope&sort=nope", nil)
	res := httptest.NewRecorder()

	server.handleListFiles(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", res.Code)
	}
	if got := strings.TrimSpace(res.Body.String()); got != "invalid type" {
		t.Fatalf("body = %q, want %q", got, "invalid type")
	}
}

// Pins the assembled query for the file list: the tag join sits between FROM and
// WHERE, and LIMIT/OFFSET take the two placeholders after the filter arguments.
func TestListFilesAssemblesQuery(t *testing.T) {
	var queries []string
	server := NewServer(nil)
	server.db = fakeDB{rows: &fakeRows{}, queries: &queries}

	req := httptest.NewRequest(http.MethodGet, "/api/files?tag=work&device=reliquary&sort=size_asc", nil)
	server.handleListFiles(httptest.NewRecorder(), req)

	if len(queries) != 1 {
		t.Fatalf("issued %d queries, want 1", len(queries))
	}
	got := queries[0]

	for _, want := range []string{
		"JOIN devices d ON f.device_id = d.id JOIN file_tags ft ON f.id = ft.file_id",
		"WHERE f.status = $1 AND f.owner = $2 AND d.name = $3 AND t.name IN ($4)",
		"ORDER BY f.size ASC",
		"LIMIT $5 OFFSET $6",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("query missing %q:\n%s", want, got)
		}
	}
	if strings.Index(got, "JOIN file_tags") > strings.Index(got, "WHERE") {
		t.Fatalf("tag join must precede WHERE:\n%s", got)
	}
}

func TestListFilesRejectsUnknownScope(t *testing.T) {
	server := NewServer(nil)
	server.db = fakeDB{rows: &fakeRows{}}

	req := httptest.NewRequest(http.MethodGet, "/api/files?scope=sideways", nil)
	res := httptest.NewRecorder()
	server.handleListFiles(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", res.Code)
	}
	if got := strings.TrimSpace(res.Body.String()); got != "invalid scope" {
		t.Fatalf("body = %q, want %q", got, "invalid scope")
	}
}

func TestListFilesFolderScopeRestrictsToDirectChildren(t *testing.T) {
	var queries []string
	server := NewServer(nil)
	server.db = fakeDB{rows: &fakeRows{}, queries: &queries}

	req := httptest.NewRequest(http.MethodGet, "/api/files?scope=folder&path=docs", nil)
	server.handleListFiles(httptest.NewRecorder(), req)

	got := queries[0]
	for _, want := range []string{
		`f.filename LIKE $4 || '%' ESCAPE '\'`,
		"position('/' in substring(f.filename from length($3) + 1)) = 0",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("query missing %q:\n%s", want, got)
		}
	}
}

// At the root the prefix is empty, so there is nothing to match with LIKE — only
// the "no separator at all" test remains. Binding an empty LIKE pattern would be
// dead weight on every unscoped root listing.
func TestListFilesFolderScopeAtRootOmitsLike(t *testing.T) {
	var queries []string
	server := NewServer(nil)
	server.db = fakeDB{rows: &fakeRows{}, queries: &queries}

	req := httptest.NewRequest(http.MethodGet, "/api/files?scope=folder", nil)
	server.handleListFiles(httptest.NewRecorder(), req)

	got := queries[0]
	if strings.Contains(got, "LIKE") && strings.Contains(got, "ESCAPE") {
		t.Fatalf("root scope should not emit a LIKE prefix match:\n%s", got)
	}
	if !strings.Contains(got, "position('/' in substring(f.filename from length($3) + 1)) = 0") {
		t.Fatalf("query missing root direct-child predicate:\n%s", got)
	}
}

// A folder named with an underscore must match literally: unescaped, `my_docs`
// would also match `myXdocs`. The raw prefix is bound first for substring/length
// (character offsets, where escaping would corrupt the count), the escaped one
// second for LIKE.
func TestFolderScopeEscapesWildcardsInPrefixOnly(t *testing.T) {
	filters, err := buildFileFilters(url.Values{}, "alice")
	if err != nil {
		t.Fatal(err)
	}
	filters.scopeToFolder("my_docs")

	if got := filters.args[2]; got != "my_docs/" {
		t.Fatalf("raw prefix = %v, want %v", got, "my_docs/")
	}
	if got := filters.args[3]; got != `my\_docs/` {
		t.Fatalf("escaped prefix = %v, want %v", got, `my\_docs/`)
	}
}

func TestListFoldersResponseShape(t *testing.T) {
	rows := &fakeRows{values: [][]any{
		{"docs", 12},
		{"invoices", 3},
	}}
	server := NewServer(nil)
	server.db = fakeDB{rows: rows}

	req := httptest.NewRequest(http.MethodGet, "/api/folders", nil)
	res := httptest.NewRecorder()
	server.handleListFolders(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	var got []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d folders, want 2", len(got))
	}
	if got[0]["name"] != "docs" || got[0]["path"] != "docs" || got[0]["file_count"] != float64(12) {
		t.Fatalf("folder[0] = %v", got[0])
	}
}

// Nested folders must report a path the client can navigate to, not a bare name.
func TestListFoldersQualifiesNestedPaths(t *testing.T) {
	rows := &fakeRows{values: [][]any{{"notes", 4}}}
	server := NewServer(nil)
	server.db = fakeDB{rows: rows}

	req := httptest.NewRequest(http.MethodGet, "/api/folders?path=docs", nil)
	res := httptest.NewRecorder()
	server.handleListFolders(res, req)

	var got []map[string]any
	json.NewDecoder(res.Body).Decode(&got)
	if got[0]["path"] != "docs/notes" {
		t.Fatalf("path = %v, want docs/notes", got[0]["path"])
	}
}

func TestListFoldersEmptyDirectoryReturnsEmptyArray(t *testing.T) {
	server := NewServer(nil)
	server.db = fakeDB{rows: &fakeRows{}}

	req := httptest.NewRequest(http.MethodGet, "/api/folders?path=docs", nil)
	res := httptest.NewRecorder()
	server.handleListFolders(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.Code)
	}
	if got := strings.TrimSpace(res.Body.String()); got != "[]" {
		t.Fatalf("body = %q, want []", got)
	}
}

func TestListFoldersRejectsBadFilters(t *testing.T) {
	server := NewServer(nil)
	server.db = fakeDB{rows: &fakeRows{}}

	req := httptest.NewRequest(http.MethodGet, "/api/folders?type=spreadsheet", nil)
	res := httptest.NewRecorder()
	server.handleListFolders(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", res.Code)
	}
}

// The folder query and the file query must apply the same predicates, or a
// folder count will disagree with the files the folder actually shows.
func TestListFoldersSharesFiltersWithFileList(t *testing.T) {
	var fileQueries, folderQueries []string
	target := "?type=pdf&tag=work&device=reliquary&path=docs"

	files := NewServer(nil)
	files.db = fakeDB{rows: &fakeRows{}, queries: &fileQueries}
	files.handleListFiles(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/api/files"+target+"&scope=folder", nil))

	folders := NewServer(nil)
	folders.db = fakeDB{rows: &fakeRows{}, queries: &folderQueries}
	folders.handleListFolders(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/api/folders"+target, nil))

	for _, shared := range []string{
		fileTypeFilter["pdf"],
		"t.name IN ($4)",
		"d.name = $3",
		"JOIN file_tags ft ON f.id = ft.file_id",
	} {
		if !strings.Contains(fileQueries[0], shared) {
			t.Fatalf("file query missing %q:\n%s", shared, fileQueries[0])
		}
		if !strings.Contains(folderQueries[0], shared) {
			t.Fatalf("folder query missing %q:\n%s", shared, folderQueries[0])
		}
	}
}

// COUNT(*) would count a file once per matching tag when the tag filter joins
// file_tags, inflating every folder count.
func TestListFoldersCountsFilesNotJoinRows(t *testing.T) {
	var queries []string
	server := NewServer(nil)
	server.db = fakeDB{rows: &fakeRows{}, queries: &queries}

	server.handleListFolders(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/api/folders?tag=a&tag=b", nil))

	if !strings.Contains(queries[0], "COUNT(DISTINCT sub.id)") {
		t.Fatalf("folder count must be COUNT(DISTINCT sub.id):\n%s", queries[0])
	}
}
