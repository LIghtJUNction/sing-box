package constant

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/sagernet/sing/common/rw"
)

const dirName = "sing-box"

var (
	resourcePaths      []string
	resourcePathsMutex sync.RWMutex
)

func AddResourcePath(path string) {
	if path == "" {
		return
	}
	resourcePathsMutex.Lock()
	defer resourcePathsMutex.Unlock()
	for _, resourcePath := range resourcePaths {
		if resourcePath == path {
			return
		}
	}
	resourcePaths = append(resourcePaths, path)
}

func FindPath(name string) (string, bool) {
	name = os.ExpandEnv(name)
	if rw.IsFile(name) {
		return name, true
	}
	resourcePathsMutex.RLock()
	paths := append([]string(nil), resourcePaths...)
	resourcePathsMutex.RUnlock()
	for _, dir := range paths {
		if path := filepath.Join(dir, dirName, name); rw.IsFile(path) {
			return path, true
		}
		if path := filepath.Join(dir, name); rw.IsFile(path) {
			return path, true
		}
	}
	return name, false
}

func init() {
	AddResourcePath(".")
	if home := os.Getenv("HOME"); home != "" {
		AddResourcePath(home)
	}
	if userConfigDir, err := os.UserConfigDir(); err == nil {
		AddResourcePath(userConfigDir)
	}
	if userCacheDir, err := os.UserCacheDir(); err == nil {
		AddResourcePath(userCacheDir)
	}
}
