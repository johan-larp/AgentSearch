package ratelimit

import (
	"net/url"
	"sync"
	"time"
)

// HostLimiter реализует задержку между запросами к одному хосту,
// чтобы минимизировать 429 Too Many Requests и баны.
type HostLimiter struct {
	delay       time.Duration
	mu          sync.Mutex
	lastRequest map[string]time.Time
}

// NewHostLimiter создает лимитер. delay=0 отключает ограничения.
func NewHostLimiter(delay time.Duration) *HostLimiter {
	return &HostLimiter{
		delay:       delay,
		lastRequest: make(map[string]time.Time),
	}
}

// Wait блокирует вызывающую горутину до тех пор, пока не пройдет
// достаточно времени с момента предыдущего запроса к этому хосту.
func (rl *HostLimiter) Wait(rawURL string) {
	if rl.delay <= 0 {
		return
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return
	}
	host := u.Host
	if host == "" {
		return
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	if last, ok := rl.lastRequest[host]; ok {
		elapsed := time.Since(last)
		if elapsed < rl.delay {
			time.Sleep(rl.delay - elapsed)
		}
	}
	rl.lastRequest[host] = time.Now()
}
