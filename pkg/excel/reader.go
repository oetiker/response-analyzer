package excel

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/oetiker/response-analyzer/pkg/logging"
	"github.com/xuri/excelize/v2"
)

// Response represents a single response from the Excel file
type Response struct {
	ID       string // Unique identifier for the response
	Text     string // The response text
	RowIndex int    // The row index in the Excel file (1-based)
	Hash     string // Hash of the response text for change detection
}

// ExcelReader handles reading responses from Excel files
type ExcelReader struct {
	logger *logging.Logger
}

// NewExcelReader creates a new ExcelReader instance
func NewExcelReader(logger *logging.Logger) *ExcelReader {
	return &ExcelReader{
		logger: logger,
	}
}

// ReadResponses reads responses from an Excel file
func (r *ExcelReader) ReadResponses(filePath, columnLetter string) ([]Response, error) {
	r.logger.Info("Reading Excel file", "path", filePath, "column", columnLetter)

	// Open the Excel file
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open Excel file: %w", err)
	}
	defer f.Close()

	// Get the first sheet
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("no sheets found in Excel file")
	}
	sheetName := sheets[0]

	// Convert column letter to index
	columnIndex, err := excelize.ColumnNameToNumber(columnLetter)
	if err != nil {
		return nil, fmt.Errorf("invalid column letter: %w", err)
	}

	// Read all rows
	rows, err := f.GetRows(sheetName)
	if err != nil {
		return nil, fmt.Errorf("failed to read rows: %w", err)
	}

	// Extract responses
	var responses []Response
	for i, row := range rows {
		rowIndex := i + 1 // Excel rows are 1-based

		// Skip header row
		if rowIndex == 1 {
			continue
		}

		// Check if column exists in this row
		if len(row) < columnIndex {
			r.logger.Warn("Row does not have the specified column", "row", rowIndex, "column", columnLetter)
			continue
		}

		// Get response text
		text := strings.TrimSpace(row[columnIndex-1])
		if text == "" {
			r.logger.Debug("Empty response", "row", rowIndex)
			continue
		}

		// Create response object
		hash := hashText(text)
		response := Response{
			ID:       fmt.Sprintf("R%d", rowIndex),
			Text:     text,
			RowIndex: rowIndex,
			Hash:     hash,
		}

		responses = append(responses, response)
	}

	r.logger.Info("Read responses from Excel file", "count", len(responses))
	return responses, nil
}

// ValidateExcelFile validates that the Excel file exists and has the specified column
func (r *ExcelReader) ValidateExcelFile(filePath, columnLetter string) error {
	r.logger.Info("Validating Excel file", "path", filePath, "column", columnLetter)

	// Check if file exists and can be opened
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to open Excel file: %w", err)
	}
	defer f.Close()

	// Check if column letter is valid
	_, err = excelize.ColumnNameToNumber(columnLetter)
	if err != nil {
		return fmt.Errorf("invalid column letter: %w", err)
	}

	// Get the first sheet
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return fmt.Errorf("no sheets found in Excel file")
	}

	r.logger.Info("Excel file validation successful")
	return nil
}

// hashText creates a hash of the text for change detection
func hashText(text string) string {
	hash := sha256.Sum256([]byte(text))
	return hex.EncodeToString(hash[:])
}
