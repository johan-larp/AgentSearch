package detector

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/johan-larp/agentsearch/internal/models"
)

// DetectionResult содержит результат анализа ответа сервера.
type DetectionResult struct {
	Found      bool
	Confidence int
	Status     models.ResultStatus
}

// Engine реализует декларативный движок детекции.
type Engine struct{}

// New создает новый движок детекции.
func New() *Engine {
	return &Engine{}
}

// Analyze проверяет HTTP-ответ по правилам, заданным в SiteConfig.
// Логика приоритета:
//  1. WAF / защита
//  2. Правила редиректов (final URL)
//  3. Декларативная проверка по CheckType
func (e *Engine) Analyze(site models.SiteConfig, resp *http.Response, body, finalURL string) DetectionResult {
	// 1. Распознавание WAF
	if isWAF(resp, body, site.WAFIndicators) {
		return DetectionResult{Status: models.StatusBlocked, Confidence: 0}
	}

	// Глобальные маркеры защиты из поля protection
	for _, p := range site.Protection {
		if p == "cf_js_challenge" || p == "custom_bot_protection" {
			// Дополнительная эвристика уже учтена в isWAF,
			// но явный маркер гарантирует пометку как blocked.
			return DetectionResult{Status: models.StatusBlocked, Confidence: 0}
		}
	}

	// 2. Проверка редиректов: если финальный URL содержит паттерн неудачи
	for _, pattern := range site.RedirectFailurePatterns {
		if strings.Contains(finalURL, pattern) {
			return DetectionResult{Status: models.StatusNotFound, Confidence: 0}
		}
	}

	// 3. Декларативная логика по типу проверки
	switch site.CheckType {
	case "status_code":
		return e.checkStatusCode(site, resp)
	case "message":
		return e.checkMessage(site, body)
	case "response_url":
		return e.checkResponseURL(site, finalURL)
	case "header":
		return e.checkHeaders(site, resp)
	default:
		// Fallback: если тип не указан, используем эвристики
		if site.ErrorCode != 0 && resp.StatusCode == site.ErrorCode {
			return DetectionResult{Status: models.StatusNotFound, Confidence: 0}
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			return DetectionResult{Found: true, Status: models.StatusFound, Confidence: calculateBaseConfidence(site, resp, body)}
		}
		return DetectionResult{Status: models.StatusNotFound, Confidence: 0}
	}
}

func (e *Engine) checkStatusCode(site models.SiteConfig, resp *http.Response) DetectionResult {
	if site.ErrorCode != 0 && resp.StatusCode == site.ErrorCode {
		return DetectionResult{Status: models.StatusNotFound, Confidence: 0}
	}
	// 2xx / 3xx считаем признаком существования профиля
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		conf := calculateBaseConfidence(site, resp, "")
		return DetectionResult{Found: true, Status: models.StatusFound, Confidence: conf}
	}
	return DetectionResult{Status: models.StatusNotFound, Confidence: 0}
}

func (e *Engine) checkMessage(site models.SiteConfig, body string) DetectionResult {
	// Сначала проверяем absence-индикаторы (профиль НЕ найден)
	for _, s := range site.AbsenceStrs {
		if strings.Contains(body, s) {
			return DetectionResult{Status: models.StatusNotFound, Confidence: 0}
		}
	}
	for _, reStr := range site.AbsenceRegexes {
		if matched, _ := regexp.MatchString(reStr, body); matched {
			return DetectionResult{Status: models.StatusNotFound, Confidence: 0}
		}
	}

	// Затем presence-индикаторы (профиль найден)
	for _, s := range site.PresenceStrs {
		if strings.Contains(body, s) {
			conf := calculateBaseConfidence(site, nil, body)
			return DetectionResult{Found: true, Status: models.StatusFound, Confidence: conf}
		}
	}
	for _, reStr := range site.PresenceRegexes {
		if matched, _ := regexp.MatchString(reStr, body); matched {
			conf := calculateBaseConfidence(site, nil, body)
			return DetectionResult{Found: true, Status: models.StatusFound, Confidence: conf}
		}
	}

	// Нет четких индикаторов — низкая уверенность, чтобы не терять потенциальные находки,
	// но и не завышать ложные срабатывания.
	conf := calculateBaseConfidence(site, nil, body)
	if conf < 30 {
		conf = 30
	}
	return DetectionResult{Found: true, Status: models.StatusFound, Confidence: conf}
}

func (e *Engine) checkResponseURL(site models.SiteConfig, finalURL string) DetectionResult {
	if site.ErrorURL != "" && strings.Contains(finalURL, site.ErrorURL) {
		return DetectionResult{Status: models.StatusNotFound, Confidence: 0}
	}
	return DetectionResult{Found: true, Status: models.StatusFound, Confidence: calculateBaseConfidence(site, nil, "")}
}

func (e *Engine) checkHeaders(site models.SiteConfig, resp *http.Response) DetectionResult {
	for k, v := range site.RequiredHeaders {
		if resp.Header.Get(k) == v {
			conf := calculateBaseConfidence(site, resp, "")
			return DetectionResult{Found: true, Status: models.StatusFound, Confidence: conf}
		}
	}
	return DetectionResult{Status: models.StatusNotFound, Confidence: 0}
}

// isWAF определяет, не ответил ли сервер WAF-заглушкой (Cloudflare, DDoS-GUARD и т.д.).
func isWAF(resp *http.Response, body string, cfg models.WAFConfig) bool {
	lowerBody := strings.ToLower(body)

	// --- Глобальные эвристики ---
	if resp.Header.Get("CF-RAY") != "" {
		return true
	}
	if strings.Contains(strings.ToLower(resp.Header.Get("Server")), "cloudflare") {
		return true
	}
	if resp.StatusCode == 403 && (strings.Contains(lowerBody, "cloudflare") || strings.Contains(lowerBody, "ray id")) {
		return true
	}
	if resp.StatusCode == 429 {
		return true
	}
	if strings.Contains(lowerBody, "ddos-guard") || strings.Contains(lowerBody, "incapsula") || strings.Contains(lowerBody, "sucuri") {
		return true
	}

	// --- Пользовательские правила из конфига сайта ---
	for _, code := range cfg.StatusCodes {
		if resp.StatusCode == code {
			return true
		}
	}
	for _, key := range cfg.HeaderKeys {
		if resp.Header.Get(key) != "" {
			return true
		}
	}
	for _, sub := range cfg.BodySubstrings {
		if strings.Contains(lowerBody, strings.ToLower(sub)) {
			return true
		}
	}

	return false
}

// calculateBaseConfidence вычисляет базовую уверенность на основе веса сайта и эвристик.
func calculateBaseConfidence(site models.SiteConfig, resp *http.Response, body string) int {
	score := site.Weight
	if score == 0 {
		score = 10
	}

	if resp != nil && resp.StatusCode == 200 {
		score += 30
	}

	// Если тело непустое и достаточно большое — скорее всего это реальная страница, а не заглушка
	if len(body) > 200 {
		score += 10
	}

	if score > 100 {
		score = 100
	}
	return score
}
