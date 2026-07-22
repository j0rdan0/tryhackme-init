package browser

import (
	"encoding/json"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/joakimcarlsson/bonk"
)

var SESSION_FILE string

func init() {
	SESSION_FILE = getSessionFilePath()
}

func getSessionFilePath() string {
	searchDirs := []string{".", ".."}

	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		searchDirs = append(searchDirs, exeDir, filepath.Dir(exeDir))
	}

	for _, dir := range searchDirs {
		p := filepath.Join(dir, "session.dat")
		if _, err := os.Stat(p); err == nil {
			if absPath, err := filepath.Abs(p); err == nil {
				return absPath
			}
		}
	}

	for _, dir := range searchDirs {
		p := filepath.Join(dir, "vpn.sh")
		if _, err := os.Stat(p); err == nil {
			if absPath, err := filepath.Abs(p); err == nil {
				return filepath.Join(filepath.Dir(absPath), "session.dat")
			}
		}
	}

	return filepath.Join(os.TempDir(), "session.dat")
}

type SavedState struct {
	Cookies []bonk.Cookie `json:"cookies"`
}

// Launch launches Chrome and restores cookies if session file exists
func Launch(headless bool) (*bonk.Browser, *bonk.BrowserContext, *bonk.Page, error) {
	b, err := bonk.Launch(
		bonk.Headless(headless),
		bonk.Args(
			"--disable-blink-features=AutomationControlled",
			"--disable-infobars",
			"--disable-session-crashed-bubble",
		),
	)
	if err != nil {
		return nil, nil, nil, err
	}

	ctx, err := b.NewContext()
	if err != nil {
		b.Close()
		return nil, nil, nil, err
	}

	page, err := ctx.NewPage()
	if err != nil {
		b.Close()
		return nil, nil, nil, err
	}

	// Restore cookies if session file exists
	if _, err := os.Stat(SESSION_FILE); err == nil {
		data, err := os.ReadFile(SESSION_FILE)
		if err == nil {
			var state SavedState
			if err := json.Unmarshal(data, &state); err == nil && len(state.Cookies) > 0 {
				_ = ctx.SetCookies(state.Cookies...)
			}
		}
	}

	registerBrowser(b)

	return b, ctx, page, nil
}

var (
	activeBrowsers   []*bonk.Browser
	activeBrowsersMu sync.Mutex
	signalOnce       sync.Once
)

func registerBrowser(b *bonk.Browser) {
	activeBrowsersMu.Lock()
	activeBrowsers = append(activeBrowsers, b)
	activeBrowsersMu.Unlock()

	b.OnDisconnect(func() {
		unregisterBrowser(b)
	})

	signalOnce.Do(func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
		go func() {
			<-sigChan
			CloseAll()
			os.Exit(1)
		}()
	})
}

func unregisterBrowser(b *bonk.Browser) {
	activeBrowsersMu.Lock()
	defer activeBrowsersMu.Unlock()
	for i, ab := range activeBrowsers {
		if ab == b {
			activeBrowsers = append(activeBrowsers[:i], activeBrowsers[i+1:]...)
			break
		}
	}
}

// CloseAll closes all active browsers launched by this package.
func CloseAll() {
	activeBrowsersMu.Lock()
	defer activeBrowsersMu.Unlock()
	for _, b := range activeBrowsers {
		_ = b.Close()
	}
	activeBrowsers = nil
}

func CleanScreenshots() {
	_ = os.Remove(filepath.Join(os.TempDir(), "headless-screenshot.png"))
	_ = os.Remove(filepath.Join(os.TempDir(), "debug-screenshot.png"))
}
