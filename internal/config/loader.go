package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/johan-larp/agentsearch/internal/models"
	"gopkg.in/yaml.v3"
)

// LoadSites загружает базу сайтов из YAML или JSON.
// Поддерживает три формата:
//  1. Прямой массив []models.SiteConfig
//  2. Объект с полем sites: { "sites": { "Name": { ... } } } — совместимость с Maigret
//  3. Объект с полем sites как массив
func LoadSites(path string) ([]models.SiteConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read sites file: %w", err)
	}

	// Попытка 1: прямой массив
	var direct []models.SiteConfig
	if err := yaml.Unmarshal(data, &direct); err == nil && len(direct) > 0 {
		// Проверим, что первый элемент имеет URL (защита от пустой десериализации)
		if direct[0].URL != "" || direct[0].Name != "" {
			return filterEnabled(direct), nil
		}
	}

	// Попытка 2: объект { sites: map[string]SiteConfig }
	var wrappedMap struct {
		Sites map[string]models.SiteConfig `json:"sites" yaml:"sites"`
	}
	if err := json.Unmarshal(data, &wrappedMap); err == nil && len(wrappedMap.Sites) > 0 {
		return convertMapToSlice(wrappedMap.Sites), nil
	}
	if err := yaml.Unmarshal(data, &wrappedMap); err == nil && len(wrappedMap.Sites) > 0 {
		return convertMapToSlice(wrappedMap.Sites), nil
	}

	// Попытка 3: объект { sites: []SiteConfig }
	var wrappedSlice struct {
		Sites []models.SiteConfig `json:"sites" yaml:"sites"`
	}
	if err := json.Unmarshal(data, &wrappedSlice); err == nil && len(wrappedSlice.Sites) > 0 {
		return filterEnabled(wrappedSlice.Sites), nil
	}
	if err := yaml.Unmarshal(data, &wrappedSlice); err == nil && len(wrappedSlice.Sites) > 0 {
		return filterEnabled(wrappedSlice.Sites), nil
	}

	return nil, fmt.Errorf("unsupported sites file format: %s", path)
}

func filterEnabled(sites []models.SiteConfig) []models.SiteConfig {
	out := make([]models.SiteConfig, 0, len(sites))
	for _, s := range sites {
		if !s.Disabled {
			out = append(out, s)
		}
	}
	return out
}

func convertMapToSlice(m map[string]models.SiteConfig) []models.SiteConfig {
	out := make([]models.SiteConfig, 0, len(m))
	for name, s := range m {
		if s.Disabled {
			continue
		}
		if s.Name == "" {
			s.Name = name
		}
		// Миграция устаревших полей Maigret → наш формат
		if s.CheckType == "" {
			switch {
			case s.ErrorCode != 0:
				s.CheckType = "status_code"
			case len(s.AbsenceStrs) > 0 || len(s.AbsenceRegexes) > 0:
				s.CheckType = "message"
			case s.ErrorURL != "":
				s.CheckType = "response_url"
			}
		}
		if s.Weight == 0 {
			s.Weight = 10
		}
		out = append(out, s)
	}
	return out
}
