package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"

	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"gorm.io/gorm"
)

type cachedDriver struct {
	driver     StorageDriver
	configHash string
}

type PolicyDriverManager struct {
	db      *gorm.DB
	mu      sync.RWMutex
	drivers map[uint]*cachedDriver
}

func NewPolicyDriverManager(db *gorm.DB) *PolicyDriverManager {
	return &PolicyDriverManager{
		db:      db,
		drivers: make(map[uint]*cachedDriver),
	}
}

func (m *PolicyDriverManager) SetDB(db *gorm.DB) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.db = db
	m.drivers = make(map[uint]*cachedDriver)
}

func (m *PolicyDriverManager) Invalidate(policyID uint) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.drivers, policyID)
}

func (m *PolicyDriverManager) InvalidateAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.drivers = make(map[uint]*cachedDriver)
}

func hashConfig(p *model.StoragePolicy) string {
	raw := fmt.Sprintf("%s:%s", p.Driver, p.Config)
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func (m *PolicyDriverManager) GetDriver(ctx context.Context, policyID uint) (StorageDriver, *model.StoragePolicy, error) {
	if m.db == nil {
		return nil, nil, fmt.Errorf("database not initialized")
	}

	var policy model.StoragePolicy
	if err := m.db.Where("id = ?", policyID).First(&policy).Error; err != nil {
		return nil, nil, fmt.Errorf("storage policy %d not found: %w", policyID, err)
	}

	cfgHash := hashConfig(&policy)

	m.mu.RLock()
	cached, ok := m.drivers[policyID]
	if ok && cached.configHash == cfgHash {
		drv := cached.driver
		m.mu.RUnlock()
		return drv, &policy, nil
	}
	m.mu.RUnlock()

	factory, ok := GetDriverFactory(policy.Driver)
	if !ok {
		return nil, &policy, fmt.Errorf("unsupported storage driver: %s", policy.Driver)
	}

	drv, err := factory(ctx, policy, m.db)
	if err != nil {
		return nil, &policy, fmt.Errorf("failed to initialize driver %s for policy %s: %w", policy.Driver, policy.Name, err)
	}

	if tr, ok := drv.(TokenRefresher); ok {
		currentPolicyID := policy.ID
		tr.SetTokenRefreshCallback(func(newConfig string) error {
			if m.db == nil {
				return fmt.Errorf("database not available")
			}
			err := m.db.Model(&model.StoragePolicy{}).Where("id = ?", currentPolicyID).Update("config", newConfig).Error
			if err != nil {
				slog.Error("Failed to persist refreshed storage policy token", "policy_id", currentPolicyID, "error", err)
				return err
			}
			m.mu.Lock()
			if c, exists := m.drivers[currentPolicyID]; exists {
				var updatedPolicy model.StoragePolicy
				if err := m.db.Where("id = ?", currentPolicyID).First(&updatedPolicy).Error; err == nil {
					c.configHash = hashConfig(&updatedPolicy)
				}
			}
			m.mu.Unlock()
			return nil
		})
	}

	m.mu.Lock()
	m.drivers[policyID] = &cachedDriver{
		driver:     drv,
		configHash: cfgHash,
	}
	m.mu.Unlock()

	return drv, &policy, nil
}

func (m *PolicyDriverManager) BuildAdHocDriver(ctx context.Context, policy model.StoragePolicy) (StorageDriver, error) {
	return BuildDriver(ctx, policy, m.db)
}
