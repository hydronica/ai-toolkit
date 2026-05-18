package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

const (
	loginURL     = "https://cursor.com/dashboard"
	cookieName   = "WorkosCursorSessionToken"
	loginTimeout = 3 * time.Minute
	pollInterval = time.Second
)

// runLogin opens a visible Chrome or Edge window and waits for the user to
// authenticate on cursor.com. Once WorkosCursorSessionToken appears in the
// browser's cookies the raw value is returned (without the cookie name prefix).
// The context should carry cancellation from the caller (e.g. Ctrl-C).
func runLogin(ctx context.Context) (string, error) {
	fmt.Println("Opening browser — please log in at cursor.com...")

	// Build allocator options from scratch rather than inheriting
	// DefaultExecAllocatorOptions so that automation-detection flags
	// (--enable-automation, --disable-blink-features=AutomationControlled,
	// --password-store=basic, etc.) are not present. Sites like cursor.com
	// use these flags to detect CDP-driven browsers and show CAPTCHA challenges.
	opts := []chromedp.ExecAllocatorOption{
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Flag("headless", false),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-setuid-sandbox", true),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"),
	}

	// Prefer Chrome; fall back to Edge on Windows.
	if browserPath := findBrowser(); browserPath != "" {
		opts = append(opts, chromedp.ExecPath(browserPath))
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, opts...)
	defer cancelAlloc()

	taskCtx, cancelTask := chromedp.NewContext(allocCtx)
	defer cancelTask()

	if err := chromedp.Run(taskCtx, chromedp.Navigate(loginURL)); err != nil {
		return "", fmt.Errorf("opening browser: %w", err)
	}

	deadline := time.Now().Add(loginTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		cookie, err := extractCookie(taskCtx)
		if err != nil {
			return "", fmt.Errorf("reading cookies: %w", err)
		}
		if cookie != "" {
			return cookie, nil
		}
		time.Sleep(pollInterval)
	}

	return "", fmt.Errorf("timed out after %s waiting for login", loginTimeout)
}

// extractCookie reads WorkosCursorSessionToken from the browser cookie store.
// Returns an empty string when the cookie is not yet present.
func extractCookie(ctx context.Context) (string, error) {
	var cookies []*network.Cookie
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var innerErr error
		cookies, innerErr = network.GetCookies().Do(ctx)
		return innerErr
	}))
	if err != nil {
		return "", err
	}
	for _, c := range cookies {
		if c.Name == cookieName && c.Value != "" {
			return c.Value, nil
		}
	}
	return "", nil
}

// findBrowser returns the path to a Chromium-compatible browser executable.
// Priority: Chrome > Brave > Edge. Returns "" if none found (chromedp will
// then fall back to its own PATH search).
func findBrowser() string {
	var candidates []string
	if runtime.GOOS == "windows" {
		userProfile := os.Getenv("USERPROFILE")
		candidates = []string{
			// Chrome — system-wide and per-user
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			filepath.Join(userProfile, `AppData\Local\Google\Chrome\Application\chrome.exe`),
			// Brave — system-wide and per-user
			`C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe`,
			`C:\Program Files (x86)\BraveSoftware\Brave-Browser\Application\brave.exe`,
			filepath.Join(userProfile, `AppData\Local\BraveSoftware\Brave-Browser\Application\brave.exe`),
			// Edge — last resort
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
			filepath.Join(userProfile, `AppData\Local\Microsoft\Edge\Application\msedge.exe`),
		}
	} else if runtime.GOOS == "darwin" {
		userHome := os.Getenv("HOME")
		candidates = []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
			filepath.Join(userHome, "Applications/Google Chrome.app/Contents/MacOS/Google Chrome"),
			filepath.Join(userHome, "Applications/Brave Browser.app/Contents/MacOS/Brave Browser"),
		}
	} else {
		// Linux
		candidates = []string{
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/brave-browser",
			"/usr/bin/brave-browser-stable",
			"/snap/bin/chromium",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
		}
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
