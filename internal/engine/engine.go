package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/johan-larp/agentsearch/internal/models"
	"github.com/johan-larp/agentsearch/internal/network"
	"github.com/johan-larp/agentsearch/internal/storage"
)

type Engine struct {
	Client     *http.Client
	Sites      []models.Site
	Target     string
	Workers    int
	Streamer   *storage.JSONStreamer
}

func NewEngine(sites []models.Site, target string, workers int, streamer *storage.JSONStreamer, client *http.Client) *Engine {
	return &Engine{
		Client:   client,
		Sites:    sites,
		Target:   target,
		Workers:  workers,
		Streamer: streamer,
	}
}

func (e *Engine) Run(ctx context.Context, progress chan<- models.Result) {
	jobs := make(chan models.Site, len(e.Sites))
	results := make(chan models.Result, len(e.Sites))
	var wg sync.WaitGroup

	for i := 0; i < e.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for site := range jobs {
				results <- e.checkSite(ctx, site)
			}
		}()
	}

	go func() {
		for _, s := range e.Sites {
			jobs <- s
		}
		close(jobs)
	}()

	go func() {
		for res := range results {
			e.Streamer.Write(res)
			progress <- res
		}
	}()

	wg.Wait()
	close(results)
}

func (e *Engine) checkSite(ctx context.Context, site models.Site) models.Result {
	start := time.Now()
	targetURL := strings.ReplaceAll(site.URL, "{target}", e.Target)
	targetURL = strings.ReplaceAll(targetURL, "{username}", e.Target)
	res := models.Result{SiteName: site.Name, Target: e.Target, URL: targetURL}

	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		res.Error = err.Error()
		return res
	}

	req.Header.Set("User-Agent", network.GetRandomUA())
	for k, v := range site.Headers {
		req.Header.Set(k, v)
	}

	resp, err := e.Client.Do(req)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	body := string(bodyBytes)

	found, confidence := e.calculateConfidence(site, resp.StatusCode, body)
	res.Found = found
	res.Confidence = confidence
	res.Duration = time.Since(start)

	return res
}

func (e *Engine) calculateConfidence(site models.Site, statusCode int, body string) (bool, int) {
	score := 0
	switch site.ErrorType {
	case "status_code":
		if statusCode == site.ErrorCode { return false, 0 }
		if statusCode == 200 { score += 40 }
	case "message":
		if site.ErrorMsg != "" && strings.Contains(body, site.ErrorMsg) { return false, 0 }
		score += 40
	}
	if strings.Contains(strings.ToLower(body), strings.ToLower(e.Target)) {
		score += 30
	}
	score += site.Weight
	if score > 100 { score = 100 }
	return score >= 50, score
}

func SmartLoadSites(path string) ([]models.Site, error) {
	data, err := os.ReadFile(path)
	if err != nil { return nil, err }

	var sites []models.Site
	if err := json.Unmarshal(data, &sites); err == nil {
		return sites, nil
	}

	var wrap struct {
		Sites map[string]struct {
			URL        string            `json:"url"`
			URLMain    string            `json:"urlMain"`
			CheckType  string            `json:"checkType"`
			AbsenceStr []string          `json:"absenceStrs"`
			Headers    map[string]string `json:"headers"`
			Disabled   bool              `json:"disabled"`
		} `json:"sites"`
	}
	if err := json.Unmarshal(data, &wrap); err == nil {
		var converted []models.Site
		for name, s := range wrap.Sites {
			if s.Disabled { continue }
			absence := ""
			if len(s.AbsenceStr) > 0 { absence = s.AbsenceStr[0] }
			errorType := "message"
			if s.CheckType == "status_code" { errorType = "status_code" }
			converted = append(converted, models.Site{
				Name: name, URL: s.URL, SearchType: "username",
				ErrorType: errorType, ErrorCode: 404, ErrorMsg: absence,
				Weight: 10, Headers: s.Headers,
			})
		}
		return converted, nil
	}
	return nil, fmt.Errorf("invalid JSON format")
}
