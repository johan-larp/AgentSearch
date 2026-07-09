package network

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/proxy"
)

// ClientConfig содержит тюнинговые параметры HTTP-клиента.
type ClientConfig struct {
	RequestTimeout      time.Duration
	MaxIdleConns        int
	MaxIdleConnsPerHost int
}

// NewOptimizedClient создает http.Client с настроенным Transport.
// Если прокси не используются — возвращает единый Transport с агрессивным пулом соединений.
// Если прокси используются — оборачивает в кастомный RoundTripper, который клонирует Transport
// под каждый запрос, избегая data race при смене Proxy/DialContext.
func NewOptimizedClient(cfg ClientConfig, ua *UARotator, pr *ProxyRotator) *http.Client {
	baseTransport := &http.Transport{
		MaxIdleConns:          cfg.MaxIdleConns,
		MaxIdleConnsPerHost:   cfg.MaxIdleConnsPerHost,
		MaxConnsPerHost:       cfg.MaxIdleConnsPerHost,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: cfg.RequestTimeout / 2,
		ExpectContinueTimeout: 1 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS12,
		},
		ForceAttemptHTTP2: true,
	}

	// Без прокси — один Transport на все запросы, пул работает на полную.
	if !pr.HasProxies() {
		return &http.Client{
			Timeout:   cfg.RequestTimeout,
			Transport: baseTransport,
		}
	}

	// С прокси — клонируем Transport под каждый запрос, чтобы избежать гонок
	// при смене DialContext/Proxy. Клонирование копирует только настройки, не сокеты.
	return &http.Client{
		Timeout: cfg.RequestTimeout,
		Transport: &proxyRotatorTransport{
			base:    baseTransport,
			rotator: pr,
			ua:      ua,
		},
	}
}

// proxyRotatorTransport реализует http.RoundTripper с динамической ротацией прокси.
type proxyRotatorTransport struct {
	base    *http.Transport
	rotator *ProxyRotator
	ua      *UARotator
}

func (t *proxyRotatorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Ротация User-Agent на уровне Transport
	if t.ua != nil {
		req.Header.Set("User-Agent", t.ua.GetRandom())
	}

	proxyAddr := t.rotator.GetNext()
	if proxyAddr == "" {
		return t.base.RoundTrip(req)
	}

	proxyURL, err := url.Parse(proxyAddr)
	if err != nil {
		return t.base.RoundTrip(req)
	}

	// Клонируем базовый Transport для изоляции настроек прокси этого конкретного запроса.
	tr := t.base.Clone()

	switch proxyURL.Scheme {
	case "http", "https":
		tr.Proxy = http.ProxyURL(proxyURL)
	case "socks5", "socks5h":
		// golang.org/x/net/proxy: поддерживаем как ContextDialer, так и legacy Dialer
		dialer, err := proxy.FromURL(proxyURL, proxy.Direct)
		if err == nil {
			if cd, ok := dialer.(proxy.ContextDialer); ok {
				tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
					return cd.DialContext(ctx, network, addr)
				}
			} else {
				tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
					return dialer.Dial(network, addr)
				}
			}
			tr.Proxy = nil // отключаем HTTP-прокси
		}
	}

	return tr.RoundTrip(req)
}
