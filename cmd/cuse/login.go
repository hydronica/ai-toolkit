package main

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

var (
	loginFindFirefox  = findFirefoxBrowser
	loginFindChromium = findChromiumBrowser
)

type loginEngine string

const (
	loginEngineFirefox  loginEngine = "firefox"
	loginEngineChromium loginEngine = "chromium"
)

type loginBrowser struct {
	engine loginEngine
	path   string
}

type browserChoice struct {
	engine loginEngine
	label  string
}

var loginBrowserOrder = []browserChoice{
	{loginEngineFirefox, "Firefox"},
	{loginEngineChromium, "Chromium-based browser"},
}

const (
	loginURL     = "https://cursor.com/dashboard"
	cookieName   = "WorkosCursorSessionToken"
	loginTimeout = 3 * time.Minute
	pollInterval = time.Second
)

// runLogin opens the system browser and waits for the user to authenticate on
// cursor.com. Once WorkosCursorSessionToken appears in the browser's cookies the
// raw value is returned (without the cookie name prefix). The context should
// carry cancellation from the caller (e.g. Ctrl-C).
//
// preferred is the -browser flag value ("firefox" or "chromium"/"chrome").
// When empty, Firefox is used if installed, otherwise Chromium, otherwise error.
func runLogin(ctx context.Context, preferred string) (string, error) {
	browser, err := resolveLoginBrowser(preferred)
	if err != nil {
		return "", err
	}

	label := "Firefox"
	if browser.engine == loginEngineChromium {
		label = strings.TrimSuffix(filepath.Base(browser.path), ".exe")
		if label == "" {
			label = "Chromium"
		}
	}
	fmt.Printf("Opening %s — please log in at cursor.com...\n", label)

	switch browser.engine {
	case loginEngineFirefox:
		return runFirefoxLogin(ctx, browser.path)
	case loginEngineChromium:
		return runChromiumLogin(ctx, browser.path)
	default:
		return "", fmt.Errorf("unsupported login browser %q", browser.engine)
	}
}

func resolveLoginBrowser(preferred string) (loginBrowser, error) {
	preferred = strings.ToLower(strings.TrimSpace(preferred))
	if preferred == "chrome" {
		preferred = string(loginEngineChromium)
	}

	for _, choice := range loginBrowserOrder {
		if preferred != "" && preferred != string(choice.engine) {
			continue
		}
		var path string
		switch choice.engine {
		case loginEngineFirefox:
			path = loginFindFirefox()
		case loginEngineChromium:
			path = loginFindChromium()
		}
		if path == "" {
			if preferred != "" {
				return loginBrowser{}, fmt.Errorf("%s not found", choice.label)
			}
			continue
		}
		return loginBrowser{engine: choice.engine, path: path}, nil
	}
	if preferred != "" {
		return loginBrowser{}, fmt.Errorf(`invalid -browser value %q (use "firefox" or "chromium")`, preferred)
	}
	return loginBrowser{}, fmt.Errorf("no browser found for login (install Firefox or a Chromium-based browser)")
}

func runChromiumLogin(ctx context.Context, browserPath string) (string, error) {
	// Build allocator options from scratch rather than inheriting
	// DefaultExecAllocatorOptions so that automation-detection flags
	// (--enable-automation, --disable-blink-features=AutomationControlled,
	// --password-store=basic, etc.) are not present. Sites like cursor.com
	// use these flags to detect CDP-driven browsers and show CAPTCHA challenges.
	opts := chromiumLoginAllocatorOptions(browserPath)

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, opts...)
	defer cancelAlloc()

	taskCtx, cancelTask := chromedp.NewContext(allocCtx)
	defer cancelTask()

	if err := chromedp.Run(taskCtx, chromedp.Navigate(loginURL)); err != nil {
		return "", fmt.Errorf("opening browser: %w", err)
	}

	return waitForLoginCookie(ctx, func() (string, error) {
		return extractChromiumCookie(taskCtx)
	})
}

func runFirefoxLogin(ctx context.Context, firefoxPath string) (string, error) {
	if err := openFirefox(firefoxPath, loginURL); err != nil {
		return "", fmt.Errorf("opening browser: %w", err)
	}

	return waitForLoginCookie(ctx, func() (string, error) {
		cookiesPath, err := firefoxCookiesPath()
		if err != nil {
			return "", nil
		}
		cookie, err := readFirefoxCookie(cookiesPath)
		if err != nil {
			return "", fmt.Errorf("reading cookies: %w", err)
		}
		return cookie, nil
	})
}

func waitForLoginCookie(ctx context.Context, readCookie func() (string, error)) (string, error) {
	deadline := time.Now().Add(loginTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		cookie, err := readCookie()
		if err != nil {
			return "", err
		}
		if cookie != "" {
			return cookie, nil
		}
		time.Sleep(pollInterval)
	}

	return "", fmt.Errorf("timed out after %s waiting for login", loginTimeout)
}

// extractChromiumCookie reads WorkosCursorSessionToken from a CDP browser.
// Returns an empty string when the cookie is not yet present.
func extractChromiumCookie(ctx context.Context) (string, error) {
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

// chromiumLoginAllocatorOptions returns chromedp options for opening a visible
// Chromium-based browser for login. GPU acceleration is disabled by default.
func chromiumLoginAllocatorOptions(browserPath string) []chromedp.ExecAllocatorOption {
	flags := chromiumGPUInitFlags()
	opts := make([]chromedp.ExecAllocatorOption, 0, len(flags)+8)
	for name, value := range flags {
		opts = append(opts, chromedp.Flag(name, value))
	}
	opts = append(opts,
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Flag("headless", false),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"),
		chromedp.ExecPath(browserPath),
	)
	if runtime.GOOS == "linux" {
		opts = append(opts,
			chromedp.Flag("no-sandbox", true),
			chromedp.Flag("disable-setuid-sandbox", true),
		)
	}
	return opts
}

// chromiumGPUInitFlags returns Chrome flags that disable hardware acceleration
// while keeping the compositor on a working SwiftShader/ANGLE software path.
// Do not set disable-gpu-compositing: it forces a broken software compositor
// that shows red damage rectangles on some GPUs (common on Brave/Chromium 152).
func chromiumGPUInitFlags() map[string]any {
	flags := map[string]any{
		"disable-gpu":               true,
		"enable-unsafe-swiftshader": true,
		"use-gl":                    "angle",
		"use-angle":                 "swiftshader",
		"disable-dev-shm-usage":     true,
	}
	switch runtime.GOOS {
	case "windows":
		flags["disable-direct-composition"] = true
	case "darwin":
		flags["disable-features"] = "Vulkan"
	}
	return flags
}
