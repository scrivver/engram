package model

import "time"

type Device struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type File struct {
	ID                string     `json:"id"`
	Filename          string     `json:"filename"`
	Size              int64      `json:"size"`
	Hash              string     `json:"hash"`
	OriginalPath      string     `json:"original_path"`
	DeviceID          string     `json:"device_id"`
	DeviceName        string     `json:"device_name,omitempty"`
	Status            string     `json:"status"`
	ObjectKey         string     `json:"object_key,omitempty"`
	StorageBucket     string     `json:"storage_bucket,omitempty"`
	MimeType          *string    `json:"mime_type,omitempty"`
	PageCount         *int       `json:"page_count,omitempty"`
	ExtractedText     *string    `json:"extracted_text,omitempty"`
	Mtime             time.Time  `json:"mtime"`
	ArchivalTimestamp *time.Time `json:"archival_timestamp,omitempty"`
	Tags              []string   `json:"tags,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type Tag struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	FileCount int    `json:"file_count,omitempty"`
}

type IngestJob struct {
	FileID        string `json:"file_id"`
	ObjectKey     string `json:"object_key"`
	StorageBucket string `json:"storage_bucket"`
	Filename      string `json:"filename"`
	Size          int64  `json:"size"`
	Hash          string `json:"hash"`
}

type UploadMeta struct {
	Filename   string    `json:"filename"`
	Size       int64     `json:"size"`
	Path       string    `json:"path"`
	DeviceName string    `json:"device_name"`
	Hash       string    `json:"hash"`
	Mtime      time.Time `json:"mtime"`
}
