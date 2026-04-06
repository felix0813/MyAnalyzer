package history

import "time"

type Record struct {
	ID                 int64     `json:"id"`
	URL                string    `json:"url"`
	Title              string    `json:"title"`
	Domain             string    `json:"domain"`
	RootURL            string    `json:"rootURL"`
	VisitedAt          time.Time `json:"visitedAt"`
	Notes              string    `json:"notes,omitempty"`
	VisitCount         int       `json:"visitCount"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
	DisplayTitle       string    `json:"displayTitle"`
	DisplayVisitedAt   string    `json:"displayVisitedAt"`
	DisplayVisitedDate string    `json:"displayVisitedDate"`
	DisplayVisitedTime string    `json:"displayVisitedTime"`
}

type recordPayload struct {
	URL        string `json:"url"`
	Title      string `json:"title"`
	VisitedAt  string `json:"visitedAt"`
	Notes      string `json:"notes"`
	VisitCount int    `json:"visitCount"`
}

type batchPayload struct {
	Source      string          `json:"source"`
	SentAt      string          `json:"sentAt"`
	RecordCount int             `json:"recordCount"`
	Records     []recordPayload `json:"records"`
}

type listResponse struct {
	Items      []Record `json:"items"`
	Total      int      `json:"total"`
	Limit      int      `json:"limit"`
	Offset     int      `json:"offset"`
	Search     string   `json:"search"`
	StartTime  string   `json:"startTime,omitempty"`
	EndTime    string   `json:"endTime,omitempty"`
	RecentOnly bool     `json:"recentOnly"`
}

type RootURLStat struct {
	RootURL              string    `json:"rootURL"`
	Domain               string    `json:"domain"`
	RecordCount          int       `json:"recordCount"`
	VisitCountTotal      int       `json:"visitCountTotal"`
	LastVisitedAt        time.Time `json:"lastVisitedAt"`
	DisplayLastVisitedAt string    `json:"displayLastVisitedAt"`
	LatestTitle          string    `json:"latestTitle"`
	LatestURL            string    `json:"latestURL"`
}

type rootURLStatsResponse struct {
	Items []RootURLStat `json:"items"`
	Days  int           `json:"days"`
	Limit int           `json:"limit"`
}
