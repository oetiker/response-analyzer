package logging

import (
	"fmt"
	"log"
	"os"
	"time"
)

// LogLevel represents the severity level of a log message
type LogLevel int

const (
	// LogLevelDebug is for detailed debugging information
	LogLevelDebug LogLevel = iota
	// LogLevelInfo is for general operational information
	LogLevelInfo
	// LogLevelWarn is for warning messages
	LogLevelWarn
	// LogLevelError is for error messages
	LogLevelError
)

// Logger provides logging functionality
type Logger struct {
	debugLogger *log.Logger
	infoLogger  *log.Logger
	warnLogger  *log.Logger
	errorLogger *log.Logger
	verbose     bool
}

// NewLogger creates a new logger instance
func NewLogger(verbose bool) *Logger {
	return &Logger{
		debugLogger: log.New(os.Stdout, "DEBUG: ", log.Ldate|log.Ltime),
		infoLogger:  log.New(os.Stdout, "INFO: ", log.Ldate|log.Ltime),
		warnLogger:  log.New(os.Stdout, "WARN: ", log.Ldate|log.Ltime),
		errorLogger: log.New(os.Stderr, "ERROR: ", log.Ldate|log.Ltime),
		verbose:     verbose,
	}
}

// formatMessage formats a log message with optional key-value pairs
func formatMessage(msg string, keyvals ...interface{}) string {
	if len(keyvals) == 0 {
		return msg
	}

	formatted := msg
	for i := 0; i < len(keyvals); i += 2 {
		var key, val string
		key = fmt.Sprintf("%v", keyvals[i])

		if i+1 < len(keyvals) {
			val = fmt.Sprintf("%v", keyvals[i+1])
		} else {
			val = "<missing>"
		}

		formatted += fmt.Sprintf(" %s=%s", key, val)
	}

	return formatted
}

// Debug logs a debug message
func (l *Logger) Debug(msg string, keyvals ...interface{}) {
	if l.verbose {
		l.debugLogger.Println(formatMessage(msg, keyvals...))
	}
}

// Info logs an informational message
func (l *Logger) Info(msg string, keyvals ...interface{}) {
	l.infoLogger.Println(formatMessage(msg, keyvals...))
}

// Warn logs a warning message
func (l *Logger) Warn(msg string, keyvals ...interface{}) {
	l.warnLogger.Println(formatMessage(msg, keyvals...))
}

// Error logs an error message
func (l *Logger) Error(msg string, keyvals ...interface{}) {
	l.errorLogger.Println(formatMessage(msg, keyvals...))
}

// LogOperation logs the start and end of an operation with timing information
func (l *Logger) LogOperation(operation string, fn func() error) error {
	l.Info(fmt.Sprintf("Starting %s", operation))
	startTime := time.Now()

	err := fn()

	duration := time.Since(startTime)
	if err != nil {
		l.Error(fmt.Sprintf("Failed %s", operation), "duration", duration, "error", err)
		return err
	}

	l.Info(fmt.Sprintf("Completed %s", operation), "duration", duration)
	return nil
}
