package analysis

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/oetiker/response-analyzer/pkg/claude"
	"github.com/oetiker/response-analyzer/pkg/config"
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
	ColumnTitle       string                         `yaml:"column_title,omitempty"` // Title of the column containing responses
}

// Analyzer handles the analysis of responses
type Analyzer struct {
	logger          *logging.Logger
	claudeClient    *claude.Client
	batchSize       int
	parallelWorkers int
	useParallel     bool
}

// NewAnalyzer creates a new Analyzer instance
func NewAnalyzer(logger *logging.Logger, claudeClient *claude.Client) *Analyzer {
	return &Analyzer{
		logger:          logger,
		claudeClient:    claudeClient,
		batchSize:       10,   // Default batch size
		parallelWorkers: 4,    // Default number of workers
		useParallel:     true, // Default to using parallel processing
	}
}

// SetBatchSize sets the batch size for processing responses
func (a *Analyzer) SetBatchSize(batchSize int) {
	if batchSize > 0 {
		a.batchSize = batchSize
	}
}

// SetParallelWorkers sets the number of parallel workers
func (a *Analyzer) SetParallelWorkers(workers int) {
	if workers > 0 {
		a.parallelWorkers = workers
	}
}

// SetUseParallel sets whether to use parallel processing
func (a *Analyzer) SetUseParallel(useParallel bool) {
	a.useParallel = useParallel
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

	// If no new responses, return early
	if len(newResponses) == 0 {
		return result, nil
	}

	// Prepare batch processing
	responseTexts := make([]string, len(newResponses))
	for i, response := range newResponses {
		responseTexts[i] = response.Text
	}

	// Use configured batch size or determine optimal batch size
	batchSize := a.batchSize
	if batchSize <= 0 {
		batchSize = 10
		if len(newResponses) > 100 {
			batchSize = 20
		} else if len(newResponses) < 10 {
			batchSize = len(newResponses)
		}
	}

	// Match responses to themes in batches
	matchedThemesBatch, err := a.claudeClient.MatchResponsesToThemesBatch(responseTexts, themes, contextPrompt, batchSize)
	if err != nil {
		return nil, fmt.Errorf("failed to match responses to themes in batch: %w", err)
	}

	// Create response analyses from batch results
	for i, response := range newResponses {
		var matchedThemes []string
		if i < len(matchedThemesBatch) {
			matchedThemes = matchedThemesBatch[i]
		} else {
			matchedThemes = []string{}
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

// MatchResponsesToThemesParallel matches responses to themes in parallel
func (a *Analyzer) MatchResponsesToThemesParallel(responses []excel.Response, themes []string, contextPrompt string, previousAnalyses map[string]ResponseAnalysis, batchSize int, numWorkers int) (map[string]ResponseAnalysis, error) {
	a.logger.Info("Matching responses to themes in parallel", "responses", len(responses), "themes", len(themes))

	// Initialize result
	result := make(map[string]ResponseAnalysis)
	var resultMutex sync.Mutex

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

	// If no new responses, return early
	if len(newResponses) == 0 {
		return result, nil
	}

	// Use provided batch size or determine optimal batch size
	if batchSize <= 0 {
		batchSize = 10
		if len(newResponses) > 100 {
			batchSize = 20
		} else if len(newResponses) < 10 {
			batchSize = len(newResponses)
		}
	}

	// Use provided number of workers or determine optimal number
	if numWorkers <= 0 {
		numWorkers = 4 // Default number of workers
		if len(newResponses) < numWorkers*batchSize {
			numWorkers = (len(newResponses) + batchSize - 1) / batchSize // Ceiling division
		}
	}

	// Create batches
	batches := make([][]excel.Response, 0)
	for i := 0; i < len(newResponses); i += batchSize {
		end := i + batchSize
		if end > len(newResponses) {
			end = len(newResponses)
		}
		batches = append(batches, newResponses[i:end])
	}

	// Process batches in parallel
	var wg sync.WaitGroup
	errorsChan := make(chan error, len(batches))
	semaphore := make(chan struct{}, numWorkers) // Limit concurrent workers

	for batchIndex, batch := range batches {
		wg.Add(1)

		go func(index int, batchResponses []excel.Response) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			a.logger.Debug("Processing batch", "batch", index, "size", len(batchResponses))

			// Extract response texts
			responseTexts := make([]string, len(batchResponses))
			for i, response := range batchResponses {
				responseTexts[i] = response.Text
			}

			// Match batch to themes
			matchedThemesBatch, err := a.claudeClient.MatchResponsesToThemesBatch(responseTexts, themes, contextPrompt, len(batchResponses))
			if err != nil {
				errorsChan <- fmt.Errorf("failed to process batch %d: %w", index, err)
				return
			}

			// Create response analyses from batch results
			batchResults := make(map[string]ResponseAnalysis)
			for i, response := range batchResponses {
				var matchedThemes []string
				if i < len(matchedThemesBatch) {
					matchedThemes = matchedThemesBatch[i]
				} else {
					matchedThemes = []string{}
				}

				// Create response analysis
				analysis := ResponseAnalysis{
					Response: response,
					Themes:   matchedThemes,
					Analyzed: time.Now(),
				}

				batchResults[response.ID] = analysis
			}

			// Add batch results to the main result
			resultMutex.Lock()
			for id, analysis := range batchResults {
				result[id] = analysis
			}
			resultMutex.Unlock()

			a.logger.Debug("Batch processed", "batch", index, "size", len(batchResponses))
		}(batchIndex, batch)
	}

	// Wait for all batches to complete
	wg.Wait()
	close(errorsChan)

	// Check for errors
	if len(errorsChan) > 0 {
		var errMsgs []string
		for err := range errorsChan {
			errMsgs = append(errMsgs, err.Error())
		}
		return nil, fmt.Errorf("errors occurred during parallel processing: %s", strings.Join(errMsgs, "; "))
	}

	a.logger.Info("Matched responses to themes in parallel", "count", len(result))
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
	// Initialize with empty slice to avoid nil
	ideas := []string{}

	// Clean up the response by removing any # symbols that might be present
	response = strings.ReplaceAll(response, "# SUMMARY:", "SUMMARY:")
	response = strings.ReplaceAll(response, "#SUMMARY:", "SUMMARY:")
	response = strings.ReplaceAll(response, "# UNIQUE IDEAS:", "UNIQUE IDEAS:")
	response = strings.ReplaceAll(response, "#UNIQUE IDEAS:", "UNIQUE IDEAS:")
	response = strings.ReplaceAll(response, "#", "")

	// Split by the marker
	parts := strings.Split(response, "UNIQUE IDEAS:")
	if len(parts) < 2 {
		// No ideas section found, return the whole response as summary
		return strings.TrimSpace(response), ideas
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

// IdentifyThemesOnly identifies themes in responses without performing full analysis
func (a *Analyzer) IdentifyThemesOnly(responses []excel.Response, contextPrompt string) ([]string, error) {
	a.logger.Info("Identifying themes only (without full analysis)")
	return a.IdentifyThemes(responses, contextPrompt)
}

// AnalyzeResponses analyzes responses using the provided configuration
func (a *Analyzer) AnalyzeResponses(responses []excel.Response, cfg *config.Config, previousResult *AnalysisResult, columnTitle string) (*AnalysisResult, error) {
	a.logger.Info("Analyzing responses", "count", len(responses))

	// Initialize result
	result := &AnalysisResult{
		Themes:            cfg.Themes,
		ResponseAnalyses:  make(map[string]ResponseAnalysis),
		ThemeAnalyses:     make(map[string]ThemeAnalysis),
		AnalysisTimestamp: time.Now(),
		ColumnTitle:       columnTitle,
	}

	// If no themes provided, identify them
	if len(result.Themes) == 0 {
		var err error
		result.Themes, err = a.IdentifyThemes(responses, cfg.ContextPrompt)
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
	if a.useParallel {
		// Use parallel processing
		result.ResponseAnalyses, err = a.MatchResponsesToThemesParallel(responses, result.Themes, cfg.ContextPrompt, previousAnalyses, a.batchSize, a.parallelWorkers)
		if err != nil {
			return nil, fmt.Errorf("failed to match responses to themes in parallel: %w", err)
		}
	} else {
		// Use batch processing
		result.ResponseAnalyses, err = a.MatchResponsesToThemes(responses, result.Themes, cfg.ContextPrompt, previousAnalyses)
		if err != nil {
			return nil, fmt.Errorf("failed to match responses to themes: %w", err)
		}
	}

	// Build theme analyses
	result.ThemeAnalyses = a.BuildThemeAnalyses(result.ResponseAnalyses, result.Themes)

	// Check if any responses have changed
	responsesChanged := len(previousAnalyses) != len(result.ResponseAnalyses)
	if !responsesChanged {
		for id, analysis := range result.ResponseAnalyses {
			if prevAnalysis, ok := previousAnalyses[id]; !ok || prevAnalysis.Response.Hash != analysis.Response.Hash {
				responsesChanged = true
				break
			}
		}
	}

	// If no responses have changed and previous result has theme summaries, reuse them
	if !responsesChanged && previousResult != nil && len(previousResult.ThemeSummaries) > 0 {
		a.logger.Info("Reusing theme summaries from previous result", "count", len(previousResult.ThemeSummaries))
		result.ThemeSummaries = previousResult.ThemeSummaries
		result.GlobalSummary = previousResult.GlobalSummary
		result.Summary = previousResult.Summary
	} else {
		// Generate theme summaries if themes are provided and theme summary prompt is provided
		if len(result.Themes) > 0 && cfg.ThemeSummaryPrompt != "" {
			result.ThemeSummaries, err = a.GenerateThemeSummaries(result.ResponseAnalyses, result.ThemeAnalyses, cfg.ThemeSummaryPrompt)
			if err != nil {
				return nil, fmt.Errorf("failed to generate theme summaries: %w", err)
			}
		}

		// Generate global summary if themes are provided and global summary prompt is provided
		if len(result.Themes) > 0 && cfg.GlobalSummaryPrompt != "" && cfg.SummaryLength > 0 {
			result.GlobalSummary, err = a.GenerateGlobalSummary(result.ThemeSummaries, cfg.GlobalSummaryPrompt, cfg.SummaryLength)
			if err != nil {
				return nil, fmt.Errorf("failed to generate global summary: %w", err)
			}
			// Set Summary to the same value for backward compatibility
			result.Summary = result.GlobalSummary
		} else if len(result.Themes) > 0 && cfg.SummaryLength > 0 {
			// Use a default global summary prompt if none is provided
			defaultGlobalPrompt := "Summarize the main points made in each theme and highlight any unique ideas or problems mentioned."
			result.GlobalSummary, err = a.GenerateGlobalSummary(result.ThemeSummaries, defaultGlobalPrompt, cfg.SummaryLength)
			if err != nil {
				return nil, fmt.Errorf("failed to generate global summary: %w", err)
			}
			// Set Summary to the same value for backward compatibility
			result.Summary = result.GlobalSummary
		}
	}

	a.logger.Info("Analysis completed",
		"themes", len(result.Themes),
		"responses", len(result.ResponseAnalyses),
		"global_summary_length", len(result.GlobalSummary))

	return result, nil
}
