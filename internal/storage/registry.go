package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

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

func isRestrictedHost(host string) bool {
	h := strings.TrimSpace(host)
	if strings.Contains(h, ":") {
		if sh, _, err := net.SplitHostPort(h); err == nil {
			h = sh
		}
	}
	h = strings.Trim(h, "[]")
	if strings.EqualFold(h, "metadata.google.internal") || strings.HasSuffix(strings.ToLower(h), ".metadata.google.internal") {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if ips, err := net.DefaultResolver.LookupIP(ctx, "ip", h); err == nil {
		for _, ip := range ips {
			if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
				return true
			}
		}
	}
	return false
}

func isPolicyConfigRestricted(configJSON string) bool {
	if configJSON == "" {
		return false
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(configJSON), &raw); err != nil {
		return false
	}
	for _, key := range []string{"endpoint", "url", "host", "domain"} {
		val, ok := raw[key].(string)
		if !ok || strings.TrimSpace(val) == "" {
			continue
		}
		targetHost := val
		if strings.HasPrefix(val, "http://") || strings.HasPrefix(val, "https://") {
			if u, err := url.Parse(val); err == nil {
				targetHost = u.Hostname()
			}
		}
		if isRestrictedHost(targetHost) {
			return true
		}
	}
	return false
}

func BuildDriver(ctx context.Context, policy model.StoragePolicy, db *gorm.DB) (StorageDriver, error) {
	if isPolicyConfigRestricted(policy.Config) {
		return nil, fmt.Errorf("restricted storage host address in policy configuration")
	}
	f, ok := GetDriverFactory(policy.Driver)
	if !ok {
		return nil, fmt.Errorf("unsupported storage driver: %s", policy.Driver)
	}
	return f(ctx, policy, db)
}
