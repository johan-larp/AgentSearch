package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/johan-larp/agentsearch/internal/config"
	"github.com/johan-larp/agentsearch/internal/detector"
	"github.com/johan-larp/agentsearch/internal/models"
	"github.com/johan-larp/agentsearch/internal/network"
	"github.com/johan-larp/agentsearch/internal/ratelimit"
	"github.com/johan-larp/agentsearch/internal/report"
	"github.com/johan-larp/agentsearch/internal/storage"
	"github.com/johan-larp/agentsearch/internal/worker"
)

// App объединяет все компоненты системы и управляет жизненным циклом поиска.
type App struct {
	cfg      *config.AppConfig
	client   *http.Client
	ua       *network.UARotator
	detector *detector.Engine
	limiter  *ratelimit.HostLimiter
	sites    []models.SiteConfig
}

// New инициализирует приложение: загружает UA, прокси, сайты и собирает HTTP-клиент.
func New(cfg *config.AppConfig) (*App, error) {
	ua, err := network.NewUARotator(cfg.UserAgentFile)
	if err != nil {
		return nil, fmt.Errorf("ua rotator: %w", err)
	}

	pr, err := network.NewProxyRotator(cfg.ProxiesFile)
	if err != nil {
		return nil, fmt.Errorf("proxy rotator: %w", err)
	}

	client := network.NewOptimizedClient(network.ClientConfig{
		RequestTimeout:      cfg.RequestTimeout,
		MaxIdleConns:        cfg.MaxIdleConns,
		MaxIdleConnsPerHost: cfg.MaxIdleConnsPerHost,
		UseUTLS:             cfg.UseUTLS,
		MaxRetries:          cfg.MaxRetries,
	}, ua, pr)

	sites, err := config.LoadSites(cfg.SitesFile)
	if err != nil {
		return nil, fmt.Errorf("load sites: %w", err)
	}
	slog.Info("sites database loaded", "count", len(sites), "file", cfg.SitesFile)

	return &App{
		cfg:      cfg,
		client:   client,
		ua:       ua,
		detector: detector.New(),
		limiter:  ratelimit.NewHostLimiter(cfg.RateLimitPerHost),
		sites:    sites,
	}, nil
}

// Run запускает поиск по всем таргетам с поддержкой Graceful Shutdown.
func (a *App) Run(ctx context.Context) error {
	// Перехват Ctrl+C / SIGTERM для корректной отмены контекста
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	for _, target := range a.cfg.Targets {
		select {
		case <-ctx.Done():
			slog.Warn("shutdown signal received, exiting")
			return ctx.Err()
		default:
		}
		if err := a.searchTarget(ctx, target); err != nil {
			slog.Error("search target failed", "target", target, "error", err)
		}
	}
	return nil
}

func (a *App) searchTarget(ctx context.Context, target string) error {
	slog.Info("starting search", "target", target, "workers", a.cfg.Workers)
	startTotal := time.Now()

	// Инициализация хранилищ результатов
	store, err := storage.NewManager(a.cfg.OutputDir, a.cfg.OutputFormats, target)
	if err != nil {
		return err
	}
	defer store.Close()

	// Создаем процессор и пул воркеров
	proc := &siteProcessor{app: a, target: target}
	pool := worker.NewPool(a.cfg.Workers, proc)
	pool.Start(ctx)

	// Собираем результаты для отчётов
	var allResults []models.Result

	// Горутина-потребитель результатов
	done := make(chan struct{})
	go func() {
		for res := range pool.Results() {
			store.Write(res)
			a.logResult(res)
			allResults = append(allResults, res)
		}
		close(done)
	}()

	// Подача задач
	jobCount := 0
	for _, site := range a.sites {
		if site.URL == "" {
			continue
		}
		select {
		case <-ctx.Done():
			break
		default:
			pool.Submit(worker.Job{Site: site, Target: target})
			jobCount++
		}
	}
	slog.Info("jobs submitted", "target", target, "count", jobCount)

	// Закрываем канал jobs — воркеры завершат текущие задачи и выйдут
	pool.Close()
	<-done

	elapsed := time.Since(startTotal)
	slog.Info("search completed", "target", target, "elapsed", elapsed.Round(time.Second), "results", len(allResults))

	// Генерация отчётов
	if len(a.cfg.ReportFormats) > 0 {
		if err := report.WriteAll(a.cfg.OutputDir, a.cfg.ReportFormats, target, allResults, elapsed); err != nil {
			slog.Error("report generation failed", "error", err)
		}
	}

	return nil
}

func (a *App) logResult(res models.Result) {
	switch res.Status {
	case models.StatusFound:
		slog.Info("profile found", "site", res.SiteName, "url", res.URL, "confidence", res.Confidence)
	case models.StatusBlocked:
		slog.Warn("blocked by WAF", "site", res.SiteName, "url", res.URL)
	case models.StatusError:
		slog.Debug("request error", "site", res.SiteName, "error", res.Error)
	default:
		slog.Debug("profile not found", "site", res.SiteName, "url", res.URL)
	}
}

// siteProcessor реализует worker.Processor — логику обработки одного сайта.
type siteProcessor struct {
	app    *App
	target string
}

// Process выполняет HTTP-запрос и применяет декларативный движок детекции.
func (p *siteProcessor) Process(ctx context.Context, job worker.Job) models.Result {
	site := job.Site
	start := time.Now()

	// Подстановка переменных в URL
	checkURL := strings.ReplaceAll(site.URL, "{username}", p.target)
	checkURL = strings.ReplaceAll(checkURL, "{target}", p.target)
	// urlProbe имеет приоритет, если задан
	if site.URLProbe != "" {
		checkURL = strings.ReplaceAll(site.URLProbe, "{username}", p.target)
		checkURL = strings.ReplaceAll(checkURL, "{target}", p.target)
	}

	res := models.Result{
		SiteName: site.Name,
		Target:   p.target,
		URL:      checkURL,
		Status:   models.StatusError,
	}

	// Rate limiting per host
	p.app.limiter.Wait(checkURL)

	// Формирование запроса
	method := http.MethodGet
	if site.RequestMethod != "" {
		method = site.RequestMethod
	}
	if site.RequestHeadOnly && method == http.MethodGet {
		method = http.MethodHead
	}

	var bodyReader io.Reader
	if site.RequestPayload != "" {
		payload := strings.ReplaceAll(site.RequestPayload, "{username}", p.target)
		payload = strings.ReplaceAll(payload, "{target}", p.target)
		bodyReader = strings.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, checkURL, bodyReader)
	if err != nil {
		res.Error = err.Error()
		return res
	}

	// Заголовки: ротация UA + кастомные заголовки сайта
	req.Header.Set("User-Agent", p.app.ua.GetRandom())
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("DNT", "1")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	for k, v := range site.Headers {
		req.Header.Set(k, strings.ReplaceAll(v, "{username}", p.target))
	}

	// Выполнение запроса с контролем редиректов
	client := p.app.client
	if !site.FollowRedirects {
		noRedirect := *client
		noRedirect.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
		client = &noRedirect
	}

	resp, err := client.Do(req)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer resp.Body.Close()

	// Определяем финальный URL
	finalURL := checkURL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}

	// Читаем тело с ограничением (128 KiB) — защита от огромных ответов
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
	if err != nil {
		res.Error = err.Error()
		return res
	}
	body := string(bodyBytes)

	// Декларативная детекция
	detection := p.app.detector.Analyze(site, resp, body, finalURL)
	res.Found = detection.Found
	res.Confidence = detection.Confidence
	res.Status = detection.Status
	res.Duration = time.Since(start)
	if finalURL != checkURL {
		res.FinalURL = finalURL
	}

	return res
}
