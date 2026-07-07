package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/user/agentsearch/internal/engine"
	"github.com/user/agentsearch/internal/models"
)

var (
	username   string
	userFile   string
	workers    int
	sitesFile  string
	timeoutSec int
	proxyFile  string
	outputFile string
	deepSearch bool
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "agentsearch",
		Short: "AgentSearch - High-speed username and email OSINT tool",
	}

	var searchCmd = &cobra.Command{
		Use:   "search",
		Short: "Search for a user across the database",
		Run: func(cmd *cobra.Command, args []string) {
			executeSearch()
		},
	}

	searchCmd.Flags().BoolVarP(&deepSearch, "deep", "d", false, "Enable Deep Search (Google Dorks)")
	searchCmd.Flags().StringVarP(&username, "username", "u", "", "Username or Email to search for")
	searchCmd.Flags().StringVarP(&userFile, "file", "f", "", "File containing list of usernames")
	searchCmd.Flags().IntVarP(&workers, "workers", "w", 100, "Number of concurrent workers")
	searchCmd.Flags().StringVarP(&sitesFile, "sites", "s", "sites.json", "Path to sites database JSON")
	searchCmd.Flags().StringVarP(&proxyFile, "proxies", "p", "", "Path to proxies file")
	searchCmd.Flags().StringVarP(&outputFile, "output", "o", "results.json", "Output JSON file")
	searchCmd.Flags().IntVarP(&timeoutSec, "timeout", "t", 10, "Request timeout in seconds")

	rootCmd.AddCommand(searchCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func executeSearch() {
	if username == "" && userFile == "" {
		fmt.Println("❌ Error: Please provide either -u (username) or -f (file)")
		return
	}

	sitesData, err := os.ReadFile(sitesFile)
	if err != nil {
		fmt.Printf("❌ Error reading sites file: %v\n", err)
		return
	}

	var sites []models.Site
	if err := json.Unmarshal(sitesData, &sites); err != nil {
		fmt.Printf("❌ Error parsing sites JSON: %v\n", err)
		return
	}

	targets := []string{}
	if username != "" {
		targets = append(targets, username)
	}
	if userFile != "" {
		data, _ := os.ReadFile(userFile)
		for _, line := range strings.Split(string(data), "\n") {
			if line != "" {
				targets = append(targets, strings.TrimSpace(line))
			}
		}
	}

	for _, target := range targets {
		fmt.Printf("\n\033[1;34m🔍 Searching for: %s\033[0m\n", target)
		
		if deepSearch {
			fmt.Println("🌐 Generating Deep Search Dorks...")
			dorks := engine.GenerateDeepQueries(target)
			for _, d := range dorks {
				fmt.Printf("  \033[33m[%s]\033[0m %s\n", d.Engine, d.URL)
			}
		}

		eng, err := engine.NewSearchEngine(sites, target, workers, proxyFile, fmt.Sprintf("results_%s.json", target))
		if err != nil {
			fmt.Printf("❌ Engine error: %v\n", err)
			continue
		}

		progress := make(chan models.Result)
		ctx, cancel := context.WithCancel(context.Background())
		
		go func() {
			for res := range progress {
				if res.Found {
					fmt.Printf(" \033[1;32m[+]\033[0m %-20s | %-10d%% | %s\n", res.SiteName, res.Confidence, res.URL)
				}
			}
		}()

		err = eng.Run(ctx, progress)
		cancel()
		if err != nil {
			fmt.Printf("❌ Search error: %v\n", err)
		}
	}
	fmt.Println("\n✅ All searches completed. Results saved to JSON files.")
}
