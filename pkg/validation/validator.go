package validation

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/oetiker/response-analyzer/pkg/config"
	"github.com/oetiker/response-analyzer/pkg/excel"
	"github.com/oetiker/response-analyzer/pkg/logging"
)

// Validator handles validation of inputs
type Validator struct {
	logger *logging.Logger
}

// NewValidator creates a new Validator instance
func NewValidator(logger *logging.Logger) *Validator {
	return &Validator{
		logger: logger,
	}
}

// ValidateConfig validates the configuration
func (v *Validator) ValidateConfig(cfg *config.Config) error {
	v.logger.Info("Validating configuration")

	// Check if Excel file exists
	if _, err := os.Stat(cfg.ExcelFilePath); os.IsNotExist(err) {
		return fmt.Errorf("Excel file does not exist: %s", cfg.ExcelFilePath)
	}

	// Check if response column is valid
	if cfg.ResponseColumn == "" {
		return fmt.Errorf("response_column is required")
	}

	// Check if Claude API key is provided
	if cfg.ClaudeAPIKey == "" {
		return fmt.Errorf("claude_api_key is required")
	}

	// Validate Excel file and column
	excelReader := excel.NewExcelReader(v.logger)
	if err := excelReader.ValidateExcelFile(cfg.ExcelFilePath, cfg.ResponseColumn); err != nil {
		return fmt.Errorf("Excel file validation failed: %w", err)
	}

	// Validate output language
	validLanguages := map[string]bool{
		"en":    true,
		"de":    true,
		"de-ch": true,
		"fr":    true,
		"it":    true,
	}
	if !validLanguages[cfg.OutputLanguage] {
		return fmt.Errorf("invalid output_language: %s (valid options: en, de, de-ch, fr, it)", cfg.OutputLanguage)
	}

	// Check if report template exists if provided
	if cfg.ReportTemplatePath != "" {
		if _, err := os.Stat(cfg.ReportTemplatePath); os.IsNotExist(err) {
			return fmt.Errorf("report template file does not exist: %s", cfg.ReportTemplatePath)
		}
	}

	// Check if cache directory exists or can be created
	if cfg.CacheEnabled && cfg.CacheDir != "" {
		if _, err := os.Stat(cfg.CacheDir); os.IsNotExist(err) {
			v.logger.Info("Creating cache directory", "path", cfg.CacheDir)
			if err := os.MkdirAll(cfg.CacheDir, 0755); err != nil {
				return fmt.Errorf("failed to create cache directory: %w", err)
			}
		}
	}

	// Create state file directory if it doesn't exist
	if cfg.StateFilePath != "" {
		stateDir := filepath.Dir(cfg.StateFilePath)
		if _, err := os.Stat(stateDir); os.IsNotExist(err) {
			v.logger.Info("Creating state file directory", "path", stateDir)
			if err := os.MkdirAll(stateDir, 0755); err != nil {
				return fmt.Errorf("failed to create state file directory: %w", err)
			}
		}
	}

	// Create report output directory if it doesn't exist
	if cfg.ReportOutputPath != "" {
		reportDir := filepath.Dir(cfg.ReportOutputPath)
		if _, err := os.Stat(reportDir); os.IsNotExist(err) {
			v.logger.Info("Creating report output directory", "path", reportDir)
			if err := os.MkdirAll(reportDir, 0755); err != nil {
				return fmt.Errorf("failed to create report output directory: %w", err)
			}
		}
	}

	v.logger.Info("Configuration validation successful")
	return nil
}

// ValidateStateFile validates that the state file exists and can be read
func (v *Validator) ValidateStateFile(path string) (bool, error) {
	v.logger.Info("Validating state file", "path", path)

	// Check if state file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		v.logger.Info("State file does not exist", "path", path)
		return false, nil
	}

	// Check if state file can be read
	if _, err := os.ReadFile(path); err != nil {
		return false, fmt.Errorf("failed to read state file: %w", err)
	}

	v.logger.Info("State file validation successful")
	return true, nil
}
