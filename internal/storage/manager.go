package storage

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/johan-larp/agentsearch/internal/models"
)

// Writer абстрагирует запись результатов в конкретный формат.
type Writer interface {
	Write(res models.Result) error
	Close() error
}

// Manager управляет одновременной записью в несколько форматов (json, csv, txt).
type Manager struct {
	writers []Writer
}

// NewManager создает writer'ы для каждого указанного формата.
func NewManager(dir string, formats []string, target string) (*Manager, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	var writers []Writer
	for _, f := range formats {
		path := filepath.Join(dir, fmt.Sprintf("%s.%s", sanitizeFilename(target), f))
		switch f {
		case "json":
			w, err := NewJSONWriter(path)
			if err != nil {
				return nil, err
			}
			writers = append(writers, w)
		case "csv":
			w, err := NewCSVWriter(path)
			if err != nil {
				return nil, err
			}
			writers = append(writers, w)
		case "txt":
			w, err := NewTXTWriter(path)
			if err != nil {
				return nil, err
			}
			writers = append(writers, w)
		default:
			slog.Warn("unknown output format, skipping", "format", f)
		}
	}
	return &Manager{writers: writers}, nil
}

// Write дублирует результат во все активные writer'ы.
func (m *Manager) Write(res models.Result) {
	for _, w := range m.writers {
		if err := w.Write(res); err != nil {
			slog.Error("write result failed", "error", err)
		}
	}
}

// Close корректно завершает все writer'ы (флашит буферы, закрывает файлы).
func (m *Manager) Close() {
	for _, w := range m.writers {
		if err := w.Close(); err != nil {
			slog.Error("close writer failed", "error", err)
		}
	}
}

// sanitizeFilename заменяет недопустимые символы в имени файла.
func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
		" ", "_",
	)
	return replacer.Replace(name)
}
