package models

import "time"

type SearchType string

const (
	TypeUsername SearchType = "username"
	TypeEmail    SearchType = "email"
)

type Site struct {
	Name       string            `json:"name"`
	URL        string            `json:"url"`
	SearchType SearchType        `json:"search_type"`
	ErrorType  string            `json:"error_type"`
	ErrorCode  int               `json:"error_code"`
	ErrorMsg   string            `json:"error_msg"`
	Weight     int               `json:"weight"`
	Headers    map[string]string `json:"headers"`
}

type Result struct {
	SiteName   string        `json:"site_name"`
	Target     string        `json:"target"`
	URL        string        `json:"url"`
	Found      bool          `json:"found"`
	Confidence int           `json:"confidence"`
	Duration   time.Duration `json:"duration"`
	Error      string        `json:"error,omitempty"`
}
