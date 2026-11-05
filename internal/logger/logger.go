package logger

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gopkg.in/natefinch/lumberjack.v2"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var (
	// GlobalLevelVar allows dynamic, thread-safe runtime log level adjustment
	GlobalLevelVar = new(slog.LevelVar)

	// AppRollingLogger holds file rotator for application logs
	AppRollingLogger *lumberjack.Logger

	// ErrorRollingLogger holds file rotator for errors
	ErrorRollingLogger *lumberjack.Logger

	// PermanentLogWriter writes to an unrotated permanent archive
	permanentMu        sync.Mutex
	permanentLogWriter *os.File
	permanentLevel     = "error" // "error", "warn", "none"

	// Ring buffer for dashboard real-time stream
	ringMu       sync.RWMutex
	recentLogs   []string
	recentErrors []string
	maxRingSize  = 100
)

type Config struct {
	Level          string `yaml:"level"`           // debug, info, warn, error
	Dir            string `yaml:"dir"`             // logs
	MaxSizeMB      int    `yaml:"max_size_mb"`     // max size per file before rotation
	MaxBackups     int    `yaml:"max_backups"`     // max old files to keep
	MaxAgeDays     int    `yaml:"max_age_days"`    // max days to retain
	Compress       bool   `yaml:"compress"`        // gzip compressed
	PermanentLevel string `yaml:"permanent_level"` // error, warn, none
}

func DefaultConfig() Config {
	return Config{
		Level:          "info",
		Dir:            "logs",
		MaxSizeMB:      20,
		MaxBackups:     10,
		MaxAgeDays:     30,
		Compress:       true,
		PermanentLevel: "error",
	}
}

// MultiLevelHandler distributes logs across rolling files, permanent logs, and in-memory streams
type MultiLevelHandler struct {
	baseHandler slog.Handler
}

func (h *MultiLevelHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.baseHandler.Enabled(ctx, level)
}

func (h *MultiLevelHandler) Handle(ctx context.Context, r slog.Record) error {
	// 1. Output to primary handler (stdout + app.log)
	_ = h.baseHandler.Handle(ctx, r)

	// Format text representation for buffer and error files
	formattedTime := r.Time.Format("2006-01-02 15:04:05")
	var attrBuilder strings.Builder
	r.Attrs(func(a slog.Attr) bool {
		attrBuilder.WriteString(fmt.Sprintf(" %s=%v", a.Key, a.Value))
		return true
	})
	line := fmt.Sprintf("[%s] [%s] %s%s", formattedTime, r.Level.String(), r.Message, attrBuilder.String())

	// 2. Append to in-memory recent logs ring buffer
	ringMu.Lock()
	recentLogs = append(recentLogs, line)
	if len(recentLogs) > maxRingSize {
		recentLogs = recentLogs[len(recentLogs)-maxRingSize:]
	}
	if r.Level >= slog.LevelError {
		recentErrors = append(recentErrors, line)
		if len(recentErrors) > maxRingSize {
			recentErrors = recentErrors[len(recentErrors)-maxRingSize:]
		}
	}
	ringMu.Unlock()

	// 3. Write errors to rolling error.log
	if r.Level >= slog.LevelError && ErrorRollingLogger != nil {
		_, _ = fmt.Fprintln(ErrorRollingLogger, line)
	}

	// 4. Permanent retention (Requirement 8: unrotated permanent storage)
	permanentMu.Lock()
	defer permanentMu.Unlock()
	if permanentLogWriter != nil && permanentLevel != "none" {
		targetLvl := slog.LevelError
		if permanentLevel == "warn" {
			targetLvl = slog.LevelWarn
		}
		if r.Level >= targetLvl {
			_, _ = fmt.Fprintln(permanentLogWriter, line)
		}
	}

	return nil
}

func (h *MultiLevelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &MultiLevelHandler{baseHandler: h.baseHandler.WithAttrs(attrs)}
}

func (h *MultiLevelHandler) WithGroup(name string) slog.Handler {
	return &MultiLevelHandler{baseHandler: h.baseHandler.WithGroup(name)}
}

// InitLogger initializes the layered logging architecture with rolling files and console output
func InitLogger(cfg Config) error {
	if cfg.Dir == "" {
		cfg.Dir = "logs"
	}
	if err := os.MkdirAll(cfg.Dir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	SetLevel(cfg.Level)
	SetPermanentLevel(cfg.PermanentLevel)

	maxSize := cfg.MaxSizeMB
	if maxSize <= 0 {
		maxSize = 20
	}
	maxBackups := cfg.MaxBackups
	if maxBackups <= 0 {
		maxBackups = 10
	}
	maxAge := cfg.MaxAgeDays
	if maxAge <= 0 {
		maxAge = 30
	}

	AppRollingLogger = &lumberjack.Logger{
		Filename:   filepath.Join(cfg.Dir, "app.log"),
		MaxSize:    maxSize,
		MaxBackups: maxBackups,
		MaxAge:     maxAge,
		Compress:   cfg.Compress,
	}

	ErrorRollingLogger = &lumberjack.Logger{
		Filename:   filepath.Join(cfg.Dir, "error.log"),
		MaxSize:    maxSize,
		MaxBackups: maxBackups,
		MaxAge:     maxAge,
		Compress:   cfg.Compress,
	}

	// Open permanent unrotated log file
	permanentMu.Lock()
	if permanentLogWriter != nil {
		permanentLogWriter.Close()
	}
	pw, err := os.OpenFile(filepath.Join(cfg.Dir, "permanent_error.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		permanentLogWriter = pw
	}
	permanentMu.Unlock()

	multiWriter := io.MultiWriter(os.Stdout, AppRollingLogger)

	baseHandler := slog.NewTextHandler(multiWriter, &slog.HandlerOptions{
		Level:     GlobalLevelVar,
		AddSource: false,
	})

	customHandler := &MultiLevelHandler{baseHandler: baseHandler}
	slog.SetDefault(slog.New(customHandler))
	return nil
}

// SetLevel updates the current log level dynamically at runtime
func SetLevel(levelStr string) {
	switch strings.ToLower(strings.TrimSpace(levelStr)) {
	case "debug":
		GlobalLevelVar.Set(slog.LevelDebug)
	case "info":
		GlobalLevelVar.Set(slog.LevelInfo)
	case "warn", "warning":
		GlobalLevelVar.Set(slog.LevelWarn)
	case "error":
		GlobalLevelVar.Set(slog.LevelError)
	default:
		GlobalLevelVar.Set(slog.LevelInfo)
	}
}

// GetLevel returns the current log level as a string
func GetLevel() string {
	switch GlobalLevelVar.Level() {
	case slog.LevelDebug:
		return "DEBUG"
	case slog.LevelInfo:
		return "INFO"
	case slog.LevelWarn:
		return "WARN"
	case slog.LevelError:
		return "ERROR"
	default:
		return "INFO"
	}
}

// SetPermanentLevel sets the threshold for permanent unrotated error logging
func SetPermanentLevel(lvl string) {
	permanentMu.Lock()
	defer permanentMu.Unlock()
	cleaned := strings.ToLower(strings.TrimSpace(lvl))
	if cleaned == "" {
		cleaned = "error"
	}
	permanentLevel = cleaned
}

// GetPermanentLevel returns the current permanent log level
func GetPermanentLevel() string {
	permanentMu.Lock()
	defer permanentMu.Unlock()
	return strings.ToUpper(permanentLevel)
}

// GetRecentLogs retrieves the most recent log entries from the in-memory ring buffer
func GetRecentLogs(count int) []string {
	ringMu.RLock()
	defer ringMu.RUnlock()
	if count <= 0 || count > len(recentLogs) {
		count = len(recentLogs)
	}
	res := make([]string, count)
	copy(res, recentLogs[len(recentLogs)-count:])
	return res
}

// GetRecentErrors retrieves the most recent error log entries from the in-memory ring buffer
func GetRecentErrors(count int) []string {
	ringMu.RLock()
	defer ringMu.RUnlock()
	if count <= 0 || count > len(recentErrors) {
		count = len(recentErrors)
	}
	res := make([]string, count)
	copy(res, recentErrors[len(recentErrors)-count:])
	return res
}

// -----------------------------------------------------------------------
// Custom GORM Logger Adapter
// Prevents raw SQL spam and internal details leakage in normal operations.
// SQL is logged ONLY when level is DEBUG, or when a query is slow/erroneous.
// -----------------------------------------------------------------------

type GormLogger struct {
	SlowThreshold time.Duration
}

func NewGormLogger() *GormLogger {
	return &GormLogger{
		SlowThreshold: 200 * time.Millisecond,
	}
}

func (g *GormLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	return g
}

func (g *GormLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	slog.Info(fmt.Sprintf(msg, data...))
}

func (g *GormLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	slog.Warn(fmt.Sprintf(msg, data...))
}

func (g *GormLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	slog.Error(fmt.Sprintf(msg, data...))
}

func (g *GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	elapsed := time.Since(begin)

	// 1. Erroneous queries
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		sql, rows := fc()
		slog.Error("Database query failed",
			"error", err,
			"elapsed", elapsed,
			"rows", rows,
			"sql", sql,
		)
		return
	}

	// 2. Slow queries (above threshold)
	if g.SlowThreshold > 0 && elapsed > g.SlowThreshold {
		sql, rows := fc()
		slog.Warn("Slow database query detected",
			"elapsed", elapsed,
			"rows", rows,
			"sql", sql,
		)
		return
	}

	// 3. Normal queries: ONLY log if current level is DEBUG
	if GlobalLevelVar.Level() <= slog.LevelDebug {
		sql, rows := fc()
		slog.Debug("Executed SQL query",
			"elapsed", elapsed,
			"rows", rows,
			"sql", sql,
		)
	}
}

// -----------------------------------------------------------------------
// Custom Gin HTTP Request Logger Middleware
// -----------------------------------------------------------------------

func GinLoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		rawQuery := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		statusCode := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method

		// Suppress noisy browser probe routes to debug level
		isProbe := strings.HasPrefix(path, "/.well-known") || path == "/favicon.ico"

		if statusCode >= http.StatusInternalServerError {
			slog.Error("HTTP request error",
				"status", statusCode,
				"method", method,
				"path", path,
				"query", rawQuery,
				"ip", clientIP,
				"latency", latency,
			)
		} else if statusCode >= http.StatusBadRequest && !isProbe {
			slog.Warn("HTTP client warning",
				"status", statusCode,
				"method", method,
				"path", path,
				"ip", clientIP,
				"latency", latency,
			)
		} else if isProbe {
			slog.Debug("HTTP probe",
				"status", statusCode,
				"method", method,
				"path", path,
				"ip", clientIP,
				"latency", latency,
			)
		} else {
			slog.Info("HTTP request",
				"status", statusCode,
				"method", method,
				"path", path,
				"ip", clientIP,
				"latency", latency,
			)
		}
	}
}
