package claude

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/oetiker/response-analyzer/pkg/cache"
	"github.com/oetiker/response-analyzer/pkg/logging"
)

// ThemeSummary represents a summary of a theme
type ThemeSummary struct {
	Summary     string   `json:"summary"`
	UniqueIdeas []string `json:"unique_ideas,omitempty"`
}

const (
	// ClaudeAPIURL is the base URL for the Claude API
	ClaudeAPIURL = "https://api.anthropic.com/v1/messages"
	// DefaultModel is the default Claude model to use
	DefaultModel = "claude-3-opus-20240229"
	// DefaultTimeout is the default timeout for API requests
	DefaultTimeout = 60 * time.Second
	// DefaultMaxTokens is the default maximum number of tokens to generate
	DefaultMaxTokens = 4096
	// DefaultRateLimitDelay is the default delay between API calls to avoid rate limiting
	DefaultRateLimitDelay = 1 * time.Second
)

// Message represents a message in the Claude API
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// RequestBody represents the request body for the Claude API
type RequestBody struct {
	Model       string    `json:"model"`
	MaxTokens   int       `json:"max_tokens"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	System      string    `json:"system,omitempty"`
}

// ResponseBody represents the response body from the Claude API
type ResponseBody struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Role         string         `json:"role"`
	Content      []ContentBlock `json:"content"`
	Model        string         `json:"model"`
	StopReason   string         `json:"stop_reason"`
	StopSequence string         `json:"stop_sequence"`
	Usage        struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// ContentBlock represents a block of content in the Claude API response
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Cost represents the cost of a Claude API call
type Cost struct {
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	TotalTokens  int     `json:"total_tokens"`
	Cost         float64 `json:"cost"`
}

// Client is a client for the Claude API
type Client struct {
	apiKey         string
	model          string
	httpClient     *http.Client
	logger         *logging.Logger
	cache          *cache.Cache
	outputLanguage string
	totalCost      float64
	totalTokens    int
	rateLimitDelay time.Duration // Delay between API calls to avoid rate limiting
}

// ModelCostPerMillionTokens returns the cost per million tokens for a given model
func ModelCostPerMillionTokens(model string) (inputCost, outputCost float64) {
	switch model {
	case "claude-3-opus-20240229":
		return 15.0, 75.0
	case "claude-3-sonnet-20240229":
		return 3.0, 15.0
	case "claude-3-haiku-20240307":
		return 0.25, 1.25
	case "claude-3-7-sonnet-20250219":
		return 3.0, 15.0
	case "claude-2.1":
		return 8.0, 24.0
	case "claude-2.0":
		return 8.0, 24.0
	default:
		// Default to opus pricing
		return 15.0, 75.0
	}
}

// CalculateCost calculates the cost of a Claude API call
func CalculateCost(model string, inputTokens, outputTokens int) Cost {
	inputCostPerMillion, outputCostPerMillion := ModelCostPerMillionTokens(model)

	inputCost := float64(inputTokens) * inputCostPerMillion / 1000000
	outputCost := float64(outputTokens) * outputCostPerMillion / 1000000
	totalCost := inputCost + outputCost
	totalTokens := inputTokens + outputTokens

	return Cost{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		TotalTokens:  totalTokens,
		Cost:         totalCost,
	}
}

// GetTotalCost returns the total cost of all Claude API calls
func (c *Client) GetTotalCost() float64 {
	return c.totalCost
}

// GetTotalTokens returns the total number of tokens used
func (c *Client) GetTotalTokens() int {
	return c.totalTokens
}

// NewClient creates a new Claude API client
func NewClient(apiKey string, logger *logging.Logger, cache *cache.Cache, outputLanguage string, model string) *Client {
	// Use provided model or default
	if model == "" {
		model = DefaultModel
	}

	return &Client{
		apiKey: apiKey,
		model:  model,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
		logger:         logger,
		cache:          cache,
		outputLanguage: outputLanguage,
		totalCost:      0.0,
		totalTokens:    0,
		rateLimitDelay: DefaultRateLimitDelay,
	}
}

// SetRateLimitDelay sets the delay between API calls to avoid rate limiting
func (c *Client) SetRateLimitDelay(delay time.Duration) {
	c.rateLimitDelay = delay
}

// SetModel sets the model to use for API requests
func (c *Client) SetModel(model string) {
	c.model = model
}

// GetCompletion gets a completion from the Claude API
func (c *Client) GetCompletion(prompt string, systemPrompt string, maxTokens int) (string, error) {
	// Check cache first
	cacheKey := fmt.Sprintf("%s:%s:%d:%s", c.model, systemPrompt, maxTokens, prompt)
	if c.cache != nil {
		if cachedResponse, found := c.cache.Get(cacheKey); found {
			c.logger.Info("Using cached response")
			return cachedResponse, nil
		}
	}

	// Log the request details
	c.logger.Info("Sending request to Claude API",
		"model", c.model,
		"prompt_length", len(prompt),
		"system_prompt_length", len(systemPrompt),
		"max_tokens", maxTokens)

	// Apply rate limiting delay if set
	if c.rateLimitDelay > 0 {
		c.logger.Debug("Applying rate limit delay", "delay", c.rateLimitDelay)
		time.Sleep(c.rateLimitDelay)
	}

	// Create request body
	reqBody := RequestBody{
		Model:     c.model,
		MaxTokens: maxTokens,
		Messages: []Message{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: 0.7,
	}

	// Add system prompt if provided
	if systemPrompt != "" {
		reqBody.System = systemPrompt
	}

	// Marshal request body
	reqData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Create request
	req, err := http.NewRequest("POST", ClaudeAPIURL, bytes.NewBuffer(reqData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	// Maximum number of retries for rate limit errors
	maxRetries := 3
	baseDelay := c.rateLimitDelay

	// Retry loop with exponential backoff
	for retry := 0; retry <= maxRetries; retry++ {
		// Send request
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("failed to send request: %w", err)
		}

		// Read response body
		respData, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			return "", fmt.Errorf("failed to read response body: %w", err)
		}

		// Check response status
		if resp.StatusCode == http.StatusOK {
			// Success, process the response
			var respBody ResponseBody
			if err := json.Unmarshal(respData, &respBody); err != nil {
				return "", fmt.Errorf("failed to unmarshal response body: %w", err)
			}

			// Extract text from response
			var responseText string
			for _, block := range respBody.Content {
				if block.Type == "text" {
					responseText += block.Text
				}
			}

			// Cache response
			if c.cache != nil {
				if err := c.cache.Set(cacheKey, responseText); err != nil {
					c.logger.Warn("Failed to cache response", "error", err)
				}
			}

			// Calculate cost
			cost := CalculateCost(c.model, respBody.Usage.InputTokens, respBody.Usage.OutputTokens)

			// Update total cost and tokens
			c.totalCost += cost.Cost
			c.totalTokens += cost.TotalTokens

			// Log response details with cost information
			c.logger.Info("Received response from Claude API",
				"input_tokens", respBody.Usage.InputTokens,
				"output_tokens", respBody.Usage.OutputTokens,
				"total_tokens", cost.TotalTokens,
				"cost", fmt.Sprintf("$%.4f", cost.Cost),
				"total_cost", fmt.Sprintf("$%.4f", c.totalCost),
				"response_length", len(responseText))

			return responseText, nil
		} else if resp.StatusCode == http.StatusTooManyRequests && retry < maxRetries {
			// Rate limit error, extract message and retry with backoff
			var errorMsg string
			var errorResp map[string]interface{}

			if err := json.Unmarshal(respData, &errorResp); err == nil {
				if errObj, ok := errorResp["error"].(map[string]interface{}); ok {
					if msg, ok := errObj["message"].(string); ok {
						errorMsg = msg
					}
				}
			}

			if errorMsg == "" {
				errorMsg = string(respData)
			}

			// Calculate backoff delay with exponential increase
			delay := baseDelay * time.Duration(1<<retry)
			c.logger.Warn("Rate limit exceeded, retrying after backoff",
				"retry", retry+1,
				"max_retries", maxRetries,
				"delay", delay,
				"error", errorMsg)

			// Wait before retrying
			time.Sleep(delay)

			// Create a new request for the retry
			req, err = http.NewRequest("POST", ClaudeAPIURL, bytes.NewBuffer(reqData))
			if err != nil {
				return "", fmt.Errorf("failed to create retry request: %w", err)
			}

			// Set headers again
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("x-api-key", c.apiKey)
			req.Header.Set("anthropic-version", "2023-06-01")
		} else {
			// Other error, extract message and return
			var errorMsg string
			var errorResp map[string]interface{}

			if err := json.Unmarshal(respData, &errorResp); err == nil {
				if errObj, ok := errorResp["error"].(map[string]interface{}); ok {
					if msg, ok := errObj["message"].(string); ok {
						errorMsg = msg
					}
				}
			}

			if errorMsg == "" {
				errorMsg = string(respData)
			}

			return "", fmt.Errorf("Claude API request failed with status %d: %s", resp.StatusCode, errorMsg)
		}
	}

	// If we get here, we've exhausted all retries
	return "", fmt.Errorf("Claude API request failed after %d retries: rate limit exceeded", maxRetries)
}

// getLanguageInstructions returns language-specific instructions based on the output language
func (c *Client) getLanguageInstructions() string {
	switch c.outputLanguage {
	case "de-ch":
		return "Respond in German using Swiss High German spelling (replace ß with ss)."
	case "de":
		return "Respond in German."
	case "fr":
		return "Respond in French."
	case "it":
		return "Respond in Italian."
	default:
		return "" // Default to English (no special instructions)
	}
}

// IdentifyThemes identifies themes in a set of responses
func (c *Client) IdentifyThemes(responses []string, contextPrompt string) ([]string, error) {
	// Combine responses into a single prompt, but limit the number of responses
	// to avoid token limits
	maxResponsesToInclude := 50
	responseCount := len(responses)
	samplesToUse := min(responseCount, maxResponsesToInclude)

	// If we have more responses than our limit, select a representative sample
	// Use a deterministic sampling approach
	var selectedResponses []string
	if responseCount > maxResponsesToInclude {
		// Deterministic sampling - take evenly distributed responses
		step := responseCount / maxResponsesToInclude
		for i := 0; i < responseCount && len(selectedResponses) < maxResponsesToInclude; i += step {
			selectedResponses = append(selectedResponses, responses[i])
		}
	} else {
		selectedResponses = responses
	}

	// Build a stable prompt with consistent formatting
	combinedResponses := ""
	for i, response := range selectedResponses {
		// Truncate very long responses to save tokens
		truncatedResponse := response
		if len(response) > 500 {
			truncatedResponse = response[:497] + "..."
		}
		combinedResponses += fmt.Sprintf("%d: %s\n", i+1, truncatedResponse)
	}

	// Get language instructions
	langInstructions := c.getLanguageInstructions()

	// Create a more concise prompt with stable format
	prompt := fmt.Sprintf("Identify main themes in these %d survey responses (sample of %d total):\n\n%s\n\nReturn themes as a YAML list with each theme on a new line starting with a dash.",
		samplesToUse, responseCount, combinedResponses)

	// Add language instructions if needed
	if langInstructions != "" {
		prompt += " " + langInstructions
	}

	// Get completion
	completion, err := c.GetCompletion(prompt, contextPrompt, DefaultMaxTokens)
	if err != nil {
		return nil, fmt.Errorf("failed to identify themes: %w", err)
	}

	// Extract themes from completion
	themes := extractThemesFromYAML(completion)

	// Ensure we don't return nil
	if themes == nil {
		themes = []string{}
	}

	c.logger.Info("Identified themes", "count", len(themes))
	return themes, nil
}

// MatchResponsesToThemes matches responses to themes
func (c *Client) MatchResponsesToThemes(response string, themes []string, contextPrompt string) ([]string, error) {
	// Create prompt with consistent theme ordering
	themesText := ""
	for i, theme := range themes {
		themesText += fmt.Sprintf("%d. %s\n", i+1, theme)
	}

	// Get language instructions
	langInstructions := c.getLanguageInstructions()

	// Truncate very long responses to save tokens and ensure consistency
	truncatedResponse := response
	if len(response) > 500 {
		truncatedResponse = response[:497] + "..."
	}

	// Create a stable prompt format
	prompt := fmt.Sprintf("Here is a survey response:\n\n%s\n\nHere are the themes:\n%s\n\nWhich themes does this response relate to? Return the theme numbers as a YAML list with each number on a new line starting with a dash.", truncatedResponse, themesText)

	// Add language instructions if needed
	if langInstructions != "" {
		prompt += " " + langInstructions
	}

	// Get completion
	completion, err := c.GetCompletion(prompt, contextPrompt, DefaultMaxTokens)
	if err != nil {
		return nil, fmt.Errorf("failed to match response to themes: %w", err)
	}

	// Extract theme numbers from completion
	themeNumbers := extractThemeNumbersFromYAML(completion)

	// Convert theme numbers to theme names
	var matchedThemes []string
	for _, num := range themeNumbers {
		if num > 0 && num <= len(themes) {
			matchedThemes = append(matchedThemes, themes[num-1])
		}
	}

	// Ensure we don't return nil
	if matchedThemes == nil {
		matchedThemes = []string{}
	}

	c.logger.Debug("Matched response to themes", "themes", matchedThemes)
	return matchedThemes, nil
}

// MatchResponsesToThemesBatch matches multiple responses to themes in a single API call
func (c *Client) MatchResponsesToThemesBatch(responses []string, themes []string, contextPrompt string, batchSize int) ([][]string, error) {
	// Default batch size if not specified
	if batchSize <= 0 {
		batchSize = 10
	}

	// Process responses in batches
	var allResults [][]string

	for i := 0; i < len(responses); i += batchSize {
		end := i + batchSize
		if end > len(responses) {
			end = len(responses)
		}

		batch := responses[i:end]
		batchResults, err := c.processBatch(batch, themes, contextPrompt)
		if err != nil {
			return nil, fmt.Errorf("failed to process batch %d-%d: %w", i, end, err)
		}

		allResults = append(allResults, batchResults...)
	}

	return allResults, nil
}

// processBatch processes a batch of responses in a single API call
func (c *Client) processBatch(responses []string, themes []string, contextPrompt string) ([][]string, error) {
	// Create theme list once - sort by index to ensure consistent order
	themesText := ""
	for i, theme := range themes {
		themesText += fmt.Sprintf("%d. %s\n", i+1, theme)
	}

	// Build the prompt with all responses in the batch - use a stable format
	prompt := "Analyze multiple survey responses and match each to relevant themes.\n\n"
	prompt += "Themes:\n" + themesText + "\n"
	prompt += "For each response, identify which themes apply. Format your answer as:\n"
	prompt += "RESPONSE 1: [comma-separated theme numbers]\nRESPONSE 2: [comma-separated theme numbers]\n...\n\n"

	// Add all responses in a stable order
	for i, response := range responses {
		// Truncate very long responses to save tokens
		truncatedResponse := response
		if len(response) > 300 {
			truncatedResponse = response[:297] + "..."
		}
		prompt += fmt.Sprintf("RESPONSE %d: %s\n\n", i+1, truncatedResponse)
	}

	// Get language instructions
	langInstructions := c.getLanguageInstructions()
	if langInstructions != "" {
		prompt += langInstructions + "\n"
	}

	// Get completion
	completion, err := c.GetCompletion(prompt, contextPrompt, DefaultMaxTokens)
	if err != nil {
		return nil, fmt.Errorf("failed to match responses to themes in batch: %w", err)
	}

	// Parse the results
	return c.parseBatchResults(completion, len(responses), themes), nil
}

// parseBatchResults parses the batch results from the API response
func (c *Client) parseBatchResults(completion string, responseCount int, themes []string) [][]string {
	results := make([][]string, responseCount)

	// Initialize with empty slices
	for i := range results {
		results[i] = []string{}
	}

	// Split by lines
	lines := strings.Split(completion, "\n")

	// Extract theme numbers for each response
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Look for lines like "RESPONSE 1: 2, 4, 7"
		if strings.HasPrefix(line, "RESPONSE ") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) != 2 {
				continue
			}

			// Extract response number
			var responseNum int
			_, err := fmt.Sscanf(parts[0], "RESPONSE %d", &responseNum)
			if err != nil || responseNum < 1 || responseNum > responseCount {
				continue
			}

			// Extract theme numbers
			themeNumsStr := strings.TrimSpace(parts[1])
			themeNumsStr = strings.ReplaceAll(themeNumsStr, " ", "")
			themeNumStrs := strings.Split(themeNumsStr, ",")

			var matchedThemes []string
			for _, numStr := range themeNumStrs {
				var num int
				if _, err := fmt.Sscanf(numStr, "%d", &num); err == nil {
					if num > 0 && num <= len(themes) {
						matchedThemes = append(matchedThemes, themes[num-1])
					}
				}
			}

			// Store matched themes
			results[responseNum-1] = matchedThemes
		}
	}

	return results
}

// GenerateThemeSummary generates a summary for a specific theme and extracts unique ideas
func (c *Client) GenerateThemeSummary(theme string, responses []string, themeSummaryPrompt string) (string, error) {
	// Limit the number of responses to include
	maxResponses := 15

	// Create prompt with consistent format
	prompt := fmt.Sprintf("Theme: %s\n\nResponses:", theme)

	// Sort responses by length to ensure consistent selection if truncated
	// This helps create more stable cache keys
	if len(responses) > maxResponses {
		// Create a copy to avoid modifying the original
		responsesCopy := make([]string, len(responses))
		copy(responsesCopy, responses)

		// Sort by length (shorter responses first)
		sort.Slice(responsesCopy, func(i, j int) bool {
			return len(responsesCopy[i]) < len(responsesCopy[j])
		})

		// Take the first maxResponses
		responses = responsesCopy[:maxResponses]
	}

	// Add responses (limited)
	responsesToInclude := min(len(responses), maxResponses)
	for i := 0; i < responsesToInclude; i++ {
		// Truncate very long responses
		truncatedResponse := responses[i]
		if len(responses[i]) > 300 {
			truncatedResponse = responses[i][:297] + "..."
		}
		prompt += fmt.Sprintf("\n- %s", truncatedResponse)
	}

	if len(responses) > maxResponses {
		prompt += fmt.Sprintf("\n\n(Showing %d of %d responses)", maxResponses, len(responses))
	}

	// Get language instructions
	langInstructions := c.getLanguageInstructions()

	// Add concise instructions for structured output (without # symbols)
	prompt += "\n\nProvide:\nSUMMARY:\n[summary]\n\nUNIQUE IDEAS:\nIDEA: [idea 1]\nIDEA: [idea 2]\n...\n\nDo not include any # symbols in your response."

	// Add language instructions if needed
	if langInstructions != "" {
		prompt += "\n" + langInstructions
	}

	// Get completion
	completion, err := c.GetCompletion(prompt, themeSummaryPrompt, DefaultMaxTokens)
	if err != nil {
		return "", fmt.Errorf("failed to generate theme summary: %w", err)
	}

	return completion, nil
}

// GenerateGlobalSummary generates a global summary based on theme summaries
func (c *Client) GenerateGlobalSummary(themeSummaries map[string]ThemeSummary, globalSummaryPrompt string, summaryLength int) (string, error) {
	// Create a more concise prompt
	prompt := "Theme summaries from survey responses:\n\n"

	// Add theme summaries (more concisely)
	for theme, summary := range themeSummaries {
		prompt += fmt.Sprintf("## %s\n%s\n", theme, summary.Summary)

		// Only include a few unique ideas to save tokens
		if len(summary.UniqueIdeas) > 0 {
			maxIdeas := min(len(summary.UniqueIdeas), 3)
			prompt += "Key ideas:\n"
			for i := 0; i < maxIdeas; i++ {
				prompt += fmt.Sprintf("- %s\n", summary.UniqueIdeas[i])
			}
			if len(summary.UniqueIdeas) > maxIdeas {
				prompt += fmt.Sprintf("(+ %d more ideas)\n", len(summary.UniqueIdeas)-maxIdeas)
			}
		}
		prompt += "\n"
	}

	// Get language instructions
	langInstructions := c.getLanguageInstructions()

	prompt += fmt.Sprintf("Create a comprehensive global summary highlighting the most important findings. Length: ~%d characters.", summaryLength)

	// Add language instructions if needed
	if langInstructions != "" {
		prompt += " " + langInstructions
	}

	// Get completion
	completion, err := c.GetCompletion(prompt, globalSummaryPrompt, DefaultMaxTokens)
	if err != nil {
		return "", fmt.Errorf("failed to generate global summary: %w", err)
	}

	return completion, nil
}

// GenerateSummary generates a summary of the themes (for backward compatibility)
func (c *Client) GenerateSummary(themeResponses map[string][]string, summaryPrompt string, summaryLength int) (string, error) {
	// Create prompt
	prompt := "Here are the themes and their associated responses:\n\n"

	for theme, responses := range themeResponses {
		prompt += fmt.Sprintf("Theme: %s\n", theme)
		for i, response := range responses {
			if i < 10 { // Limit to 10 responses per theme to avoid token limits
				prompt += fmt.Sprintf("- %s\n", response)
			}
		}
		prompt += "\n"
	}

	// Get language instructions
	langInstructions := c.getLanguageInstructions()

	prompt += fmt.Sprintf("\nBased on the above, provide a summary of the main points made in each theme and highlight any unique ideas or problems mentioned. The summary should be approximately %d characters long.", summaryLength)

	// Add language instructions if needed
	if langInstructions != "" {
		prompt += " " + langInstructions
	}

	// Get completion
	completion, err := c.GetCompletion(prompt, summaryPrompt, DefaultMaxTokens)
	if err != nil {
		return "", fmt.Errorf("failed to generate summary: %w", err)
	}

	return completion, nil
}

// extractThemesFromYAML extracts themes from a YAML list
func extractThemesFromYAML(yamlText string) []string {
	// Initialize with empty slice to avoid nil
	themes := []string{}

	// Split by lines
	lines := bytes.Split([]byte(yamlText), []byte("\n"))

	// Extract themes
	for _, line := range lines {
		// Trim whitespace
		trimmed := bytes.TrimSpace(line)

		// Check if line starts with a dash
		if len(trimmed) > 0 && trimmed[0] == '-' {
			// Extract theme
			theme := string(bytes.TrimSpace(trimmed[1:]))
			if theme != "" {
				themes = append(themes, theme)
			}
		}
	}

	return themes
}

// extractThemeNumbersFromYAML extracts theme numbers from a YAML list
func extractThemeNumbersFromYAML(yamlText string) []int {
	// Initialize with empty slice to avoid nil
	numbers := []int{}

	// Split by lines
	lines := bytes.Split([]byte(yamlText), []byte("\n"))

	// Extract numbers
	for _, line := range lines {
		// Trim whitespace
		trimmed := bytes.TrimSpace(line)

		// Check if line starts with a dash
		if len(trimmed) > 0 && trimmed[0] == '-' {
			// Extract number
			var num int
			numStr := string(bytes.TrimSpace(trimmed[1:]))
			if _, err := fmt.Sscanf(numStr, "%d", &num); err == nil {
				numbers = append(numbers, num)
			}
		}
	}

	return numbers
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
