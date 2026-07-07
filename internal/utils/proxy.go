package utils

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
		return nil, fmt.Errorf("failed to open proxy file: %v", err)
	}
	defer file.Close()

	var proxies []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			proxies = append(proxies, line)
		}
	}
	if len(proxies) == 0 {
		return nil, fmt.Errorf("proxy file is empty")
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

type DynamicProxyTransport struct {
	Base    *http.Transport
	Rotator *ProxyRotator
}

func (d *DynamicProxyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if d.Rotator == nil {
		return d.Base.RoundTrip(req)
	}

	proxyAddr := d.Rotator.GetNext()
	proxyURL, err := url.Parse(proxyAddr)
	if err != nil {
		return d.Base.RoundTrip(req)
	}

	tempTransport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
	}

	return tempTransport.RoundTrip(req)
}

func NewOptimizedClient(rotator *ProxyRotator) *http.Client {
	baseTransport := &http.Transport{
		MaxIdleConns:        1000,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
	}

	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &DynamicProxyTransport{
			Base:    baseTransport,
			Rotator: rotator,
		},
	}
}
