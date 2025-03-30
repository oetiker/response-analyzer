# Response Analyzer

A Go application for analyzing free-form responses from questionnaires using Claude AI.

## Features

- **Quantitative Analysis**: Identify and analyze the main topics people are talking about in survey responses
- **Unique Ideas Extraction**: Get a list of unique ideas or problems mentioned in the responses
- **Audit Log**: Generate an audit log to verify how original texts were mapped to identified themes
- **Incremental Processing**: Only analyze new or changed responses in subsequent runs
- **Caching**: Cache Claude API responses to avoid repeated API calls
- **Cost Tracking**: Track and display the cost of Claude API calls
- **Rate Limiting**: Automatically handle API rate limits with exponential backoff
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

4. If no themes are defined in your config file, the application will automatically run in themes-identification mode:
   - It will identify themes in the responses
   - Output the themes to the console and a `themes.yaml` file
   - Stop after theme identification

5. Add the identified themes to your configuration file under the `themes:` section.

6. Run the application again to perform the full analysis:
   ```
   ./response-analyzer -config config.yaml
   ```

## Workflow

1. **Themes Identification**: Automatically activated when no themes are in the config file
   - The application identifies themes in the responses
   - Outputs identified themes to the console and a `themes.yaml` file
   - Stops after theme identification, allowing you to review and add themes to the config file
   - No full analysis is performed at this stage
   - You can also force this mode with the `-identify-themes-only` flag, even if themes are present in the config

2. **Full Analysis**: Runs when themes are present in the config file
   - The application uses themes from your config file
   - Matches responses to themes
   - Generates a state file with responses and matching themes
   - Produces an audit log showing how responses map to themes
   - Creates a summary of main points and unique ideas

This two-step workflow ensures you can review and customize the themes before the full analysis is performed.

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
