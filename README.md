# Response Analyzer

A Go application for analyzing free-form responses from questionnaires using Claude AI.

## Features

- **Quantitative Analysis**: Identify and analyze the main topics people are talking about in survey responses
- **Unique Ideas Extraction**: Get a list of unique ideas or problems mentioned in the responses
- **Audit Log**: Generate an audit log to verify how original texts were mapped to identified themes
- **Incremental Processing**: Only analyze new or changed responses in subsequent runs
- **Caching**: Cache Claude API responses to avoid repeated API calls
- **YAML Configuration**: Control the application using a YAML configuration file

## Requirements

- Go 1.18 or higher
- Claude API key
- Excel file (.xlsx) containing survey responses

## Installation

1. Clone the repository:
   ```
   git clone https://github.com/oetiker/response-analyzer.git
   cd response-analyzer
   ```

2. Build the application:
   ```
   go build -o response-analyzer ./cmd/response-analyzer
   ```

## Usage

1. Create a configuration file based on the sample:
   ```
   cp config-sample.yaml config.yaml
   ```

2. Edit the configuration file to set your Excel file path, response column, and Claude API key.

3. Run the application:
   ```
   ./response-analyzer -config config.yaml
   ```

4. On the first run, the application will identify themes in the responses and output them to the console and a `themes.yaml` file. Add these themes to your configuration file for subsequent runs.

5. On subsequent runs, the application will match responses to the themes and generate a summary.

## Workflow

1. **First Run**: The application analyzes all responses to identify themes
   - Outputs identified themes to the console and a `themes.yaml` file
   - You can add/revise these themes in the config file

2. **Subsequent Runs**: The application matches responses to themes
   - Generates a state file with responses and matching themes
   - Produces an audit log showing how responses map to themes
   - Creates a summary of main points and unique ideas

## Output Files

- **State File**: Contains the complete analysis result (responses, themes, mappings)
- **Audit Log**: Shows how each response was mapped to themes
- **Theme Statistics**: Provides quantitative analysis of theme prevalence
- **Summary**: A text file containing the AI-generated summary of main points and unique ideas

## Configuration Options

See `config-sample.yaml` for a complete list of configuration options with comments.

Key options include:
- `excel_file_path`: Path to the Excel file containing responses
- `response_column`: Column letter containing the responses
- `claude_api_key`: Your Claude API key
- `claude_model`: Claude model to use (defaults to claude-3-opus-20240229)
- `context_prompt`: Prompt for theme identification
- `theme_summary_prompt`: Prompt for per-theme summaries
- `global_summary_prompt`: Prompt for global summary
- `summary_prompt`: Prompt for summary generation (for backward compatibility)
- `output_language`: Language for the output (en, de, de-ch, fr, it)
- `themes`: List of themes to use (populated after first run)
- `cache_enabled`: Enable caching to avoid repeated API calls
- `report_template_path`: Path to a custom report template
- `report_output_path`: Path for the generated report

## Example

```yaml
# Excel file configuration
excel_file_path: "responses.xlsx"
response_column: "C"

# Claude API configuration
claude_api_key: "your-claude-api-key-here"
claude_model: "claude-3-opus-20240229"
context_prompt: "Analyze these survey responses about our product."
theme_summary_prompt: "For this theme, provide a detailed summary and extract unique ideas."
global_summary_prompt: "Based on the theme summaries, provide a comprehensive overview."
summary_length: 1000

# Output language configuration
output_language: "de-ch"  # German with Swiss spelling (ß -> ss)

# Themes (populated after first run)
themes:
  - "User Interface Issues"
  - "Performance Problems"
  - "Feature Requests"

# Report template configuration
report_template_path: "report-template.tmpl"
report_output_path: "report.txt"
```

## Output Languages

The application supports the following output languages:

- `en`: English (default)
- `de`: German
- `de-ch`: German with Swiss High German spelling (ß -> ss)
- `fr`: French
- `it`: Italian

## Report Templates

You can create custom report templates using Go's text/template syntax. The template has access to the following variables:

- `Themes`: List of identified themes
- `ThemeStats`: Statistics for each theme (count, percentage)
- `ThemeSummaries`: Map of theme summaries with unique ideas
- `GlobalSummary`: The generated global summary
- `Summary`: The generated summary (for backward compatibility)
- `Responses`: All analyzed responses
- `ResponseCount`: Total number of responses
- `AnalysisDate`: Date of the analysis

Example template:
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

## License

MIT
