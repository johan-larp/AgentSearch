package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// --- MODELS ---

type Site struct {
	Name       string            `json:"name"`
	URL        string            `json:"url"`
	SearchType string            `json:"search_type"`
	ErrorType  string            `json:"error_type"`
	ErrorCode  int               `json:"error_code"`
	ErrorMsg   string            `json:"error_msg"`
	Weight     int               `json:"weight"`
	Headers    map[string]string `json:"headers"`
}

type Result struct {
	SiteName   string        `json:"site_name"`
	Target     string        `json:"target"`
	URL 	   string 		 `json:"url"`
	Found      bool          `json:"found"`
	Confidence int           `json:"confidence"`
	Duration   time.Duration `json:"duration"`
	Error      string        `json:"error,omitempty"`
}

// --- NETWORK ---

var UserAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
}

type ProxyRotator struct {
	proxies []string
	index   uint64
}

func NewProxyRotator(filePath string) *ProxyRotator {
	if filePath == "" { return nil }
	file, err := os.Open(filePath)
	if err != nil { return nil }
	defer file.Close()
	var proxies []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if line := scanner.Text(); line != "" {
			proxies = append(proxies, line)
		}
	}
	return &ProxyRotator{proxies: proxies}
}

func (pr *ProxyRotator) GetNext() string {
	if pr == nil || len(pr.proxies) == 0 { return "" }
	idx := atomic.AddUint64(&pr.index, 1)
	return pr.proxies[idx%uint64(len(pr.proxies))]
}

type RotatorTransport struct {
	Rotator *ProxyRotator
}

func (t *RotatorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.Rotator == nil {
		return http.DefaultTransport.RoundTrip(req)
	}
	proxyAddr := t.Rotator.GetNext()
	proxyURL, err := url.Parse(proxyAddr)
	if err != nil {
		return http.DefaultTransport.RoundTrip(req)
	}
	tempTransport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
		DialContext: (&net.Dialer{
			Timeout: 5 * time.Second,
		}).DialContext,
	}
	return tempTransport.RoundTrip(req)
}

// --- ENGINE ---

type Engine struct {
	Client     *http.Client
	Sites      []Site
	Target     string
	Workers    int
	OutputFile string
}

func NewEngine(sites []Site, target string, workers int, proxyFile string, outputFile string) *Engine {
	pr := NewProxyRotator(proxyFile)
	return &Engine{
		Client: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &RotatorTransport{Rotator: pr},
		},
		Sites:      sites,
		Target:     target,
		Workers:    workers,
		OutputFile: outputFile,
	}
}

func (e *Engine) Run(ctx context.Context, progress chan<- Result) {
	jobs := make(chan Site, len(e.Sites))
	results := make(chan Result, len(e.Sites))
	var wg sync.WaitGroup

	outFile, _ := os.Create(e.OutputFile)
	defer outFile.Close()
	outFile.WriteString("[\n")

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
		first := true
		for res := range results {
			progress <- res
			data, _ := json.Marshal(res)
			if !first {
				outFile.WriteString(",\n")
			}
			outFile.Write(data)
			first = false
		}
	}()

	wg.Wait()
	close(results)
	outFile.WriteString("\n]")
}

func (e *Engine) checkSite(ctx context.Context, site Site) Result {
	start := time.Now()
	targetURL := strings.ReplaceAll(site.URL, "{target}", e.Target)
	targetURL = strings.ReplaceAll(targetURL, "{username}", e.Target)
	res := Result{SiteName: site.Name, Target: e.Target, URL: targetURL}

	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		res.Error = err.Error()
		return res
	}

	req.Header.Set("User-Agent", UserAgents[time.Now().UnixNano()%int64(len(UserAgents))])
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

	found, confidence := e.score(site, resp.StatusCode, body)
	res.Found = found
	res.Confidence = confidence
	res.Duration = time.Since(start)

	return res
}

func (e *Engine) score(site Site, statusCode int, body string) (bool, int) {
	confidence := 0
	switch site.ErrorType {
	case "status_code":
		if statusCode == site.ErrorCode {
			return false, 0
		}
		if statusCode == 200 {
			confidence += 40
		}
	case "message":
		if site.ErrorMsg != "" && strings.Contains(body, site.ErrorMsg) {
			return false, 0
		}
		confidence += 40
	}

	if strings.Contains(strings.ToLower(body), strings.ToLower(e.Target)) {
		confidence += 30
	}
	confidence += site.Weight
	if confidence > 100 {
		confidence = 100
	}
	return confidence >= 50, confidence
}

// --- SMART JSON LOADER ---

func loadSites(path string) ([]Site, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Try parsing as direct array
	var sites []Site
	if err := json.Unmarshal(data, &sites); err == nil {
		return sites, nil
	}

	// Try parsing as object { "sites": { "Name": { ... } } }
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
		var converted []Site
		for name, s := range wrap.Sites {
			if s.Disabled {
				continue
			}
			absence := ""
			if len(s.AbsenceStr) > 0 {
				absence = s.AbsenceStr[0]
			}
			errorType := "message"
			if s.CheckType == "status_code" {
				errorType = "status_code"
			}
			converted = append(converted, Site{
				Name:       name,
				URL:        s.URL,
				SearchType: "username",
				ErrorType:  errorType,
				ErrorCode:  404,
				ErrorMsg:   absence,
				Weight:     10,
				Headers:    s.Headers,
			})
		}
		return converted, nil
	}

	return nil, fmt.Errorf("invalid JSON format")
}

// --- MAIN ---

func main() {
	u := flag.String("u", "", "Target username or email")
	f := flag.String("f", "", "File with targets")
	w := flag.Int("w", 100, "Number of workers")
	s := flag.String("s", "sites.json", "Sites database JSON")
	p := flag.String("p", "", "Proxies file")
	_ = flag.String("o", "results.json", "Output file")
	flag.Parse()

	if *u == "" && *f == "" {
		fmt.Println("❌ Usage: ./agentsearch -u target OR -f targets.txt")
		os.Exit(1)
	}

	sites, err := loadSites(*s)
	if err != nil {
		fmt.Printf("❌ DB Error: %v\n", err)
		os.Exit(1)
	}

	targets := []string{}
	if *u != "" {
		targets = append(targets, *u)
	}
	if *f != "" {
		fileData, _ := os.ReadFile(*f)
		for _, line := range strings.Split(string(fileData), "\n") {
			if line := strings.TrimSpace(line); line != "" {
				targets = append(targets, line)
			}
		}
	}

	for _, target := range targets {
		fmt.Printf("\n\033[1;34m🔍 Searching for: %s\033[0m\n", target)
		eng := NewEngine(sites, target, *w, *p, fmt.Sprintf("res_%s.json", target))
		progress := make(chan Result)
		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			for res := range progress {
				if res.Found {
					fmt.Printf(" \033[1;32m[+]\033[0m %-20s | %d%% | %s\n", res.SiteName, res.Confidence, res.URL)
				}
			}
		}()

		eng.Run(ctx, progress)
		cancel()
	}
	fmt.Println("\n✅ All targets processed.")
}
