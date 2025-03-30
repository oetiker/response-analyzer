package output

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/oetiker/response-analyzer/pkg/analysis"
	"github.com/oetiker/response-analyzer/pkg/logging"
	"github.com/oetiker/response-analyzer/pkg/template"
	"gopkg.in/yaml.v3"
)

// Writer handles writing output files
type Writer struct {
	logger   *logging.Logger
	renderer *template.Renderer
}

// NewWriter creates a new Writer instance
func NewWriter(logger *logging.Logger) *Writer {
	return &Writer{
		logger:   logger,
		renderer: template.NewRenderer(logger),
	}
}

// SaveState saves the analysis result to a state file
func (w *Writer) SaveState(result *analysis.AnalysisResult, path string) error {
	w.logger.Info("Saving state to file", "path", path)

	// Marshal result to YAML
	data, err := yaml.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	// Write to file
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	w.logger.Info("State saved to file", "path", path)
	return nil
}

// LoadState loads the analysis result from a state file
func (w *Writer) LoadState(path string) (*analysis.AnalysisResult, error) {
	w.logger.Info("Loading state from file", "path", path)

	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	// Unmarshal result
	var result analysis.AnalysisResult
	if err := yaml.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal result: %w", err)
	}

	w.logger.Info("State loaded from file", "path", path)
	return &result, nil
}

// SaveThemes saves the themes to a YAML file
func (w *Writer) SaveThemes(themes []string, path string) error {
	w.logger.Info("Saving themes to file", "path", path, "count", len(themes))

	// Create themes map
	themesMap := map[string][]string{
		"themes": themes,
	}

	// Marshal themes to YAML
	data, err := yaml.Marshal(themesMap)
	if err != nil {
		return fmt.Errorf("failed to marshal themes: %w", err)
	}

	// Write to file
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write themes file: %w", err)
	}

	w.logger.Info("Themes saved to file", "path", path)
	return nil
}

// SaveSummary saves the summary to a file
func (w *Writer) SaveSummary(summary string, path string) error {
	w.logger.Info("Saving summary to file", "path", path)

	// Write to file
	if err := os.WriteFile(path, []byte(summary), 0644); err != nil {
		return fmt.Errorf("failed to write summary file: %w", err)
	}

	w.logger.Info("Summary saved to file", "path", path)
	return nil
}

// SaveAuditLog saves the audit log to a YAML file
func (w *Writer) SaveAuditLog(result *analysis.AnalysisResult, path string) error {
	w.logger.Info("Saving audit log to file", "path", path)

	// Create audit log
	type ResponseAudit struct {
		ID       string   `yaml:"id"`
		Text     string   `yaml:"text"`
		Themes   []string `yaml:"themes"`
		RowIndex int      `yaml:"row_index"`
	}

	auditLog := make([]ResponseAudit, 0, len(result.ResponseAnalyses))
	for _, responseAnalysis := range result.ResponseAnalyses {
		audit := ResponseAudit{
			ID:       responseAnalysis.Response.ID,
			Text:     responseAnalysis.Response.Text,
			Themes:   responseAnalysis.Themes,
			RowIndex: responseAnalysis.Response.RowIndex,
		}
		auditLog = append(auditLog, audit)
	}

	// Marshal audit log to YAML
	data, err := yaml.Marshal(auditLog)
	if err != nil {
		return fmt.Errorf("failed to marshal audit log: %w", err)
	}

	// Write to file
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write audit log file: %w", err)
	}

	w.logger.Info("Audit log saved to file", "path", path)
	return nil
}

// SaveThemeStats saves theme statistics to a YAML file
func (w *Writer) SaveThemeStats(result *analysis.AnalysisResult, path string) error {
	w.logger.Info("Saving theme statistics to file", "path", path)

	// Create theme stats
	type ThemeStat struct {
		Theme      string  `yaml:"theme"`
		Count      int     `yaml:"count"`
		Percentage float64 `yaml:"percentage"`
	}

	totalResponses := len(result.ResponseAnalyses)
	themeStats := make([]ThemeStat, 0, len(result.ThemeAnalyses))

	for _, themeAnalysis := range result.ThemeAnalyses {
		count := len(themeAnalysis.Responses)
		percentage := 0.0
		if totalResponses > 0 {
			percentage = float64(count) / float64(totalResponses) * 100.0
		}

		stat := ThemeStat{
			Theme:      themeAnalysis.Theme,
			Count:      count,
			Percentage: percentage,
		}
		themeStats = append(themeStats, stat)
	}

	// Marshal theme stats to YAML
	data, err := yaml.Marshal(themeStats)
	if err != nil {
		return fmt.Errorf("failed to marshal theme stats: %w", err)
	}

	// Write to file
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write theme stats file: %w", err)
	}

	w.logger.Info("Theme statistics saved to file", "path", path)
	return nil
}

// GenerateReport generates a report using a template
func (w *Writer) GenerateReport(result *analysis.AnalysisResult, templatePath, outputPath string) error {
	w.logger.Info("Generating report", "template", templatePath, "output", outputPath)

	// Create output directory if it doesn't exist
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Render template
	if err := w.renderer.RenderTemplate(templatePath, outputPath, result); err != nil {
		return fmt.Errorf("failed to render template: %w", err)
	}

	w.logger.Info("Report generated", "path", outputPath)
	return nil
}
