package claude

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// Client is a client for the Claude API
type Client struct {
	apiKey         string
	model          string
	httpClient     *http.Client
	logger         *logging.Logger
	cache          *cache.Cache
	outputLanguage string
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
	}
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

	c.logger.Info("Sending request to Claude API", "model", c.model)

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

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(respData))
	}

	// Unmarshal response
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

	c.logger.Info("Received response from Claude API",
		"input_tokens", respBody.Usage.InputTokens,
		"output_tokens", respBody.Usage.OutputTokens)

	return responseText, nil
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
	// Combine responses into a single prompt
	combinedResponses := ""
	for i, response := range responses {
		combinedResponses += fmt.Sprintf("Response %d: %s\n\n", i+1, response)
	}

	// Get language instructions
	langInstructions := c.getLanguageInstructions()

	// Create prompt
	prompt := fmt.Sprintf("Here are %d survey responses:\n\n%s\n\nIdentify the main themes or topics discussed in these responses. Return the themes as a YAML list with each theme on a new line starting with a dash.", len(responses), combinedResponses)

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
	c.logger.Info("Identified themes", "count", len(themes))

	return themes, nil
}

// MatchResponsesToThemes matches responses to themes
func (c *Client) MatchResponsesToThemes(response string, themes []string, contextPrompt string) ([]string, error) {
	// Create prompt
	themesText := ""
	for i, theme := range themes {
		themesText += fmt.Sprintf("%d. %s\n", i+1, theme)
	}

	// Get language instructions
	langInstructions := c.getLanguageInstructions()

	prompt := fmt.Sprintf("Here is a survey response:\n\n%s\n\nHere are the themes:\n%s\n\nWhich themes does this response relate to? Return the theme numbers as a YAML list with each number on a new line starting with a dash.", response, themesText)

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

	c.logger.Debug("Matched response to themes", "themes", matchedThemes)
	return matchedThemes, nil
}

// GenerateThemeSummary generates a summary for a specific theme and extracts unique ideas
func (c *Client) GenerateThemeSummary(theme string, responses []string, themeSummaryPrompt string) (string, error) {
	// Create prompt
	prompt := fmt.Sprintf("Theme: %s\n\nResponses related to this theme:\n\n", theme)

	// Add responses
	for i, response := range responses {
		if i < 10 { // Limit to 10 responses per theme to avoid token limits
			prompt += fmt.Sprintf("- %s\n", response)
		}
	}

	// Get language instructions
	langInstructions := c.getLanguageInstructions()

	// Add instructions for structured output
	prompt += "\nFor this theme, provide:\n"
	prompt += "1. A summary of the main points discussed in the responses.\n"
	prompt += "2. A list of unique ideas or suggestions.\n\n"
	prompt += "Format your response with two clearly separated sections:\n"
	prompt += "SUMMARY:\n[Your summary text here]\n\n"
	prompt += "UNIQUE IDEAS:\n"
	prompt += "IDEA: [First unique idea]\n"
	prompt += "IDEA: [Second unique idea]\n"
	prompt += "...\n"

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
	// Create prompt
	prompt := "Here are summaries for each theme from the survey responses:\n\n"

	// Add theme summaries
	for theme, summary := range themeSummaries {
		prompt += fmt.Sprintf("## %s\n", theme)
		prompt += fmt.Sprintf("Summary: %s\n\n", summary.Summary)

		if len(summary.UniqueIdeas) > 0 {
			prompt += "Unique ideas:\n"
			for _, idea := range summary.UniqueIdeas {
				prompt += fmt.Sprintf("- %s\n", idea)
			}
		}
		prompt += "\n"
	}

	// Get language instructions
	langInstructions := c.getLanguageInstructions()

	prompt += fmt.Sprintf("\nBased on the above theme summaries, provide a comprehensive global summary of the survey responses. The summary should be approximately %d characters long and highlight the most important findings across all themes.", summaryLength)

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
	var themes []string

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
	var numbers []int

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
