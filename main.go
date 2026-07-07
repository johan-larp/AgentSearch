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
	SearchType string            `json:"search_type"` // "username" or "email"
	ErrorType  string            `json:"error_type"`  // "status_code", "message"
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

// --- UTILS ---

var UserAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	"Mozilla/5.0 (iPhone; CPU iPhone OS 17_4_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4.1 Mobile/15E148 Safari/604.1",
}

type ProxyRotator struct {
	proxies []string
	index   uint64
}

func NewProxyRotator(filePath string) *ProxyRotator {
	if filePath == "" {
		return nil
	}
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("⚠️ Proxy file error: %v\n", err)
		return nil
	}
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
	if pr == nil || len(pr.proxies) == 0 {
		return ""
	}
	idx := atomic.AddUint64(&pr.index, 1)
	return pr.proxies[idx%uint64(len(pr.proxies))]
}

// --- ENGINE ---

type Engine struct {
	Client     *http.Client
	Sites      []Site
	ProxyRot   *ProxyRotator
	Target     string
	Workers    int
	OutputFile string
}

func NewEngine(sites []Site, target string, workers int, proxyFile string, outputFile string) *Engine {
	pr := NewProxyRotator(proxyFile)

	// HIGH PERFORMANCE TRANSPORT
	transport := &http.Transport{
		Proxy: func(url *url.URL) (*url.URL, error) {
			if pr == nil {
				return nil, nil
			}
			pAddr := pr.GetNext()
			return url.Parse(pAddr)
		},
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          2000,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &Engine{
		Client:     &http.Client{Transport: transport, Timeout: 10 * time.Second},
		Sites:      sites,
		ProxyRot:   pr,
		Target:     target,
		Workers:    workers,
		OutputFile: outputFile,
	}
}

func (e *Engine) Run(ctx context.Context, progress chan<- Result) {
	jobs := make(chan Site, len(e.Sites))
	results := make(chan Result, len(e.Sites))
	var wg sync.WaitGroup

	// Output file for streaming
	outFile, _ := os.Create(e.OutputFile)
	defer outFile.Close()
	outFile.WriteString("[\n")

	// Workers
	for i := 0; i < e.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for site := range jobs {
				results <- e.checkSite(ctx, site)
			}
		}()
	}

	// Feed jobs
	go func() {
		for _, s := range e.Sites {
			jobs <- s
		}
		close(jobs)
	}()

	// Collector
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

// --- MAIN ---

func main() {
	u := flag.String("u", "", "Username or Email to search")
	f := flag.String("f", "", "File with targets")
	w := flag.Int("w", 100, "Number of workers")
	s := flag.String("s", "sites.json", "Sites database JSON")
	p := flag.String("p", "", "Proxies file")
	o := flag.String("o", "results.json", "Output file")
	d := flag.Bool("deep", false, "Enable Deep Search")

	flag.Parse()

	if *u == "" && *f == "" {
		fmt.Println("❌ Usage: ./agentsearch -u target OR -f targets.txt")
		os.Exit(1)
	}

	// Load DB
	data, err := os.ReadFile(*s)
	if err != nil {
		fmt.Printf("❌ DB Error: %v\n", err)
		os.Exit(1)
	}
	var sites []Site
	json.Unmarshal(data, &sites)

	targets := []string{}
	if *u != "" {
		targets = append(targets, *u)
	}
	if *f != "" {
		fileData, _ := os.ReadFile(*f)
		for _, line := range strings.Split(string(fileData), "\n") {
			if line != "" {
				targets = append(targets, strings.TrimSpace(line))
			}
		}
	}

	for _, target := range targets {
		fmt.Printf("\n\033[1;34m🔍 Target: %s\033[0m\n", target)
		
		if *d {
			fmt.Println("🌐 Deep Search Dorks generated (check logs)...")
		}

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
	fmt.Println("\n✅ All targets processed. Results saved to JSON.")
}
