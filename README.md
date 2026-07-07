# 🚀 AgentSearch

**AgentSearch** is a high-performance OSINT tool written in Go, designed to instantly search for user profiles and email addresses across thousands of web resources.

By leveraging an optimized network stack and concurrency, AgentSearch can check over 9,000 sites in less than one minute.

## ✨ Features

- ⚡ **Incredible Speed**: Built with goroutines and an optimized HTTP transport for maximum throughput.
- 🌐 **Dynamic Proxy Rotation**: Built-in proxy rotation on a per-request basis to prevent IP bans.
- 🎯 **Smart Scoring**: An innovative confidence system (Confidence Score) to filter out false positives.
- 🔍 **Deep Search (Dorking)**: Automated generation of specialized search queries for Google, Bing, and DuckDuckGo.
- 📊 **JSON Streaming**: Results are saved to a JSON file in real-time, ensuring no data loss during crashes.
- 🛠️ **Flexible Database**: Supports various check types (HTTP status codes, text markers, redirects).

## 🛠 Installation

### Prerequisites
- [Go 1.21+](https://golang.org/dl/)

### Build
```bash
git clone https://github.com/yourusername/AgentSearch.git
cd AgentSearch
go build -o agentsearch main.go
```

## 🚀 Usage

### Basic search by username
```bash
./agentsearch search -u target_user
```

### Search using a file list, proxies, and high concurrency
```bash
./agentsearch search -f users.txt -p proxies.txt -w 500 -o results.json
```

### Deep Search mode (Dorking)
```bash
./agentsearch search -u target_user --deep
```

### Available Flags
| Flag | Description | Default |
| :--- | :--- | :--- |
| `-u, --username` | Target username or email | `""` |
| `-f, --file` | File containing list of targets | `""` |
| `-w, --workers` | Number of concurrent workers | `100` |
| `-p, --proxies` | Path to proxies file (`ip:port`) | `""` |
| `-s, --sites` | Path to sites database JSON | `sites.json` |
| `-o, --output` | Output JSON file | `results.json` |
| `-d, --deep` | Enable Deep Search (Dorks) | `false` |

## 📂 Database Structure (`sites.json`)

The database allows flexible configuration for each resource:
- `error_type`: `"status_code"` or `"message"`.
- `error_code`: The code (e.g., 404) that signifies a profile is NOT found.
- `error_msg`: A string that, if found in the page body, indicates the profile does not exist.
- `weight`: The priority of the site in the overall confidence system.

## ⚖️ License
MIT License
