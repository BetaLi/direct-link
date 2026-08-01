package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
)

func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

type LogEntry struct {
	Time   time.Time `json:"time"`
	Level  string   `json:"level"`
	Msg    string   `json:"msg"`
}

type Logger struct {
	mu       sync.Mutex
	entries  []LogEntry
	maxSize  int
	filePath string
}

var defaultLogger *Logger

func Init(logDir string) {
	if logDir == "" {
		logDir = filepath.Join(os.Getenv("APPDATA"), "DirectLink")
	}
	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create log dir: %v\n", err)
		logDir = ""
	}
	defaultLogger = &Logger{
		entries:  make([]LogEntry, 0, 500),
		maxSize:  500,
		filePath: filepath.Join(logDir, "directlink.log"),
	}
}

func Get() *Logger {
	if defaultLogger == nil {
		Init("")
	}
	return defaultLogger
}

func (l *Logger) log(level LogLevel, format string, args ...interface{}) {
	entry := LogEntry{
		Time:  time.Now(),
		Level: level.String(),
		Msg:   fmt.Sprintf(format, args...),
	}

	l.mu.Lock()
	l.entries = append(l.entries, entry)
	if len(l.entries) > l.maxSize {
		l.entries = l.entries[len(l.entries)-l.maxSize:]
	}
	l.mu.Unlock()

	// Also print to console
	fmt.Printf("[%s] %s %s\n", entry.Time.Format("15:04:05"), entry.Level, entry.Msg)

	// Append to log file
	if l.filePath != "" {
		f, err := os.OpenFile(l.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			defer f.Close()
			fmt.Fprintf(f, "[%s] %s %s\n",
				entry.Time.Format("2006-01-02 15:04:05"),
				entry.Level, entry.Msg)
		}
	}
}

func (l *Logger) Debug(format string, args ...interface{}) { l.log(LevelDebug, format, args...) }
func (l *Logger) Info(format string, args ...interface{})  { l.log(LevelInfo, format, args...) }
func (l *Logger) Warn(format string, args ...interface{})  { l.log(LevelWarn, format, args...) }
func (l *Logger) Error(format string, args ...interface{}) { l.log(LevelError, format, args...) }

func (l *Logger) GetEntries() []LogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	// Return reversed copy (newest first)
	n := len(l.entries)
	result := make([]LogEntry, n)
	for i, e := range l.entries {
		result[n-1-i] = e
	}
	return result
}

// Package-level convenience functions
func Debug(format string, args ...interface{}) { Get().Debug(format, args...) }
func Info(format string, args ...interface{})  { Get().Info(format, args...) }
func Warn(format string, args ...interface{})  { Get().Warn(format, args...) }
func Error(format string, args ...interface{}) { Get().Error(format, args...) }
