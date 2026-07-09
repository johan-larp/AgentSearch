# AgentSearch — Refactored

Высокопроизводительный OSINT-инструмент для поиска никнеймов и email-адресов на Go 1.21+.  
Полностью переписан с акцентом на **отказоустойчивость**, **расширяемость** и **минимизацию ложных срабатываний**.

## Архитектурные улучшения

1. **Сетевой уровень** — кастомный `http.Client` с тюнингом `http.Transport`: жесткие таймауты (`DialContext`, `TLSHandshakeTimeout`, `ResponseHeaderTimeout`), агрессивный пул соединений (`MaxIdleConns`, `MaxIdleConnsPerHost`), динамическая ротация User-Agent, поддержка HTTP/HTTPS/SOCKS5-прокси с ротацией из файла **или автозагрузкой с ProxyScrape**.
2. **Worker Pool** — ограниченное число воркеров вместо неуправляемого спавна горутин. Поддержка `context.Context` с таймаутом на весь процесс и **Graceful Shutdown** по `Ctrl+C` / `SIGTERM`.
3. **Smart Detection** — декларативный движок на основе YAML/JSON конфигурации. Поддержка:
   - проверки статус-кодов (в том числе кастомных правил);
   - поиска по подстрокам и регулярным выражениям (`presence_strs`, `absence_strs`, `presence_regexes`, `absence_regexes`);
   - проверки финального URL после редиректов (`redirect_failure_patterns`);
   - проверки специфических HTTP-заголовков (`required_headers`).
4. **Защита от блокировок** — распознавание Cloudflare / WAF по заголовкам (`CF-RAY`), телу ответа и статусу `403/429`. Корректная классификация как `blocked`, а не `found`. Настраиваемый `Rate Limiting` per host.
5. **Логирование и вывод** — структурированные логи через стандартный `log/slog`. Сохранение результатов в `JSON` (потоковый), `CSV` и `TXT` одновременно.
6. **Универсальный лоадер баз** — поддержка нативного YAML, Sherlock `data.json` и Maigret `data.json` (включая обёртку `sites`). Встроенный конвертер для слияния обеих баз в единый YAML.

## Структура проекта

```
AgentSearch/
├── cmd/
│   ├── agentsearch/main.go      # Точка входа — поиск
│   └── convert/main.go          # Утилита слияния Sherlock + Maigret → YAML
├── internal/
│   ├── app/app.go               # Оркестрация: graceful shutdown, pipeline
│   ├── config/
│   │   ├── config.go            # CLI-флаги
│   │   ├── loader.go            # Загрузка YAML/JSON (Sherlock, Maigret, нативный)
│   │   └── convert.go           # Логика мержа и конвертации баз
│   ├── detector/detector.go     # Декларативный движок детекции + WAF-распознавание
│   ├── models/models.go         # Доменные модели
│   ├── network/
│   │   ├── client.go            # Оптимизированный HTTP-клиент
│   │   ├── proxy.go             # Ротация прокси (файл / ProxyScrape API / HTTP / SOCKS5)
│   │   └── ua.go                # Ротация User-Agent
│   ├── ratelimit/limiter.go     # Rate limiter per host
│   ├── storage/
│   │   ├── manager.go           # Мультиформатный менеджер вывода
│   │   ├── json.go              # Потоковый JSON writer
│   │   ├── csv.go               # CSV writer
│   │   └── txt.go               # TXT writer
│   └── worker/pool.go           # Worker Pool с семафором через каналы
├── configs/sites.yaml           # Пример декларативной базы сайтов
├── proxies.txt                  # Пример списка прокси
└── go.mod
```

## Сборка

```bash
go build -o agentsearch ./cmd/agentsearch
go build -o convert    ./cmd/convert
```

## Использование

### Базовый поиск

```bash
./agentsearch -u target_user
```

### Массовый поиск с прокси из файла

```bash
./agentsearch -f users.txt -p proxies.txt -w 100 -rl 200ms -o results
```

### Поиск с прокси из ProxyScrape API (runtime)

Если файл с прокси не указан, но в коде активирован `FetchProxies()`, прокси загружаются автоматически:

```go
proxies := network.FetchProxies() // []string{"1.2.3.4:8080", "5.6.7.8:1080", ...}
```

Для использования через CLI сохраните их во временный файл:

```bash
# Пример: shell-скрипт для запуска с живыми прокси
./agentsearch -u target_user -p <(go run -e '...FetchProxies()...') -w 50
```

### Только JSON-вывод

```bash
./agentsearch -u target_user -of json
```

### Объединение баз Sherlock + Maigret

```bash
# 1. Скачайте свежие базы
curl -L -o sherlock_data.json https://raw.githubusercontent.com/sherlock-project/sherlock/master/sherlock_project/resources/data.json
curl -L -o maigret_data.json  https://raw.githubusercontent.com/soxoj/maigret/main/maigret/resources/data.json

# 2. Сконвертируйте в единый YAML
./convert -o my_sites.yaml sherlock_data.json maigret_data.json

# 3. Используйте для поиска
./agentsearch -u target_user -s my_sites.yaml -w 100 -rl 200ms
```

### Флаги

| Флаг | Описание | По умолчанию |
|------|----------|--------------|
| `-u` | Целевой никнейм / email | — |
| `-f` | Файл со списком целей | — |
| `-s` | База сайтов (YAML/JSON) | `configs/sites.yaml` |
| `-p` | Файл с прокси | — |
| `-w` | Число воркеров | `50` |
| `-rl`| Задержка между запросами к одному хосту | `500ms` |
| `-rt` | Таймаут HTTP-запроса | `15s` |
| `-tt` | Общий таймаут на поиск | `10m` |
| `-o` | Директория для результатов | `output` |
| `-of`| Форматы вывода (json,csv,txt) | `json,csv,txt` |
| `-ua`| Внешний файл со списком User-Agent | — |
| `-mc` | MaxIdleConns | `500` |
| `-mch`| MaxIdleConnsPerHost | `100` |

## Формат базы сайтов

Поддерживаются прямые массивы YAML, а также форматы **Sherlock** и **Maigret** (`{ "sites": { "Name": { ... } } }`).  
Ключевые поля:

```yaml
- name: GitHub
  url: https://github.com/{username}
  url_probe: https://api.github.com/users/{username}  # опционально
  check_type: status_code      # status_code | message | response_url | header
  error_code: 404
  presence_strs:
    - "p-nickname"
  absence_strs:
    - "Not Found"
  absence_regexes:
    - "404\s+error"
  redirect_failure_patterns:
    - "/login"
  required_headers:
    X-User-Exists: "true"
  waf_indicators:
    status_codes: [403]
    header_keys: ["CF-RAY"]
    body_substrings: ["cloudflare"]
  headers:
    Authorization: "Bearer xxx"
  request_method: GET
  request_head_only: false
  follow_redirects: true
  regex_check: "^[a-zA-Z0-9_-]+$"
  weight: 20
  tags: [coding, us]
```

## Совмещение баз Sherlock и Maigret

| Поле | Sherlock (`data.json`) | Maigret (`data.json`) | Наш YAML |
|------|------------------------|----------------------|----------|
| Имя | ключ объекта | ключ внутри `sites` | `name` |
| URL | `url` (`{}`) | `url` (`{username}`) | `url` |
| Probe | `urlProbe` | `urlProbe` | `url_probe` |
| Тип проверки | `errorType` | `checkType` | `check_type` |
| Отсутствие | `errorMsg` (строка/массив) | `absenceStrs` | `absence_strs` |
| Присутствие | — | `presenceStrs` | `presence_strs` |
| Редирект-ошибка | `errorUrl` | `errorUrl` | `error_url` |
| HEAD-only | `request_head_only` | — | `request_head_only` |

Конвертер (`cmd/convert`) автоматически:
- заменяет `{}` → `{username}`;
- нормализует `errorMsg` (строка или массив) в `absence_strs`;
- дедуплицирует по имени+URL (конфликты получают суффикс);
- отбрасывает записи без URL (engine-only сайты Maigret).

## Лицензия

MIT
