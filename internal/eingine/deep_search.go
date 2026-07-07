package engine

import (
	"fmt"
	"net/url"
)

type DeepSearchQuery struct {
	Engine string
	Query  string
	URL    string
}

// GenerateDeepQueries создает список ссылок для ручного или автоматического поиска через поисковики
func GenerateDeepQueries(target string) []DeepSearchQuery {
	queries := []string{
		fmt.Sprintf("site:instagram.com \"%s\"", target),
		fmt.Sprintf("site:twitter.com \"%s\"", target),
		fmt.Sprintf("site:github.com \"%s\"", target),
		fmt.Sprintf("site:facebook.com \"%s\"", target),
		fmt.Sprintf("site:reddit.com \"%s\"", target),
		fmt.Sprintf("site:linkedin.com/in/ \"%s\"", target),
		fmt.Sprintf("\"%s\" (username OR profile OR account)", target),
	}

	engines := []struct {
		name string
		base string
	}{
		{"Google", "https://www.google.com/search?q="},
		{"Bing", "https://www.bing.com/search?q="},
		{"DuckDuckGo", "https://duckduckgo.com/?q="},
	}

	var result []DeepSearchQuery
	for _, eng := range engines {
		for _, q := range queries {
			encodedQuery := url.QueryEscape(q)
			result = append(result, DeepSearchQuery{
				Engine: eng.name,
				Query:  q,
				URL:    eng.base + encodedQuery,
			})
		}
	}
	return result
}
