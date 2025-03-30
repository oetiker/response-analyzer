package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/oetiker/response-analyzer/pkg/analysis"
	"github.com/oetiker/response-analyzer/pkg/cache"
	"github.com/oetiker/response-analyzer/pkg/claude"
	"github.com/oetiker/response-analyzer/pkg/config"
	"github.com/oetiker/response-analyzer/pkg/excel"
	"github.com/oetiker/response-analyzer/pkg/logging"
	"github.com/oetiker/response-analyzer/pkg/output"
	"github.com/oetiker/response-analyzer/pkg/validation"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "", "Path to the configuration file")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")
	identifyThemesOnly := flag.Bool("identify-themes-only", false, "Only identify themes without performing full analysis")
	flag.Parse()

	// Initialize logger
	logger := logging.NewLogger(*verbose)
	logger.Info("Starting response analyzer")

	// Check if config file is provided
	if *configPath == "" {
		logger.Error("No configuration file provided")
		fmt.Println("Please provide a configuration file using the -config flag")
		flag.Usage()
		os.Exit(1)
	}

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		logger.Error("Failed to load configuration", "error", err)
		fmt.Printf("Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	// Create state file path if not specified in config
	if cfg.StateFilePath == "" {
		dir := filepath.Dir(*configPath)
		base := filepath.Base(*configPath)
		ext := filepath.Ext(base)
		name := base[:len(base)-len(ext)]
		cfg.StateFilePath = filepath.Join(dir, name+".state.yaml")
	}

	logger.Info("Configuration loaded", "excel_file", cfg.ExcelFilePath, "state_file", cfg.StateFilePath)

	// Run the main workflow
	claudeClient, err := runWorkflow(logger, cfg, *identifyThemesOnly)
	if err != nil {
		logger.Error("Workflow failed", "error", err)
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Get total cost from Claude client
	totalCost := claudeClient.GetTotalCost()
	totalTokens := claudeClient.GetTotalTokens()
	logger.Info("Response analysis completed",
		"total_tokens", totalTokens,
		"total_cost", fmt.Sprintf("$%.4f", totalCost))
	fmt.Printf("\nTotal tokens used: %d\n", totalTokens)
	fmt.Printf("Total cost: $%.4f\n", totalCost)
}

// runWorkflow runs the main workflow
func runWorkflow(logger *logging.Logger, cfg *config.Config, identifyThemesOnly bool) (*claude.Client, error) {
	// Validate configuration
	validator := validation.NewValidator(logger)
	if err := validator.ValidateConfig(cfg); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	// Initialize cache
	cacheDir := cfg.CacheDir
	if cacheDir == "" {
		cacheDir = ".cache"
	}
	cacheInstance, err := cache.NewCache(logger, cacheDir, 24*time.Hour, cfg.CacheEnabled)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize cache: %w", err)
	}

	// Initialize Claude API client
	claudeClient := claude.NewClient(cfg.ClaudeAPIKey, logger, cacheInstance, cfg.OutputLanguage, cfg.ClaudeModel)

	// Set rate limit delay if configured
	if cfg.RateLimitDelay > 0 {
		claudeClient.SetRateLimitDelay(time.Duration(cfg.RateLimitDelay) * time.Millisecond)
		logger.Info("Rate limit delay set", "delay_ms", cfg.RateLimitDelay)
	}

	// Initialize Excel reader
	excelReader := excel.NewExcelReader(logger)

	// Initialize analyzer
	analyzer := analysis.NewAnalyzer(logger, claudeClient)

	// Log performance optimization settings
	if cfg.UseParallel {
		logger.Info("Using parallel processing",
			"workers", cfg.ParallelWorkers,
			"batch_size", cfg.BatchSize)
	} else {
		logger.Info("Using batch processing",
			"batch_size", cfg.BatchSize)
	}

	// Initialize output writer
	writer := output.NewWriter(logger)

	// Read responses from Excel file
	responses, err := excelReader.ReadResponses(cfg.ExcelFilePath, cfg.ResponseColumn)
	if err != nil {
		return nil, fmt.Errorf("failed to read responses: %w", err)
	}

	logger.Info("Read responses from Excel file", "count", len(responses))

	// Check if state file exists
	var previousResult *analysis.AnalysisResult
	stateExists, err := validator.ValidateStateFile(cfg.StateFilePath)
	if err != nil {
		logger.Warn("Failed to validate state file", "error", err)
	} else if stateExists {
		// Load previous state
		previousResult, err = writer.LoadState(cfg.StateFilePath)
		if err != nil {
			logger.Warn("Failed to load previous state", "error", err)
		} else {
			logger.Info("Loaded previous state",
				"themes", len(previousResult.Themes),
				"responses", len(previousResult.ResponseAnalyses))
		}
	}

	// Check if we're in identify-themes-only mode or if no themes are provided
	if identifyThemesOnly || (len(cfg.Themes) == 0 && (previousResult == nil || len(previousResult.Themes) == 0)) {
		// Only identify themes without performing full analysis
		logger.Info("Running in identify-themes-only mode")

		// Identify themes
		themes, err := analyzer.IdentifyThemesOnly(responses, cfg.ContextPrompt)
		if err != nil {
			return nil, fmt.Errorf("failed to identify themes: %w", err)
		}

		// Output identified themes
		fmt.Println("\nIdentified themes:")
		for i, theme := range themes {
			fmt.Printf("%d. %s\n", i+1, theme)
		}

		// Save themes to a file
		themesPath := filepath.Join(filepath.Dir(cfg.StateFilePath), "themes.yaml")
		if err := writer.SaveThemes(themes, themesPath); err != nil {
			logger.Warn("Failed to save themes", "error", err)
		} else {
			logger.Info("Saved themes to file", "path", themesPath)
			fmt.Printf("\nThemes saved to: %s\n", themesPath)
		}

		fmt.Println("\n==========================================================")
		fmt.Println("THEMES IDENTIFICATION COMPLETED")
		fmt.Println("==========================================================")
		fmt.Println("To perform the full analysis:")
		fmt.Println("1. Add these themes to your config file under the 'themes:' section")
		fmt.Println("2. Run the program again without the -identify-themes-only flag")
		fmt.Println("==========================================================")

		return claudeClient, nil
	}

	// Update analyzer to use configuration settings
	analyzer.SetBatchSize(cfg.BatchSize)
	analyzer.SetParallelWorkers(cfg.ParallelWorkers)
	analyzer.SetUseParallel(cfg.UseParallel)

	// Perform full analysis
	var result *analysis.AnalysisResult
	if len(cfg.Themes) > 0 {
		// Use themes from config
		logger.Info("Using themes from configuration", "count", len(cfg.Themes))
		result, err = analyzer.AnalyzeResponses(
			responses,
			cfg.Themes,
			cfg.ContextPrompt,
			cfg.SummaryPrompt,
			cfg.ThemeSummaryPrompt,
			cfg.GlobalSummaryPrompt,
			cfg.SummaryLength,
			previousResult,
		)
	} else if previousResult != nil && len(previousResult.Themes) > 0 {
		// Use themes from previous state
		logger.Info("Using themes from previous state", "count", len(previousResult.Themes))
		result, err = analyzer.AnalyzeResponses(
			responses,
			previousResult.Themes,
			cfg.ContextPrompt,
			cfg.SummaryPrompt,
			cfg.ThemeSummaryPrompt,
			cfg.GlobalSummaryPrompt,
			cfg.SummaryLength,
			previousResult,
		)
	} else {
		// Identify themes and perform full analysis
		logger.Info("No themes provided, identifying themes and performing full analysis")
		result, err = analyzer.AnalyzeResponses(
			responses,
			nil,
			cfg.ContextPrompt,
			cfg.SummaryPrompt,
			cfg.ThemeSummaryPrompt,
			cfg.GlobalSummaryPrompt,
			cfg.SummaryLength,
			previousResult,
		)

		// Output identified themes
		fmt.Println("\nIdentified themes:")
		for i, theme := range result.Themes {
			fmt.Printf("%d. %s\n", i+1, theme)
		}
		fmt.Println("\nAdd these themes to your config file to use them in subsequent runs.")

		// Save themes to a file
		themesPath := filepath.Join(filepath.Dir(cfg.StateFilePath), "themes.yaml")
		if err := writer.SaveThemes(result.Themes, themesPath); err != nil {
			logger.Warn("Failed to save themes", "error", err)
		} else {
			logger.Info("Saved themes to file", "path", themesPath)
			fmt.Printf("\nThemes saved to: %s\n", themesPath)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to analyze responses: %w", err)
	}

	// Save state
	if err := writer.SaveState(result, cfg.StateFilePath); err != nil {
		return nil, fmt.Errorf("failed to save state: %w", err)
	}

	// Save audit log
	auditPath := filepath.Join(filepath.Dir(cfg.StateFilePath), "audit.yaml")
	if err := writer.SaveAuditLog(result, auditPath); err != nil {
		logger.Warn("Failed to save audit log", "error", err)
	} else {
		logger.Info("Saved audit log", "path", auditPath)
		fmt.Printf("\nAudit log saved to: %s\n", auditPath)
	}

	// Save theme statistics
	statsPath := filepath.Join(filepath.Dir(cfg.StateFilePath), "theme_stats.yaml")
	if err := writer.SaveThemeStats(result, statsPath); err != nil {
		logger.Warn("Failed to save theme statistics", "error", err)
	} else {
		logger.Info("Saved theme statistics", "path", statsPath)
		fmt.Printf("Theme statistics saved to: %s\n", statsPath)
	}

	// Save summary if available
	if result.Summary != "" {
		summaryPath := filepath.Join(filepath.Dir(cfg.StateFilePath), "summary.txt")
		if err := writer.SaveSummary(result.Summary, summaryPath); err != nil {
			logger.Warn("Failed to save summary", "error", err)
		} else {
			logger.Info("Saved summary", "path", summaryPath)
			fmt.Printf("Summary saved to: %s\n", summaryPath)
		}
	}

	// Generate report if template is provided
	if cfg.ReportTemplatePath != "" {
		reportPath := cfg.ReportOutputPath
		if reportPath == "" {
			reportPath = filepath.Join(filepath.Dir(cfg.StateFilePath), "report.txt")
		}
		if err := writer.GenerateReport(result, cfg.ReportTemplatePath, reportPath); err != nil {
			logger.Warn("Failed to generate report", "error", err)
		} else {
			logger.Info("Generated report", "path", reportPath)
			fmt.Printf("Report generated at: %s\n", reportPath)
		}
	}

	return claudeClient, nil
}
