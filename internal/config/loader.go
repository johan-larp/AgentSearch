package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/johan-larp/agentsearch/internal/models"
	"gopkg.in/yaml.v3"
)

// LoadSites загружает базу сайтов из YAML или JSON.
// Поддерживает форматы:
//  1. Прямой массив []models.SiteConfig
//  2. Maigret: { "sites": { "Name": { ... } } }
//  3. Maigret: { "sites": [ { ... } ] }
//  4. Sherlock: { "Name": { ... } }  (без обёртки sites)
func LoadSites(path string) ([]models.SiteConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read sites file: %w", err)
	}

	// Попытка 1: прямой массив YAML/JSON
	var direct []models.SiteConfig
	if err := yaml.Unmarshal(data, &direct); err == nil && len(direct) > 0 && (direct[0].URL != "" || direct[0].Name != "") {
		return filterEnabled(direct), nil
	}
	if err := json.Unmarshal(data, &direct); err == nil && len(direct) > 0 && (direct[0].URL != "" || direct[0].Name != "") {
		return filterEnabled(direct), nil
	}

	// Попытка 2: объект — определяем, есть ли поле sites
	var hasSites struct {
		Sites json.RawMessage `json:"sites" yaml:"sites"`
	}
	isJSON := json.Unmarshal(data, &hasSites) == nil && len(hasSites.Sites) > 0
	isYAML := yaml.Unmarshal(data, &hasSites) == nil && len(hasSites.Sites) > 0

	if isJSON || isYAML {
		// Maigret-формат: sites может быть map или array
		var mapWrap struct {
			Sites map[string]models.SiteConfig `json:"sites" yaml:"sites"`
		}
		var arrWrap struct {
			Sites []models.SiteConfig `json:"sites" yaml:"sites"`
		}

		if json.Unmarshal(data, &mapWrap) == nil && len(mapWrap.Sites) > 0 {
			return convertMapToSlice(mapWrap.Sites), nil
		}
		if yaml.Unmarshal(data, &mapWrap) == nil && len(mapWrap.Sites) > 0 {
			return convertMapToSlice(mapWrap.Sites), nil
		}
		if json.Unmarshal(data, &arrWrap) == nil && len(arrWrap.Sites) > 0 {
			return filterEnabled(arrWrap.Sites), nil
		}
		if yaml.Unmarshal(data, &arrWrap) == nil && len(arrWrap.Sites) > 0 {
			return filterEnabled(arrWrap.Sites), nil
		}
	}

	// Попытка 3: Sherlock-формат (плоский объект, ключ = имя сайта)
	var sherlockMap map[string]models.SiteConfig
	if json.Unmarshal(data, &sherlockMap) == nil && len(sherlockMap) > 0 {
		return convertMapToSlice(sherlockMap), nil
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
