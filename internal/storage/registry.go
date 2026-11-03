package storage

import (
	"context"
	"fmt"
	"sync"

	"github.com/UtopiaXC/UtopiaImages/internal/model"
	"gorm.io/gorm"
)

type DriverFactoryFunc func(ctx context.Context, policy model.StoragePolicy, db *gorm.DB) (StorageDriver, error)

var (
	registryMu sync.RWMutex
	factories  = make(map[string]DriverFactoryFunc)
)

func RegisterDriver(driverName string, factory DriverFactoryFunc) {
	registryMu.Lock()
	defer registryMu.Unlock()
	factories[driverName] = factory
}

func GetDriverFactory(driverName string) (DriverFactoryFunc, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	f, ok := factories[driverName]
	return f, ok
}

func SupportedDrivers() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	list := make([]string, 0, len(factories))
	for k := range factories {
		list = append(list, k)
	}
	return list
}

func BuildDriver(ctx context.Context, policy model.StoragePolicy, db *gorm.DB) (StorageDriver, error) {
	f, ok := GetDriverFactory(policy.Driver)
	if !ok {
		return nil, fmt.Errorf("unsupported storage driver: %s", policy.Driver)
	}
	return f(ctx, policy, db)
}
