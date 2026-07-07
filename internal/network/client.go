package network

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync/atomic"
	"time"
)

type ProxyRotator struct {
	proxies []string
	index   uint64
}

func NewProxyRotator(filePath string) (*ProxyRotator, error) {
	if filePath == "" {
		return nil, nil
	}
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var proxies []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if line := scanner.Text(); line != "" {
			proxies = append(proxies, line)
		}
	}
	return &ProxyRotator{proxies: proxies}, nil
}

func (pr *ProxyRotator) GetNext() string {
	if pr == nil || len(pr.proxies) == 0 {
		return ""
	}
	idx := atomic.AddUint64(&pr.index, 1)
	return pr.proxies[idx%uint64(len(pr.proxies))]
}

type RotatorTransport struct {
	Rotator *ProxyRotator
	Base    *http.Transport
}

func (t *RotatorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.Rotator == nil {
		return t.Base.RoundTrip(req)
	}

	proxyAddr := t.Rotator.GetNext()
	proxyURL, err := url.Parse(proxyAddr)
	if err != nil {
		return t.Base.RoundTrip(req)
	}

	// Для обеспечения максимальной скорости и изоляции прокси, 
	// мы создаем легкий транспорт для каждого уникального прокси
	tempTransport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
	}

	return tempTransport.RoundTrip(req)
}

func NewOptimizedClient(proxyFile string) (*http.Client, error) {
	rotator, err := NewProxyRotator(proxyFile)
	if err != nil {
		return nil, err
	}

	baseTransport := &http.Transport{
		MaxIdleConns:        2000,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
	}

	return &http.Client{
		Timeout: 15 * time.Second,
		Transport: &RotatorTransport{
			Rotator: rotator,
			Base:    baseTransport,
		},
	}, nil
}

func GetRandomUA() string {
	uas := []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_4_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4.1 Mobile/15E148 Safari/604.1",
	}
	return uas[time.Now().UnixNano()%int64(len(uas))]
}
