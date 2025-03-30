# Changelog

All notable changes to the Response Analyzer project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.0] - 2025-03-30

### Added
- Column title from Excel file is now available in report templates (@oetiker)
- Added ColumnTitle field to AnalysisResult struct (@oetiker)

### Changed
- Renamed `summary_length` to `global_summary_length` for clarity (@oetiker)
- Simplified function parameters by passing the entire Config struct (@oetiker)
- Improved code maintainability for future enhancements (@oetiker)
- Modified the analyzer to use a default global summary prompt when needed (@oetiker)
- Prevented global summary from including a title (@oetiker)

### Deprecated
- Deprecated the redundant `summary_prompt` parameter in favor of theme_summary_prompt and global_summary_prompt (@oetiker)

## [0.1.0] - 2025-03-15

### Added
- Initial release of Response Analyzer (@oetiker)
- Theme identification from free-form responses (@oetiker)
- Response matching to identified themes (@oetiker)
- Theme summaries with unique ideas extraction (@oetiker)
- Global summary generation (@oetiker)
- Excel file input support (@oetiker)
- Caching for Claude API responses (@oetiker)
- Incremental processing (only analyze new/changed responses) (@oetiker)
- Report template system (@oetiker)
- Multi-language support (en, de, de-ch, fr, it) (@oetiker)
