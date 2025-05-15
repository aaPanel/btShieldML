/*
 * @Date: 2025-04-15 10:43:09
 * @Editors: Mr wpl
 * @Description: 主程序入口
 */
package main

import (
	"bt-shieldml/internal/config"
	"bt-shieldml/internal/engine"
	"bt-shieldml/pkg/logging"
	"flag"
	"os"
	"strings"
)

func main() {
	// --- Argument Parsing ---
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	targetPathsRaw := flag.String("path", "", "Comma-separated files or directories to scan (required)")
	exclusionsRaw := flag.String("exclude", "", "Comma-separated files or directories to exclude")
	outputFormat := flag.String("format", "", "Output format (console, json, html). Overrides config file.")
	reportPath := flag.String("output", "", "Path to save report file (for json/html formats)")

	flag.Parse()

	if *targetPathsRaw == "" {
		logging.ErrorLogger.Println("Error: -path argument is required.")
		flag.Usage()
		os.Exit(1)
	}

	// --- Load Configuration ---
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		// If default config also failed, LoadConfig might return err.
		// If LoadConfig returns nil because file not found & flag not set, it used defaults.
		if cfg == nil {
			logging.ErrorLogger.Fatalf("Failed to load configuration: %v", err)
		}
		// Continue with default config if LoadConfig handled the 'not found' case gracefully
	}

	// Override config with flags if provided
	if *outputFormat != "" {
		cfg.Output.Format = *outputFormat
	}

	// --- Initialize Engine ---
	scanEngine, err := engine.NewEngine(cfg)
	if err != nil {
		logging.ErrorLogger.Fatalf("Failed to initialize engine: %v", err)
	}

	// --- Prepare Scan Task ---
	paths := strings.Split(*targetPathsRaw, ",")
	exclusions := []string{}
	if *exclusionsRaw != "" {
		exclusions = strings.Split(*exclusionsRaw, ",")
	}

	// Trim spaces from paths and exclusions
	for i := range paths {
		paths[i] = strings.TrimSpace(paths[i])
	}
	for i := range exclusions {
		exclusions[i] = strings.TrimSpace(exclusions[i])
	}

	task := &engine.Task{
		Paths:        paths,
		Exclusions:   exclusions,
		ReportPath:   *reportPath,
		OutputFormat: cfg.Output.Format, // Use potentially overridden format
	}

	// --- Run Scan ---
	if err := scanEngine.Scan(task); err != nil {
		logging.ErrorLogger.Fatalf("Scan failed: %v", err)
	}

	logging.InfoLogger.Println("Scan completed successfully.")
}
