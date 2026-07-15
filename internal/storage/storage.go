package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/johan-larp/agentsearch/internal/models"
)

type JSONStreamer struct {
	file   *os.File
	mu     sync.Mutex
	first  bool
	closed bool
}

func NewJSONStreamer(filePath string) (*JSONStreamer, error) {
	f, err := os.Create(filePath)
	if err != nil {
		return nil, err
	}
	f.WriteString("[\n")
	return &JSONStreamer{file: f, first: true}, nil
}

func (s *JSONStreamer) Write(res models.Result) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("streamer is closed")
	}

	data, err := json.MarshalIndent(res, "  ", "  ")
	if err != nil {
		return err
	}

	if !s.first {
		s.file.WriteString(",\n")
	}
	s.file.Write(data)
	s.first = false
	return nil
}

func (s *JSONStreamer) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.file.WriteString("\n]")
	return s.file.Close()
}
