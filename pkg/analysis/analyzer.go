package analysis

import (
	"fmt"
	"strings"
	"time"

	"github.com/oetiker/response-analyzer/pkg/claude"
	"github.com/oetiker/response-analyzer/pkg/excel"
	"github.com/oetiker/response-analyzer/pkg/logging"
)

// ResponseAnalysis represents the analysis of a response
type ResponseAnalysis struct {
	Response excel.Response `yaml:"response"`
	Themes   []string       `yaml:"themes,omitempty"`
	Analyzed time.Time      `yaml:"analyzed"`
}

// ThemeAnalysis represents the analysis of a theme
type ThemeAnalysis struct {
	Theme     string   `yaml:"theme"`
	Responses []string `yaml:"response_ids,omitempty"`
}

// AnalysisResult represents the result of the analysis
type AnalysisResult struct {
	Themes            []string                       `yaml:"themes"`
	ResponseAnalyses  map[string]ResponseAnalysis    `yaml:"response_analyses"`
	ThemeAnalyses     map[string]ThemeAnalysis       `yaml:"theme_analyses"`
	ThemeSummaries    map[string]claude.ThemeSummary `yaml:"theme_summaries,omitempty"`
	Summary           string                         `yaml:"summary,omitempty"`        // Global summary (for backward compatibility)
	GlobalSummary     string                         `yaml:"global_summary,omitempty"` // Same as Summary, new name for clarity
	UniqueIdeas       []string                       `yaml:"unique_ideas,omitempty"`   // Kept for backward compatibility
	AnalysisTimestamp time.Time                      `yaml:"analysis_timestamp"`
}

// Analyzer handles the analysis of responses
type Analyzer struct {
	logger       *logging.Logger
	claudeClient *claude.Client
}

// NewAnalyzer creates a new Analyzer instance
func NewAnalyzer(logger *logging.Logger, claudeClient *claude.Client) *Analyzer {
	return &Analyzer{
		logger:       logger,
		claudeClient: claudeClient,
	}
}

// IdentifyThemes identifies themes in responses
func (a *Analyzer) IdentifyThemes(responses []excel.Response, contextPrompt string) ([]string, error) {
	a.logger.Info("Identifying themes in responses", "count", len(responses))

	// Extract response texts
	var responseTexts []string
	for _, response := range responses {
		responseTexts = append(responseTexts, response.Text)
	}

	// Identify themes using Claude API
	themes, err := a.claudeClient.IdentifyThemes(responseTexts, contextPrompt)
	if err != nil {
		return nil, fmt.Errorf("failed to identify themes: %w", err)
	}

	a.logger.Info("Identified themes", "count", len(themes))
	return themes, nil
}

// MatchResponsesToThemes matches responses to themes
func (a *Analyzer) MatchResponsesToThemes(responses []excel.Response, themes []string, contextPrompt string, previousAnalyses map[string]ResponseAnalysis) (map[string]ResponseAnalysis, error) {
	a.logger.Info("Matching responses to themes", "responses", len(responses), "themes", len(themes))

	// Initialize result
	result := make(map[string]ResponseAnalysis)

	// Reuse previous analyses for unchanged responses
	newResponses := []excel.Response{}
	for _, response := range responses {
		if previousAnalysis, ok := previousAnalyses[response.ID]; ok && previousAnalysis.Response.Hash == response.Hash {
			// Response hasn't changed, reuse previous analysis
			a.logger.Debug("Reusing previous analysis", "response_id", response.ID)
			result[response.ID] = previousAnalysis
		} else {
			// Response is new or has changed, analyze it
			newResponses = append(newResponses, response)
		}
	}

	a.logger.Info("New or changed responses", "count", len(newResponses))

	// Match new responses to themes
	for _, response := range newResponses {
		a.logger.Debug("Matching response to themes", "response_id", response.ID)

		// Match response to themes using Claude API
		matchedThemes, err := a.claudeClient.MatchResponsesToThemes(response.Text, themes, contextPrompt)
		if err != nil {
			return nil, fmt.Errorf("failed to match response to themes: %w", err)
		}

		// Create response analysis
		analysis := ResponseAnalysis{
			Response: response,
			Themes:   matchedThemes,
			Analyzed: time.Now(),
		}

		// Add to result
		result[response.ID] = analysis
	}

	a.logger.Info("Matched responses to themes", "count", len(result))
	return result, nil
}

// BuildThemeAnalyses builds theme analyses from response analyses
func (a *Analyzer) BuildThemeAnalyses(responseAnalyses map[string]ResponseAnalysis, themes []string) map[string]ThemeAnalysis {
	a.logger.Info("Building theme analyses", "themes", len(themes))

	// Initialize result
	result := make(map[string]ThemeAnalysis)

	// Initialize theme analyses
	for _, theme := range themes {
		result[theme] = ThemeAnalysis{
			Theme:     theme,
			Responses: []string{},
		}
	}

	// Add responses to themes
	for responseID, analysis := range responseAnalyses {
		for _, theme := range analysis.Themes {
			if themeAnalysis, ok := result[theme]; ok {
				themeAnalysis.Responses = append(themeAnalysis.Responses, responseID)
				result[theme] = themeAnalysis
			}
		}
	}

	a.logger.Info("Built theme analyses", "count", len(result))
	return result
}

// GenerateThemeSummaries generates summaries for each theme and extracts unique ideas
func (a *Analyzer) GenerateThemeSummaries(responseAnalyses map[string]ResponseAnalysis, themeAnalyses map[string]ThemeAnalysis, themeSummaryPrompt string) (map[string]claude.ThemeSummary, error) {
	a.logger.Info("Generating theme summaries")

	// Initialize result
	result := make(map[string]claude.ThemeSummary)

	// Process each theme
	for theme, analysis := range themeAnalyses {
		// Skip themes with no responses
		if len(analysis.Responses) == 0 {
			continue
		}

		// Get response texts for this theme
		var responses []string
		for _, responseID := range analysis.Responses {
			if responseAnalysis, ok := responseAnalyses[responseID]; ok {
				responses = append(responses, responseAnalysis.Response.Text)
			}
		}

		// Generate theme summary using Claude API
		a.logger.Debug("Generating summary for theme", "theme", theme, "responses", len(responses))
		themeSummaryResponse, err := a.claudeClient.GenerateThemeSummary(theme, responses, themeSummaryPrompt)
		if err != nil {
			return nil, fmt.Errorf("failed to generate summary for theme %s: %w", theme, err)
		}

		// Extract summary and unique ideas
		summary, uniqueIdeas := extractSummaryAndIdeas(themeSummaryResponse)

		// Create theme summary
		themeSummary := claude.ThemeSummary{
			Summary:     summary,
			UniqueIdeas: uniqueIdeas,
		}

		// Add to result
		result[theme] = themeSummary
	}

	a.logger.Info("Generated theme summaries", "count", len(result))
	return result, nil
}

// GenerateGlobalSummary generates a global summary based on theme summaries
func (a *Analyzer) GenerateGlobalSummary(themeSummaries map[string]claude.ThemeSummary, globalSummaryPrompt string, summaryLength int) (string, error) {
	a.logger.Info("Generating global summary")

	// Generate global summary using Claude API
	summary, err := a.claudeClient.GenerateGlobalSummary(themeSummaries, globalSummaryPrompt, summaryLength)
	if err != nil {
		return "", fmt.Errorf("failed to generate global summary: %w", err)
	}

	a.logger.Info("Generated global summary", "length", len(summary))
	return summary, nil
}

// GenerateSummary generates a summary of the analysis (for backward compatibility)
func (a *Analyzer) GenerateSummary(responseAnalyses map[string]ResponseAnalysis, themeAnalyses map[string]ThemeAnalysis, summaryPrompt string, summaryLength int) (string, error) {
	a.logger.Info("Generating summary")

	// Build map of themes to response texts
	themeResponses := make(map[string][]string)
	for theme, analysis := range themeAnalyses {
		var responses []string
		for _, responseID := range analysis.Responses {
			if responseAnalysis, ok := responseAnalyses[responseID]; ok {
				responses = append(responses, responseAnalysis.Response.Text)
			}
		}
		themeResponses[theme] = responses
	}

	// Generate summary using Claude API
	summary, err := a.claudeClient.GenerateSummary(themeResponses, summaryPrompt, summaryLength)
	if err != nil {
		return "", fmt.Errorf("failed to generate summary: %w", err)
	}

	a.logger.Info("Generated summary", "length", len(summary))
	return summary, nil
}

// extractSummaryAndIdeas extracts the summary and unique ideas from a theme summary response
func extractSummaryAndIdeas(response string) (string, []string) {
	// Split by the marker
	parts := strings.Split(response, "UNIQUE IDEAS:")
	if len(parts) < 2 {
		// No ideas section found, return the whole response as summary
		return strings.TrimSpace(response), []string{}
	}

	// Get the summary section
	summary := strings.TrimSpace(parts[0])
	if strings.HasPrefix(summary, "SUMMARY:") {
		summary = strings.TrimSpace(summary[8:])
	}

	// Get the ideas section
	ideasSection := parts[1]

	// Split by newlines
	lines := strings.Split(ideasSection, "\n")

	// Extract ideas
	var ideas []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "IDEA: ") {
			// Remove the prefix and add to ideas list
			idea := strings.TrimPrefix(line, "IDEA: ")
			if idea != "" {
				ideas = append(ideas, idea)
			}
		} else if strings.HasPrefix(line, "- ") {
			// Alternative format: bullet points
			idea := strings.TrimPrefix(line, "- ")
			if idea != "" {
				ideas = append(ideas, idea)
			}
		}
	}

	return summary, ideas
}

// AnalyzeResponses analyzes responses
func (a *Analyzer) AnalyzeResponses(responses []excel.Response, themes []string, contextPrompt string, summaryPrompt string, themeSummaryPrompt string, globalSummaryPrompt string, summaryLength int, previousResult *AnalysisResult) (*AnalysisResult, error) {
	a.logger.Info("Analyzing responses", "count", len(responses))

	// Initialize result
	result := &AnalysisResult{
		Themes:            themes,
		ResponseAnalyses:  make(map[string]ResponseAnalysis),
		ThemeAnalyses:     make(map[string]ThemeAnalysis),
		AnalysisTimestamp: time.Now(),
	}

	// If no themes provided, identify them
	if len(themes) == 0 {
		var err error
		result.Themes, err = a.IdentifyThemes(responses, contextPrompt)
		if err != nil {
			return nil, fmt.Errorf("failed to identify themes: %w", err)
		}
	}

	// Get previous response analyses if available
	previousAnalyses := make(map[string]ResponseAnalysis)
	if previousResult != nil {
		previousAnalyses = previousResult.ResponseAnalyses
	}

	// Match responses to themes
	var err error
	result.ResponseAnalyses, err = a.MatchResponsesToThemes(responses, result.Themes, contextPrompt, previousAnalyses)
	if err != nil {
		return nil, fmt.Errorf("failed to match responses to themes: %w", err)
	}

	// Build theme analyses
	result.ThemeAnalyses = a.BuildThemeAnalyses(result.ResponseAnalyses, result.Themes)

	// Generate theme summaries if themes are provided and theme summary prompt is provided
	if len(result.Themes) > 0 && themeSummaryPrompt != "" {
		result.ThemeSummaries, err = a.GenerateThemeSummaries(result.ResponseAnalyses, result.ThemeAnalyses, themeSummaryPrompt)
		if err != nil {
			return nil, fmt.Errorf("failed to generate theme summaries: %w", err)
		}
	}

	// Generate global summary if themes are provided and global summary prompt is provided
	if len(result.Themes) > 0 && globalSummaryPrompt != "" && summaryLength > 0 {
		result.GlobalSummary, err = a.GenerateGlobalSummary(result.ThemeSummaries, globalSummaryPrompt, summaryLength)
		if err != nil {
			return nil, fmt.Errorf("failed to generate global summary: %w", err)
		}
		// Set Summary to the same value for backward compatibility
		result.Summary = result.GlobalSummary
	} else if len(result.Themes) > 0 && summaryLength > 0 {
		// Fallback to old summary generation for backward compatibility
		result.Summary, err = a.GenerateSummary(result.ResponseAnalyses, result.ThemeAnalyses, summaryPrompt, summaryLength)
		if err != nil {
			return nil, fmt.Errorf("failed to generate summary: %w", err)
		}
		// Set GlobalSummary to the same value for backward compatibility
		result.GlobalSummary = result.Summary
	}

	a.logger.Info("Analysis completed",
		"themes", len(result.Themes),
		"responses", len(result.ResponseAnalyses),
		"summary_length", len(result.Summary))

	return result, nil
}
