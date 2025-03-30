package template

import (
	"fmt"
	"os"
	"sort"
	"text/template"
	"time"

	"github.com/oetiker/response-analyzer/pkg/analysis"
	"github.com/oetiker/response-analyzer/pkg/claude"
	"github.com/oetiker/response-analyzer/pkg/logging"
)

// ThemeStat represents statistics for a theme
type ThemeStat struct {
	Theme      string  `yaml:"theme"`
	Count      int     `yaml:"count"`
	Percentage float64 `yaml:"percentage"`
}

// TemplateData represents the data available in templates
type TemplateData struct {
	Themes         []string
	ThemeStats     []ThemeStat
	ThemeSummaries map[string]claude.ThemeSummary
	Summary        string
	GlobalSummary  string
	Responses      []ResponseData
	ResponseCount  int
	AnalysisDate   time.Time
	ColumnTitle    string
}

// ResponseData represents a response in the template data
type ResponseData struct {
	ID       string
	Text     string
	Themes   []string
	RowIndex int
}

// Renderer handles rendering templates
type Renderer struct {
	logger *logging.Logger
}

// NewRenderer creates a new Renderer instance
func NewRenderer(logger *logging.Logger) *Renderer {
	return &Renderer{
		logger: logger,
	}
}

// RenderTemplate renders a template with the given data
func (r *Renderer) RenderTemplate(templatePath, outputPath string, result *analysis.AnalysisResult) error {
	r.logger.Info("Rendering template", "template", templatePath, "output", outputPath)

	// Set a default value for ColumnTitle if it's empty
	if result.ColumnTitle == "" {
		result.ColumnTitle = "Survey Responses"
	}

	// Read template file
	tmplContent, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("failed to read template file: %w", err)
	}

	// Parse template
	tmpl, err := template.New("report").Parse(string(tmplContent))
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	// Prepare template data
	data, err := r.prepareTemplateData(result)
	if err != nil {
		return fmt.Errorf("failed to prepare template data: %w", err)
	}

	// Create output file
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	// Execute template
	if err := tmpl.Execute(file, data); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	r.logger.Info("Template rendered", "output", outputPath)
	return nil
}

// prepareTemplateData prepares the data for the template
func (r *Renderer) prepareTemplateData(result *analysis.AnalysisResult) (*TemplateData, error) {
	// Create theme stats
	themeStats := make([]ThemeStat, 0, len(result.ThemeAnalyses))
	totalResponses := len(result.ResponseAnalyses)

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

	// Sort theme stats by count in descending order
	sort.Slice(themeStats, func(i, j int) bool {
		return themeStats[i].Count > themeStats[j].Count
	})

	// Create response data
	responses := make([]ResponseData, 0, len(result.ResponseAnalyses))
	for _, responseAnalysis := range result.ResponseAnalyses {
		response := ResponseData{
			ID:       responseAnalysis.Response.ID,
			Text:     responseAnalysis.Response.Text,
			Themes:   responseAnalysis.Themes,
			RowIndex: responseAnalysis.Response.RowIndex,
		}
		responses = append(responses, response)
	}

	// Create template data
	data := &TemplateData{
		Themes:         result.Themes,
		ThemeStats:     themeStats,
		ThemeSummaries: result.ThemeSummaries,
		Summary:        result.Summary,
		GlobalSummary:  result.GlobalSummary,
		Responses:      responses,
		ResponseCount:  totalResponses,
		AnalysisDate:   result.AnalysisTimestamp,
		ColumnTitle:    result.ColumnTitle,
	}

	// If ColumnTitle is empty, use a default value
	if data.ColumnTitle == "" {
		data.ColumnTitle = "Survey Responses"
	}

	return data, nil
}
