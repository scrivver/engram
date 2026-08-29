package model

import "time"

type Device struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type File struct {
	ID            string    `json:"id"`
	Filename      string    `json:"filename"`
	Size          int64     `json:"size"`
	Hash          string    `json:"hash"`
	FilePath      string    `json:"file_path"`
	DeviceID      string    `json:"device_id"`
	DeviceName    string    `json:"device_name,omitempty"`
	Status        string    `json:"status"`
	StorageType   string    `json:"storage_type"`
	MimeType      *string   `json:"mime_type,omitempty"`
	PageCount     *int      `json:"page_count,omitempty"`
	ExtractedText *string   `json:"extracted_text,omitempty"`
	Mtime         time.Time `json:"mtime"`
	Tags          []string  `json:"tags,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	DownloadURL   string    `json:"download_url,omitempty"`
}

// Folder is a directory derived from the display paths of a user's files. It
// is computed per request and never stored. FileCount is recursive: every file
// beneath Path, at any depth, matching the active filters.
type Folder struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	FileCount int    `json:"file_count"`
}

type Tag struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	FileCount int    `json:"file_count,omitempty"`
}
