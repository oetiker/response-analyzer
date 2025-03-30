# Response Analyzer Technical Roadmap

## Project Overview
Response Analyzer is a Go application that uses Claude AI to analyze free-form responses from questionnaires. It identifies themes, matches responses to themes, generates summaries, and extracts unique ideas.

## Project Structure
```
response-analyzer/
├── cmd/
│   └── response-analyzer/
│       └── main.go           # Application entry point
├── pkg/
│   ├── analysis/             # Core analysis functionality
│   │   └── analyzer.go       # Implements theme identification and response analysis
│   ├── cache/                # Caching functionality
│   │   └── cache.go          # Implements caching for Claude API responses
│   ├── claude/               # Claude API client
│   │   └── client.go         # Handles communication with Claude API
│   ├── config/               # Configuration handling
│   │   └── config.go         # Defines configuration structure and loading
│   ├── excel/                # Excel file handling
│   │   └── reader.go         # Reads responses from Excel files
│   ├── logging/              # Logging functionality
│   │   └── logger.go         # Implements logging
│   ├── output/               # Output generation
│   │   └── writer.go         # Handles writing results to files
│   ├── template/             # Template rendering
│   │   └── renderer.go       # Renders templates for reports
│   └── validation/           # Input validation
│       └── validator.go      # Validates configuration and inputs
├── config-sample.yaml        # Sample configuration file
├── report-template.tmpl      # German report template
└── report-template-en.tmpl   # English report template
```

## Key Components and Their Interactions

### 1. Configuration (`pkg/config/config.go`)
The `Config` struct defines all configuration options:
```go
type Config struct {
    // Excel file configuration
    ExcelFilePath  string `yaml:"excel_file_path"`
    ResponseColumn string `yaml:"response_column"`

    // Claude API configuration
    ClaudeAPIKey  string `yaml:"claude_api_key"`
    ClaudeModel   string `yaml:"claude_model,omitempty"`
    ContextPrompt string `yaml:"context_prompt"`
    SummaryPrompt string `yaml:"summary_prompt"`
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

    // Report template configuration
    ReportTemplatePath string `yaml:"report_template_path,omitempty"`
    ReportOutputPath   string `yaml:"report_output_path,omitempty"`
}
```

### 2. Claude Client (`pkg/claude/client.go`)
Handles communication with the Claude API:
- `NewClient(apiKey, logger, cache, outputLanguage, model)` - Creates a new client
- `IdentifyThemes(responses, contextPrompt)` - Identifies themes in responses
- `MatchResponsesToThemes(response, themes, contextPrompt)` - Matches a response to themes
- `GenerateThemeSummary(theme, responses, themeSummaryPrompt)` - Generates a summary for a theme
- `GenerateGlobalSummary(themeSummaries, globalSummaryPrompt, summaryLength)` - Generates a global summary
- `GenerateSummary(themeResponses, summaryPrompt, summaryLength)` - Legacy summary generation

The client adds language instructions based on the `outputLanguage` configuration:
```go
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
```

### 3. Analyzer (`pkg/analysis/analyzer.go`)
Core analysis functionality:
- `AnalyzeResponses(responses, themes, contextPrompt, summaryPrompt, themeSummaryPrompt, globalSummaryPrompt, summaryLength, previousResult)` - Main analysis function
- `IdentifyThemes(responses, contextPrompt)` - Identifies themes in responses
- `MatchResponsesToThemes(responses, themes, contextPrompt, previousAnalyses)` - Matches responses to themes
- `BuildThemeAnalyses(responseAnalyses, themes)` - Builds theme analyses
- `GenerateThemeSummaries(responseAnalyses, themeAnalyses, themeSummaryPrompt)` - Generates summaries for each theme
- `GenerateGlobalSummary(themeSummaries, globalSummaryPrompt, summaryLength)` - Generates a global summary
- `GenerateSummary(responseAnalyses, themeAnalyses, summaryPrompt, summaryLength)` - Legacy summary generation

Key data structures:
```go
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
    Summary           string                         `yaml:"summary,omitempty"`
    GlobalSummary     string                         `yaml:"global_summary,omitempty"`
    UniqueIdeas       []string                       `yaml:"unique_ideas,omitempty"`
    AnalysisTimestamp time.Time                      `yaml:"analysis_timestamp"`
}
```

### 4. Template Renderer (`pkg/template/renderer.go`)
Renders templates for reports:
- `RenderTemplate(templatePath, outputPath, result)` - Renders a template with the given data

The template data structure:
```go
// TemplateData represents the data available in templates
type TemplateData struct {
    Themes        []string
    ThemeStats    []ThemeStat
    Summary       string
    Responses     []ResponseData
    ResponseCount int
    AnalysisDate  time.Time
}
```

### 5. Output Writer (`pkg/output/writer.go`)
Handles writing results to files:
- `SaveState(result, path)` - Saves the analysis result to a state file
- `LoadState(path)` - Loads the analysis result from a state file
- `SaveThemes(themes, path)` - Saves the themes to a YAML file
- `SaveSummary(summary, path)` - Saves the summary to a file
- `SaveAuditLog(result, path)` - Saves the audit log to a YAML file
- `SaveThemeStats(result, path)` - Saves theme statistics to a YAML file
- `GenerateReport(result, templatePath, outputPath)` - Generates a report using a template

### 6. Main Workflow (`cmd/response-analyzer/main.go`)
The main workflow:
1. Parse command line flags
2. Load configuration
3. Initialize components (cache, Claude client, Excel reader, analyzer, output writer)
4. Read responses from Excel file
5. Check if state file exists and load previous result if available
6. Analyze responses
   - If themes are provided in config, use them
   - If themes are available in previous state, use them
   - Otherwise, identify themes
7. Save state, audit log, theme statistics, and summary
8. Generate report if template is provided

## Template System
Templates use Go's `text/template` package. Available variables:
- `Themes`: List of identified themes
- `ThemeStats`: Statistics for each theme (count, percentage)
- `ThemeSummaries`: Map of theme summaries with unique ideas
- `GlobalSummary`: The generated global summary
- `Summary`: The generated summary (for backward compatibility)
- `Responses`: All analyzed responses
- `ResponseCount`: Total number of responses
- `AnalysisDate`: Date of the analysis

Example template usage:
```
# Survey Analysis Report
Date: {{.AnalysisDate.Format "02.01.2006"}}
Total Responses: {{.ResponseCount}}

## Global Summary
{{.GlobalSummary}}

## Themes
{{range .ThemeStats}}
### {{.Theme}} ({{.Count}} responses, {{printf "%.1f" .Percentage}}%)
{{end}}

## Detailed Theme Analysis
{{range $theme := .Themes}}
{{if index $.ThemeSummaries $theme}}
### {{$theme}}

#### Summary
{{(index $.ThemeSummaries $theme).Summary}}

{{if (index $.ThemeSummaries $theme).UniqueIdeas}}
#### Unique Ideas
{{range $idea := (index $.ThemeSummaries $theme).UniqueIdeas}}
- {{$idea}}
{{end}}
{{end}}
{{end}}
{{end}}
```

## Workflow for Adding New Features

1. **Update Configuration**: Add new configuration options to `pkg/config/config.go`
2. **Update Claude Client**: Add new methods to `pkg/claude/client.go` if needed
3. **Update Analyzer**: Add new analysis functionality to `pkg/analysis/analyzer.go`
4. **Update Output Writer**: Add new output functionality to `pkg/output/writer.go` if needed
5. **Update Main Workflow**: Update `cmd/response-analyzer/main.go` to use new functionality
6. **Update Templates**: Update templates to display new data
7. **Update Documentation**: Update `README.md` and `config-sample.yaml`

## GitHub Actions Workflow

The project includes a GitHub Actions workflow for building and releasing the application:

```
.github/workflows/
└── release.yml         # Workflow for building and releasing the application
```

### Release Workflow

The release workflow is triggered manually and performs the following steps:
1. Creates a new tag based on the provided version
2. Builds the application for multiple platforms:
   - Windows (x86_64, ARM64)
   - Linux (x86_64, ARM64)
   - macOS (x86_64, ARM64)
3. Packages the application with sample config and documentation:
   - Windows: ZIP format
   - Linux: tar.gz format
   - macOS: tar.gz format
4. Creates a GitHub release with the packaged files

To create a new release:
1. Go to the GitHub repository
2. Navigate to the "Actions" tab
3. Select the "Build and Release" workflow
4. Click "Run workflow"
5. Enter the version tag (e.g., v1.0.0)
6. Select whether this is a prerelease
7. Click "Run workflow"

The workflow will create a new release with the following files:
- `response-analyzer-v1.0.0-windows-amd64.zip`
- `response-analyzer-v1.0.0-windows-arm64.zip`
- `response-analyzer-v1.0.0-linux-amd64.tar.gz`
- `response-analyzer-v1.0.0-linux-arm64.tar.gz`
- `response-analyzer-v1.0.0-darwin-amd64.tar.gz`
- `response-analyzer-v1.0.0-darwin-arm64.tar.gz`

## Common Patterns

### Adding a New Configuration Option
1. Add the field to the `Config` struct in `pkg/config/config.go`
2. Set a default value in the `LoadConfig` function if needed
3. Update `config-sample.yaml` with the new option and documentation

### Adding a New Claude API Functionality
1. Add a new method to the `Client` struct in `pkg/claude/client.go`
2. Implement the prompt construction, including language instructions
3. Call the `GetCompletion` method to get the response
4. Process the response and return the result

### Adding a New Analysis Feature
1. Add new fields to the `AnalysisResult` struct in `pkg/analysis/analyzer.go` if needed
2. Add a new method to the `Analyzer` struct
3. Update the `AnalyzeResponses` method to use the new functionality
4. Update the template data structure in `pkg/template/renderer.go` if needed

### Adding a New Output Format
1. Add a new method to the `Writer` struct in `pkg/output/writer.go`
2. Update the main workflow in `cmd/response-analyzer/main.go` to use the new method
3. Create a new template if needed
