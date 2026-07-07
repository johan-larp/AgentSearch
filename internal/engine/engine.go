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

	"github.com/user/agentsearch/internal/models"
	"github.com/user/agentsearch/internal/utils"
)

type SearchEngine struct {
	Client     *http.Client
	Sites      []models.Site
	ProxyRot   *utils.ProxyRotator
	Target     string
	Workers    int
	OutputFile string
}

func NewSearchEngine(sites []models.Site, target string, workers int, proxyFile string, outputFile string) (*SearchEngine, error) {
	pr, err := utils.NewProxyRotator(proxyFile)
	if err != nil {
		return nil, err
	}

	return &SearchEngine{
		Client:     utils.NewOptimizedClient(pr),
		Sites:      sites,
		ProxyRot:   pr,
		Target:     target,
		Workers:    workers,
		OutputFile: outputFile,
	}, nil
}

func (e *SearchEngine) Run(ctx context.Context, progress chan<- models.Result) error {
	jobs := make(chan models.Site, len(e.Sites))
	results := make(chan models.Result, len(e.Sites))
	var wg sync.WaitGroup

	// Создаем JSON файл для стриминга результатов
	outFile, err := os.Create(e.OutputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer outFile.Close()
	outFile.WriteString("[\n")

	// Запуск воркеров
	for i := 0; i < e.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for site := range jobs {
				results <- e.checkSite(ctx, site)
			}
		}()
	}

	// Загрузка задач
	go func() {
		for _, site := range e.Sites {
			jobs <- site
		}
		close(jobs)
	}()

	// Сбор результатов и запись в файл
	go func() {
		first := true
		for res := range results {
			progress <- res
			
			data, _ := json.MarshalIndent(res, "", "  ")
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
	
	return nil
}

func (e *SearchEngine) checkSite(ctx context.Context, site models.Site) models.Result {
	start := time.Now()
	targetURL := strings.ReplaceAll(site.URL, "{target}", e.Target)
	
	res := models.Result{
		SiteName: site.Name,
		Target:   e.Target,
		URL:      targetURL,
	}

	// Динамическая настройка прокси для этого конкретного запроса
	transport := e.Client.Transport.(*http.Transport)
	if e.ProxyRot != nil {
		proxyAddr := e.ProxyRot.GetNext()
		proxyURL, _ := utils.ParseProxy(proxyAddr)
		if proxyURL != nil {
			// В реальном Go для смены прокси на лету лучше создавать новый Transport 
			// или использовать PerConnContext. Для упрощения и скорости 
			// здесь мы используем оптимизированный клиент, но в финале 
			// добавим полноценный Proxy-Transport.
		}
	}

	req, err := http.NewRequestWithContext(ctx, "GET", targetURL, nil)
	if err != nil {
		res.Error = err.Error()
		return res
	}

	req.Header.Set("User-Agent", utils.GetRandomUA())
	for k, v := range site.Headers {
		req.Header.Set(k, v)
	}

	resp, err := e.Client.Do(req)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer resp.Body.Close()

	// Анализ результата (Smart Scoring)
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	body := string(bodyBytes)

	found, confidence := e.score(site, resp.StatusCode, body)
	res.Found = found
	res.Confidence = confidence
	res.Duration = time.Since(start)

	return res
}

func (e *SearchEngine) score(site models.Site, statusCode int, body string) (bool, int) {
	confidence := 0

	// 1. Проверка по типу ошибки
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

	// 2. Дополнительные проверки (инновационный подход)
	if strings.Contains(strings.ToLower(body), strings.ToLower(e.Target)) {
		confidence += 30
	}

	// 3. Учет веса сайта
	confidence += site.Weight

	if confidence > 100 {
		confidence = 100
	}

	return confidence >= 50, confidence
}
