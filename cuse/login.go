package main

import (
	"context"
	"fmt"
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

// runLogin opens the system browser and waits for the user to authenticate on
// cursor.com. Once WorkosCursorSessionToken appears in the browser's cookies the
// raw value is returned (without the cookie name prefix). The context should
// carry cancellation from the caller (e.g. Ctrl-C).
func runLogin(ctx context.Context) (string, error) {
	fmt.Println("Opening browser — please log in at cursor.com...")

	strategy, browserPath, err := chooseLoginStrategy()
	if err != nil {
		return "", err
	}

	switch strategy {
	case loginStrategyChromium:
		return runChromiumLogin(ctx, browserPath)
	case loginStrategyFirefox:
		return runFirefoxLogin(ctx)
	default:
		return "", fmt.Errorf("unsupported login strategy")
	}
}

func runChromiumLogin(ctx context.Context, browserPath string) (string, error) {
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
		chromedp.ExecPath(browserPath),
	}

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

func runFirefoxLogin(ctx context.Context) (string, error) {
	cookiesPath, err := firefoxCookiesPath()
	if err != nil {
		return "", fmt.Errorf("locating Firefox cookies: %w", err)
	}

	if err := openURL(loginURL); err != nil {
		return "", fmt.Errorf("opening browser: %w", err)
	}

	return waitForLoginCookie(ctx, func() (string, error) {
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
