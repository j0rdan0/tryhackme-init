package browser

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/joakimcarlsson/bonk"
)

var SESSION_FILE string

func init() {
	if _, err := os.Stat("session.dat"); err == nil {
		SESSION_FILE = "session.dat"
		return
	}
	SESSION_FILE = filepath.Join(os.TempDir(), "session.dat")
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

	return b, ctx, page, nil
}

func CleanScreenshots() {
	_ = os.Remove(filepath.Join(os.TempDir(), "headless-screenshot.png"))
	_ = os.Remove(filepath.Join(os.TempDir(), "debug-screenshot.png"))
}
