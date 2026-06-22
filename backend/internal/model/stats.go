package model

type StatsResponse struct {
	Status        string         `json:"status"`
	EfficiencyPct int            `json:"efficiency_pct"`
	ActiveProcess string         `json:"active_process"`
	SyncFrequency string         `json:"sync_frequency"`
	LatencyMs     int            `json:"latency_ms"`
	SyncSpeedMbps float64        `json:"sync_speed_mbps"`
	UptimePct     float64        `json:"uptime_pct"`
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
