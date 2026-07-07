package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/user/agentsearch/internal/engine"
	"github.com/user/agentsearch/internal/models"
	"github.com/user/agentsearch/internal/network"
	"github.com/user/agentsearch/internal/storage"
)

func main() {
	u := flag.String("u", "", "Target username or email")
	f := flag.String("f", "", "File with targets")
	w := flag.Int("w", 100, "Number of workers")
	s := flag.String("s", "sites.json", "Sites database JSON")
	p := flag.String("p", "", "Proxies file")
	o := flag.String("o", "results.json", "Output file")
	flag.Parse()

	if *u == "" && *f == "" {
		fmt.Println("❌ Usage: agentsearch -u target OR -f targets.txt")
		os.Exit(1)
	}

	// 1. Load DB
	data, err := os.ReadFile(*s)
	if err != nil {
		fmt.Printf("❌ DB Error: %v\n", err)
		os.Exit(1)
	}
	var sites []models.Site
	if err := json.Unmarshal(data, &sites); err != nil {
		fmt.Printf("❌ JSON Parse Error: %v\n", err)
		os.Exit(1)
	}

	// 2. Prepare Targets
	targets := []string{}
	if *u != "" {
		targets = append(targets, *u)
	}
	if *f != "" {
		fileData, _ := os.ReadFile(*f)
		for _, line := range strings.Split(string(fileData), "\n") {
			if line := strings.TrimSpace(line); line != "" {
				targets = append(targets, line)
			}
		}
	}

	// 3. Network setup
	client, err := network.NewOptimizedClient(*p)
	if err != nil {
		fmt.Printf("❌ Network Error: %v\n", err)
		os.Exit(1)
	}

	// 4. Execution
	for _, target := range targets {
		fmt.Printf("\n\033[1;34m🔍 Searching for: %s\033[0m\n", target)

		streamer, err := storage.NewJSONStreamer(fmt.Sprintf("res_%s.json", target))
		if err != nil {
			fmt.Printf("❌ Storage Error: %v\n", err)
			continue
		}

		eng := engine.NewEngine(sites, target, *w, streamer, client)
		progress := make(chan models.Result)
		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			for res := range progress {
				if res.Found {
					fmt.Printf(" \033[1;32m[+]\033[0m %-20s | %d%% | %s\n", res.SiteName, res.Confidence, res.URL)
				}
			}
		}()

		eng.Run(ctx, progress)
		cancel()
		streamer.Close()
	}
	fmt.Println("\n✅ All tasks completed.")
}
