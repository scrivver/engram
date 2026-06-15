package publisher

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFileEventMatchesCanonicalFixture(t *testing.T) {
	fixturePath := filepath.Join("..", "..", "..", "contracts", "file-events", "create.json")
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var want map[string]any
	if err := json.Unmarshal(data, &want); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}

	event := FileEvent{
		Event:       "create",
		FilePath:    "files/alice/2026/06/report.pdf",
		Filename:    "report.pdf",
		Size:        204800,
		Hash:        "sha256:abcdef123456",
		Mtime:       "2026-06-15T12:00:00Z",
		DeviceName:  "reliquary",
		StorageType: "s3",
	}
	data, err = json.Marshal(event)
	if err != nil {
		t.Fatalf("encode event: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decode event: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("event does not match canonical fixture:\ngot:  %#v\nwant: %#v", got, want)
	}
}
