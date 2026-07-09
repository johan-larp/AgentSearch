# 🔍 AgentSearch

[![Go Version](https://img.shields.io/badge/go-1.21+-00ADD8.svg)](https://golang.org/dl/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

**AgentSearch** — высокопроизводительный OSINT-инструмент для поиска никнеймов и email-адресов по тысячам веб-ресурсов. Написан на Go с акцентом на скорость, отказоустойчивость и минимизацию ложных срабатываний.

## ✨ Возможности

- ⚡ **Worker Pool** — контролируемая конкурентность вместо неуправляемого спавна горутин
- 🌐 **Прокси** — ротация из файла или автозагрузка с ProxyScrape API (HTTP/HTTPS/SOCKS5)
- 🧠 **Smart Detection** — декларативный движок: статус-коды, подстроки, regex, редиректы, заголовки
- 🛡️ **WAF Detection** — распознаёт Cloudflare, DDoS-Guard, Incapsula и не считает их за найденный профиль
- ⏱️ **Rate Limiting** — настраиваемая задержка между запросами к одному хосту
- 📝 **Structured Logging** — `log/slog` с человекочитаемым выводом
- 💾 **Мультиформатный вывод** — JSON (потоковый), CSV и TXT одновременно
- 📦 **Универсальные базы** — поддержка нативного YAML, Sherlock `data.json` и Maigret `data.json`
- 🔄 **Graceful Shutdown** — корректная остановка по `Ctrl+C` без потери результатов

## 📋 Требования

- [Go 1.21+](https://golang.org/dl/)

## 🚀 Установка и сборка

```bash
git clone https://github.com/johan-larp/AgentSearch.git
cd AgentSearch

# Основной бинарник
go build -o agentsearch ./cmd/agentsearch

# Утилита конвертации баз (опционально)
go build -o convert ./cmd/convert
```

## 🎯 Быстрый старт

### 1. Поиск одного никнейма

```bash
./agentsearch -u johndoe
```

Результат сохранится в директорию `output/`:
```
output/
├── johndoe.json
├── johndoe.csv
└── johndoe.txt
```

### 2. Поиск с прокси из файла

Создайте `proxies.txt`:
```
http://123.45.67.89:8080
socks5://98.76.54.32:1080
http://user:pass@proxy.example.com:3128
```

```bash
./agentsearch -u johndoe -p proxies.txt -w 100 -rl 200ms
```

### 3. Массовый поиск по списку

Создайте `targets.txt`:
```
johndoe
janedoe
admin
```

```bash
./agentsearch -f targets.txt -w 50 -o results -of json,csv
```

### 4. Использование объединённой базы Sherlock + Maigret

```bash
# Скачайте свежие базы
curl -L -o sherlock_data.json \
  https://raw.githubusercontent.com/sherlock-project/sherlock/master/sherlock_project/resources/data.json

curl -L -o maigret_data.json \
  https://raw.githubusercontent.com/soxoj/maigret/main/maigret/resources/data.json

# Слейте в единый YAML
./convert -o merged_sites.yaml sherlock_data.json maigret_data.json

# Ищите
./agentsearch -u johndoe -s merged_sites.yaml -w 100 -rl 300ms -tt 30m
```

### 5. Загрузка прокси с ProxyScrape API (в коде)

```go
package main

import "github.com/johan-larp/agentsearch/internal/network"

func main() {
    proxies := network.FetchProxies()
    // proxies == []string{"1.2.3.4:8080", "5.6.7.8:1080", ...}
}
```

## 🎛️ Флаги командной строки

| Флаг | Короткий | Описание | По умолчанию |
|------|----------|----------|--------------|
| `-u` | — | Целевой никнейм или email | *(обязательно, если нет `-f`)* |
| `-f` | — | Файл со списком целей (одна на строку) | — |
| `-s` | — | Путь к базе сайтов (YAML или JSON) | `configs/sites.yaml` |
| `-p` | — | Файл со списком прокси | — |
| `-w` | — | Количество конкурентных воркеров | `50` |
| `-rl` | — | Задержка между запросами к одному хосту | `500ms` |
| `-rt` | — | Таймаут одного HTTP-запроса | `15s` |
| `-tt` | — | Общий таймаут на весь поиск | `10m` |
| `-o` | — | Директория для сохранения результатов | `output` |
| `-of` | — | Форматы вывода через запятую | `json,csv,txt` |
| `-ua` | — | Внешний файл со списком User-Agent | — |
| `-mc` | — | Максимальное число idle-соединений в пуле | `500` |
| `-mch` | — | Максимальное число idle-соединений на хост | `100` |
| `-d` | — | Включить Deep Search (заглушка) | `false` |

### Примеры комбинаций флагов

```bash
# Максимально быстрый поиск (много воркеров, без rate limit)
./agentsearch -u johndoe -w 200 -rl 0s -mc 1000 -mch 200

# Тихий режим только с JSON-выводом
./agentsearch -u johndoe -of json -o /tmp/results

# Поиск с кастомной базой и прокси
./agentsearch -u johndoe -s my_sites.yaml -p proxies.txt -w 80 -rl 100ms

# Долгий поиск по большой базе с увеличенным таймаутом
./agentsearch -u johndoe -s merged_sites.yaml -w 100 -tt 60m -rt 20s
```

## 📂 Примеры конфигураций

### База сайтов (`configs/sites.yaml`)

```yaml
- name: GitHub
  url: https://github.com/{username}
  url_probe: https://api.github.com/users/{username}
  check_type: status_code
  error_code: 404
  regex_check: ^[a-zA-Z0-9](?:[a-zA-Z0-9]|-(?=[a-zA-Z0-9])){0,38}$
  weight: 20
  tags:
    - coding

- name: Twitter/X
  url: https://x.com/{username}
  check_type: message
  absence_strs:
    - "page doesn't exist"
    - "This account doesn't exist"
  presence_strs:
    - '"screen_name":"{username}"'
  weight: 25
  waf_indicators:
    status_codes: [403]
    header_keys: ["CF-RAY"]
    body_substrings: ["cloudflare"]

- name: Instagram
  url: https://www.instagram.com/{username}/
  check_type: message
  absence_strs:
    - "Sorry, this page isn't available."
  presence_strs:
    - '"biography"'
  headers:
    x-ig-app-id: "936619743392459"
  weight: 20

- name: VK
  url: https://vk.com/{username}
  check_type: status_code
  error_code: 404
  weight: 10
  tags:
    - ru
    - social

- name: Steam
  url: https://steamcommunity.com/id/{username}
  check_type: message
  absence_strs:
    - "The specified profile could not be found"
  presence_strs:
    - "actual_persona_name"
  weight: 14
```

### Список прокси (`proxies.txt`)

```
# Строки, начинающиеся с #, игнорируются
http://123.45.67.89:8080
https://98.76.54.32:443
socks5://192.168.1.1:1080
socks5h://proxy.example.com:1080

# Если схема не указана, подразумевается http://
11.22.33.44:3128
```

### Список целей (`targets.txt`)

```
# Комментарии и пустые строки игнорируются
johndoe
janedoe
admin
testuser
```

## 📊 Форматы вывода

### JSON (`results.json`)

Потоковая запись массива объектов. Не требует хранения всего результата в памяти:

```json
[
  {"site_name":"GitHub","target":"johndoe","url":"https://github.com/johndoe","found":false,"confidence":0,"status":"not_found","duration":56200000,"error":"","final_url":""},
  {"site_name":"Twitter/X","target":"johndoe","url":"https://x.com/johndoe","found":false,"confidence":0,"status":"blocked","duration":187000000,"error":"","final_url":""},
  {"site_name":"Instagram","target":"johndoe","url":"https://www.instagram.com/johndoe/","found":true,"confidence":30,"status":"found","duration":164000000,"error":"","final_url":""}
]
```

### CSV (`results.csv`)

```csv
site_name,target,url,found,confidence,status,duration_ms,error,final_url
GitHub,johndoe,https://github.com/johndoe,false,0,not_found,56,,https://github.com/johndoe
Twitter/X,johndoe,https://x.com/johndoe,false,0,blocked,187,,https://x.com/johndoe
Instagram,johndoe,https://www.instagram.com/johndoe/,true,30,found,164,,https://www.instagram.com/johndoe/
```

### TXT (`results.txt`)

```
[GitHub] johndoe | Found: false | Confidence: 0% | Status: not_found | URL: https://github.com/johndoe
[Twitter/X] johndoe | Found: false | Confidence: 0% | Status: blocked | URL: https://x.com/johndoe
[Instagram] johndoe | Found: true | Confidence: 30% | Status: found | URL: https://www.instagram.com/johndoe/
```

## 🏗️ Архитектура

```
┌─────────────────┐     ┌──────────────┐     ┌─────────────────┐
│   CLI Flags     │────▶│  App Config  │────▶│  Sites Loader   │
│  (config.go)    │     │              │     │ (loader.go)     │
└─────────────────┘     └──────────────┘     └─────────────────┘
                                                       │
                              ┌────────────────────────┘
                              ▼
┌─────────────────┐     ┌──────────────┐     ┌─────────────────┐
│  Proxy Rotator  │────▶│ HTTP Client  │◀────│  UA Rotator     │
│  (proxy.go)     │     │ (client.go)  │     │  (ua.go)        │
└─────────────────┘     └──────────────┘     └─────────────────┘
                               │
                               ▼
┌─────────────────┐     ┌──────────────┐     ┌─────────────────┐
│   Worker Pool   │────▶│   Jobs       │────▶│   Processor     │
│  (pool.go)      │     │  (каналы)    │     │  (app.go)       │
└─────────────────┘     └──────────────┘     └─────────────────┘
                               │
                               ▼
┌─────────────────┐     ┌──────────────┐     ┌─────────────────┐
│  Rate Limiter   │────▶│  Detector    │────▶│  WAF Check      │
│ (limiter.go)    │     │(detector.go) │     │ (detector.go)   │
└─────────────────┘     └──────────────┘     └─────────────────┘
                               │
                               ▼
┌─────────────────┐     ┌──────────────┐     ┌─────────────────┐
│  JSON Writer    │◀────│   Storage    │────▶│   CSV Writer    │
│  (json.go)      │     │ (manager.go) │     │  (csv.go)       │
└─────────────────┘     └──────────────┘     └─────────────────┘
                               │
                               ▼
                        ┌──────────────┐
                        │  TXT Writer  │
                        │   (txt.go)   │
                        └──────────────┘
```

## 🔄 Совмещение баз Sherlock и Maigret

AgentSearch поддерживает три формата баз **напрямую**, без конвертации:

| Формат | Структура | Пример файла |
|--------|-----------|--------------|
| Нативный YAML | `[]SiteConfig` | `configs/sites.yaml` |
| Sherlock JSON | `{ "SiteName": { ... } }` | `sherlock/data.json` |
| Maigret JSON | `{ "sites": { "SiteName": { ... } } }` | `maigret/data.json` |

Для слияния Sherlock + Maigret в единый YAML:

```bash
./convert -o merged_sites.yaml sherlock_data.json maigret_data.json
```

**Что делает конвертер:**
- Заменяет `{}` → `{username}` (Sherlock)
- Нормализует `errorMsg` (строка или массив) → `absence_strs`
- Сливает `presenceStrs` + `presenseStrs` (Maigret опечатка)
- Дедуплицирует по имени+URL
- Отбрасывает записи без URL (engine-only сайты)

## 🛡️ Graceful Shutdown

При нажатии `Ctrl+C` или получении `SIGTERM`:
1. Новые задачи не принимаются
2. Текущие воркеры дорабатывают свои запросы
3. Результаты флашатся в файлы
4. Программа завершается корректно

```bash
./agentsearch -u johndoe -s merged_sites.yaml -w 100
# ^C
# time=2026-07-09T17:19:45.000Z level=WARN msg="shutdown signal received, exiting"
```

## 📄 Лицензия

MIT License. См. [LICENSE](LICENSE).
