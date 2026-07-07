package models

import "time"

type SearchType string

const (
	TypeUsername SearchType = "username"
	TypeEmail    SearchType = "email"
)

// Site определяет параметры проверки конкретного ресурса
type Site struct {
	Name        string            `json:"name"`
	URL         string            `json:"url"`          // Шаблон URL, например "https://example.com/{target}"
	SearchType  SearchType        `json:"search_type"`   // username или email
	ErrorType   string            `json:"error_type"`    // "status_code", "message", "redirect"
	ErrorCode   int               `json:"error_code"`     // Код, который означает "не найден"
	ErrorMsg    string            `json:"error_msg"`      // Строка, которая означает "не найден"
	Weight      int               `json:"weight"`         // Вес сайта (для системы уверенности)
	Headers     map[string]string `json:"headers"`        // Дополнительные заголовки
}

// Result содержит итог проверки одного сайта
type Result struct {
	SiteName   string        `json:"site_name"`
	Target     string        `json:"target"`
	URL        string        `json:"url"`
	Found      bool          `json:"found"`
	Confidence int           `json:"confidence"` // 0-100%
	Duration   time.Duration `json:"duration"`
	Error      string        `json:"error,omitempty"`
}
