package utils

import (
	"math/rand"
	"net"
	"net/http"
	"time"
)

var UserAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:125.0) Gecko/20100101 Firefox/125.0",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 14.4; rv:125.0) Gecko/20100101 Firefox/125.0",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	"Mozilla/5.0 (iPhone; CPU iPhone OS 17_4_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4.1 Mobile/15E148 Safari/604.1",
}

// GetRandomUA возвращает случайный User-Agent из списка
func GetRandomUA() string {
	return UserAgents[rand.Intn(len(UserAgents))]
}

// NewOptimizedClient создает HTTP-клиент с оптимизированным транспортом для массовых запросов
func NewOptimizedClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			// Максимальное количество открытых соединений в общем пуле
			MaxIdleConns:        1000, 
			// Максимальное количество соединений на один хост (критично для скорости)
			MaxIdleConnsPerHost: 100,
			// Время ожидания TCP-соединения
			IdleConnTimeout:     90 * time.Second,
			// Оптимизация DNS и TCP
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			// Отключаем проверку TLS для некоторых ресурсов (опционально, для скорости)
			TLSHandshakeTimeout: 5 * time.Second,
		},
	}
}
