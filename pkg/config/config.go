package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	// Excel file configuration
	ExcelFilePath  string `yaml:"excel_file_path"`
	ResponseColumn string `yaml:"response_column"`

	// Claude API configuration
	ClaudeAPIKey  string `yaml:"claude_api_key"`
	ClaudeModel   string `yaml:"claude_model,omitempty"`
	ContextPrompt string `yaml:"context_prompt"`
	SummaryLength int    `yaml:"global_summary_length"` // Renamed from summary_length for clarity

	// Theme summary configuration
	ThemeSummaryPrompt  string `yaml:"theme_summary_prompt,omitempty"`
	GlobalSummaryPrompt string `yaml:"global_summary_prompt,omitempty"`

	// Output language configuration
	OutputLanguage string `yaml:"output_language,omitempty"`

	// Themes (populated after first run)
	Themes []string `yaml:"themes,omitempty"`

	// State management
	StateFilePath string `yaml:"state_file_path,omitempty"`

	// Cache configuration
	CacheEnabled bool   `yaml:"cache_enabled"`
	CacheDir     string `yaml:"cache_dir,omitempty"`

	// Rate limiting configuration
	RateLimitDelay int `yaml:"rate_limit_delay,omitempty"`

	// Performance optimization configuration
	BatchSize       int  `yaml:"batch_size,omitempty"`       // Batch size for processing responses
	ParallelWorkers int  `yaml:"parallel_workers,omitempty"` // Number of parallel workers
	UseParallel     bool `yaml:"use_parallel,omitempty"`     // Whether to use parallel processing

	// Report template configuration
	ReportTemplatePath string `yaml:"report_template_path,omitempty"`
	ReportOutputPath   string `yaml:"report_output_path,omitempty"`
}

// LoadConfig loads the configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Validate required fields
	if cfg.ExcelFilePath == "" {
		return nil, fmt.Errorf("excel_file_path is required")
	}

	if cfg.ResponseColumn == "" {
		return nil, fmt.Errorf("response_column is required")
	}

	if cfg.ClaudeAPIKey == "" {
		return nil, fmt.Errorf("claude_api_key is required")
	}

	// Set defaults
	if cfg.SummaryLength == 0 {
		cfg.SummaryLength = 500 // Default global summary length
	}

	if cfg.CacheDir == "" && cfg.CacheEnabled {
		cfg.CacheDir = ".cache" // Default cache directory
	}

	if cfg.ContextPrompt == "" {
		cfg.ContextPrompt = "Analyze the following survey responses and identify the main themes or topics discussed."
	}

	if cfg.OutputLanguage == "" {
		cfg.OutputLanguage = "en" // Default to English
	}

	if cfg.RateLimitDelay == 0 {
		cfg.RateLimitDelay = 1000 // Default to 1000ms (1 second)
	}

	// Set defaults for performance optimization
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 10 // Default batch size
	}

	if cfg.ParallelWorkers == 0 {
		cfg.ParallelWorkers = 4 // Default number of workers
	}

	if !cfg.UseParallel {
		cfg.UseParallel = true // Default to using parallel processing
	}

	return &cfg, nil
}

// SaveConfig saves the configuration to a YAML file
func SaveConfig(cfg *Config, path string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
