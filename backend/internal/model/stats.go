package model

type StatsResponse struct {
	Status        string         `json:"status"`
	TotalFiles    int            `json:"total_files"`
	FilesByStatus map[string]int `json:"files_by_status"`
}

type ActivityEntry struct {
	ID          string `json:"id"`
	Icon        string `json:"icon"`
	Description string `json:"description"`
	Timestamp   string `json:"timestamp"`
}

type ActivityResponse struct {
	Entries []ActivityEntry `json:"entries"`
	Total   int             `json:"total"`
}
