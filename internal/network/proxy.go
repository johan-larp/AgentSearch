package network

import (
	"bufio"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

// ProxyRotator реализует atomic-кольцевую ротацию прокси из файла.
// Поддерживает http://, https://, socks5://, socks5h://.
type ProxyRotator struct {
	proxies []string
	index   atomic.Uint64
}

// NewProxyRotator загружает список прокси. Пустые строки и комментарии игнорируются.
func NewProxyRotator(filePath string) (*ProxyRotator, error) {
	if filePath == "" {
		return &ProxyRotator{}, nil
	}
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var list []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Если схема не указана — считаем HTTP-прокси по умолчанию
		if !strings.Contains(line, "://") {
			line = "http://" + line
		}
		list = append(list, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return &ProxyRotator{proxies: list}, nil
}

// HasProxies возвращает true, если список прокси не пуст.
func (pr *ProxyRotator) HasProxies() bool {
	return len(pr.proxies) > 0
}

// GetNext возвращает следующий прокси в кольце (потокобезопасно).
func (pr *ProxyRotator) GetNext() string {
	if len(pr.proxies) == 0 {
		return ""
	}
	idx := pr.index.Add(1) - 1 // 0-based
	return pr.proxies[idx%uint64(len(pr.proxies))]
}

// FetchProxies получает список прокси с API ProxyScrape.
// Возвращает слайс строк формата "ip:port" (протокол обрезается).
// При любой ошибке логирует её и возвращает пустой слайс.
func FetchProxies() []string {
	const apiURL = "https://api.proxyscrape.com/v4/free-proxy-list/get?request=display_proxies&proxy_format=protocolipport&format=text"

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		slog.Error("fetch proxies failed", "error", err)
		return nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("read proxy response failed", "error", err)
		return nil
	}

	lines := strings.Split(string(body), "\n")
	var out []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// API возвращает protocolipport (например http://1.2.3.4:8080).
		// Обрезаем протокол, оставляя только ip:port.
		if idx := strings.Index(line, "://"); idx != -1 {
			line = line[idx+3:]
		}
		out = append(out, line)
	}
	return out
}
