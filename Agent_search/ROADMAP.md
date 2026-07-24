# AgentSearch — Roadmap

## Этап 1: Stealth & WAF Evasion (критично)
- [x] Базовая WAF detection
- [ ] **uTLS** — JA3/JA4 fingerprint spoofing (обход Cloudflare TLS fingerprinting)
- [ ] **Retry logic** — exponential backoff на 429/5xx через go-retryablehttp
- [ ] **Proxy health-check** — ping + test request, исключение "мертвых" прокси
- [ ] **Dynamic User-Agent rotation** — расширенный список с актуальными браузерами

## Этап 2: Reporting System (важно)
- [ ] **CLI Report** — цветная таблица с прогресс-баром и итогами
- [ ] **HTML Report** — интерактивный отчёт в браузере (фильтры, сортировка, графики)
- [ ] **DOCX Report** — формальный документ для заказчика
- [ ] **Summary export** — краткая сводка (found/blocked/error по категориям)

## Этап 3: Performance (оптимизация)
- [ ] **sync.Pool** для буферов HTTP-ответов
- [ ] **Bloom Filter** — deduplication запросов (никнейм+сайт)
- [ ] **SQLite** — индексированное хранение базы сайтов вместо YAML в памяти
- [ ] **fasthttp** — опциональный режим для CPU-bound сценариев

## Этап 4: Scale & Observability
- [ ] **Prometheus metrics** — RPS, latency, error rate per host
- [ ] **pprof endpoints** — профилирование CPU/memory
- [ ] **Redis** — distributed rate limiting + shared cache
- [ ] **Pebble/BadgerDB** — on-disk key-value для истории результатов

---

**Текущий фокус:** Этап 1 + Этап 2 (решают ваши конкретные боли).
