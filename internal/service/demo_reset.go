package service

import (
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/UtopiaXC/UtopiaImages/internal/config"
	appLogger "github.com/UtopiaXC/UtopiaImages/internal/logger"
	"github.com/UtopiaXC/UtopiaImages/internal/repository"
	"gorm.io/gorm"
)

const DemoResetInterval = 30 * time.Minute

type DemoStatus struct {
	ResetEpoch      int64 `json:"reset_epoch"`
	LastResetAt     int64 `json:"last_reset_at"`
	NextResetAt     int64 `json:"next_reset_at"`
	ServerTime      int64 `json:"server_time"`
	RemainingMs     int64 `json:"remaining_ms"`
	IntervalSeconds int64 `json:"interval_seconds"`
	IsResetting     bool  `json:"is_resetting"`
}

type configSnapshot struct {
	Site    config.SiteConfig
	Server  config.ServerConfig
	Cache   config.CacheConfig
	Logging config.LoggingConfig
}

type DemoResetService struct {
	cfg        *config.Config
	configPath string
	snapshot   configSnapshot

	getDB      func() *gorm.DB
	onDBChange func(*gorm.DB)
	onPreReset func()

	mu          sync.RWMutex
	resetMu     sync.Mutex
	resetEpoch  int64
	lastResetAt time.Time
	nextResetAt time.Time
	isResetting atomic.Bool

	subMu       sync.Mutex
	subscribers map[chan DemoStatus]struct{}
	stopCh      chan struct{}
}

func NewDemoResetService(
	cfg *config.Config,
	configPath string,
	initialLastResetAt time.Time,
	getDB func() *gorm.DB,
	onPreReset func(),
	onDBChange func(*gorm.DB),
) *DemoResetService {
	if initialLastResetAt.IsZero() {
		initialLastResetAt = time.Now()
	}
	var snap configSnapshot
	if cfg != nil {
		snap = configSnapshot{
			Site:    cfg.Site,
			Server:  cfg.Server,
			Cache:   cfg.Cache,
			Logging: cfg.Logging,
		}
	}
	svc := &DemoResetService{
		cfg:         cfg,
		configPath:  configPath,
		snapshot:    snap,
		getDB:       getDB,
		onPreReset:  onPreReset,
		onDBChange:  onDBChange,
		resetEpoch:  1,
		lastResetAt: initialLastResetAt,
		nextResetAt: initialLastResetAt.Add(DemoResetInterval),
		subscribers: make(map[chan DemoStatus]struct{}),
		stopCh:      make(chan struct{}),
	}
	go svc.runLoop()
	return svc
}

func (s *DemoResetService) Stop() {
	select {
	case <-s.stopCh:
	default:
		close(s.stopCh)
	}
}

func (s *DemoResetService) IsResetting() bool {
	return s.isResetting.Load()
}

func (s *DemoResetService) GetStatus() DemoStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	rem := s.nextResetAt.Sub(now).Milliseconds()
	if rem < 0 {
		rem = 0
	}
	return DemoStatus{
		ResetEpoch:      s.resetEpoch,
		LastResetAt:     s.lastResetAt.UnixMilli(),
		NextResetAt:     s.nextResetAt.UnixMilli(),
		ServerTime:      now.UnixMilli(),
		RemainingMs:     rem,
		IntervalSeconds: int64(DemoResetInterval.Seconds()),
		IsResetting:     s.isResetting.Load(),
	}
}

func (s *DemoResetService) Subscribe() chan DemoStatus {
	ch := make(chan DemoStatus, 8)
	s.subMu.Lock()
	s.subscribers[ch] = struct{}{}
	s.subMu.Unlock()
	return ch
}

func (s *DemoResetService) Unsubscribe(ch chan DemoStatus) {
	s.subMu.Lock()
	if _, ok := s.subscribers[ch]; ok {
		delete(s.subscribers, ch)
		close(ch)
	}
	s.subMu.Unlock()
}

func (s *DemoResetService) broadcast(status DemoStatus) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	for ch := range s.subscribers {
		select {
		case ch <- status:
		default:
		}
	}
}

func (s *DemoResetService) runLoop() {
	for {
		s.mu.RLock()
		waitDur := time.Until(s.nextResetAt)
		s.mu.RUnlock()
		if waitDur < 0 {
			waitDur = 0
		}

		timer := time.NewTimer(waitDur)
		select {
		case <-s.stopCh:
			timer.Stop()
			return
		case <-timer.C:
			if err := s.ResetNow(); err != nil {
				slog.Error("Demo periodic reset failed", "error", err)
			}
		}
	}
}

// clearDirectoryContents deletes all files/subfolders inside dir without removing dir itself.
func clearDirectoryContents(dir string) error {
	if dir == "" {
		return nil
	}
	cleanDir := filepath.Clean(dir)
	if cleanDir == "." || cleanDir == "/" {
		return nil
	}
	entries, err := os.ReadDir(cleanDir)
	if err != nil {
		if os.IsNotExist(err) {
			return os.MkdirAll(cleanDir, 0750)
		}
		return err
	}
	for _, e := range entries {
		_ = os.RemoveAll(filepath.Join(cleanDir, e.Name()))
	}
	return nil
}

// ClearStorageAndCache removes all runtime files in local storage and derived/chunk cache directories.
func ClearStorageAndCache(cfg *config.Config) {
	if cfg == nil {
		return
	}
	if cfg.Storage.Local.BasePath != "" {
		if err := clearDirectoryContents(cfg.Storage.Local.BasePath); err != nil {
			slog.Warn("Failed to clear storage directory during demo reset", "error", err)
		}
	}
	if cfg.Cache.Dir != "" {
		if err := clearDirectoryContents(cfg.Cache.Dir); err != nil {
			slog.Warn("Failed to clear cache directory during demo reset", "error", err)
		}
		_ = os.MkdirAll(filepath.Join(filepath.Clean(cfg.Cache.Dir), "chunks"), 0755)
	}
}

// ResetNow clears storage and cache directories, resets database to initial demo state,
// and notifies all connected frontend clients.
func (s *DemoResetService) ResetNow() error {
	s.resetMu.Lock()
	defer s.resetMu.Unlock()

	s.isResetting.Store(true)
	defer s.isResetting.Store(false)

	slog.Warn("Executing scheduled 30-minute demo reset...")

	if s.onPreReset != nil {
		s.onPreReset()
	}

	// Restore initial config snapshot
	if s.cfg != nil {
		s.cfg.Site = s.snapshot.Site
		s.cfg.Server = s.snapshot.Server
		s.cfg.Cache = s.snapshot.Cache
		s.cfg.Logging = s.snapshot.Logging
		s.cfg.SMTP.Enabled = false
		s.cfg.SMTP.Host = ""
		s.cfg.SMTP.User = ""
		s.cfg.SMTP.Password = ""
		s.cfg.SMTP.From = ""

		appLogger.SetLevel(s.cfg.Logging.Level)
		appLogger.SetPermanentLevel(s.cfg.Logging.PermanentLevel)

		if s.configPath != "" {
			_ = config.SaveConfig(s.configPath, s.cfg)
		}
	}

	// 1. Reset database back to initial demo state
	var curDB *gorm.DB
	if s.getDB != nil {
		curDB = s.getDB()
	}
	newDB, err := repository.ResetDatabase(s.cfg, curDB)
	if err != nil {
		return err
	}

	// 2. Clear local storage & cache directories
	ClearStorageAndCache(s.cfg)

	// 4. Propagate fresh DB handle to server and all controllers/services
	if s.onDBChange != nil {
		s.onDBChange(newDB)
	}

	now := time.Now()
	s.mu.Lock()
	s.resetEpoch++
	s.lastResetAt = now
	s.nextResetAt = now.Add(DemoResetInterval)
	s.mu.Unlock()

	status := s.GetStatus()
	slog.Info("Scheduled demo reset completed", "reset_epoch", status.ResetEpoch, "next_reset_at", s.nextResetAt.Format(time.RFC3339))

	s.broadcast(status)
	return nil
}
