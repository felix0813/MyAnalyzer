package history

import "time"

type Record struct {
	ID                 int64     `json:"id"`
	URL                string    `json:"url"`
	Title              string    `json:"title"`
	Domain             string    `json:"domain"`
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
	RecentOnly bool     `json:"recentOnly"`
}
