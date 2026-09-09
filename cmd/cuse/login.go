package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

var ErrLoginBrowserClosed = errors.New("login browser closed")

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

// loginSession owns a dedicated browser opened for cursor.com login.
// Call WaitForCookie, persist the cookie, then Close.
type loginSession struct {
	ctx        context.Context
	cancel     context.CancelCauseFunc
	closeOnce  sync.Once
	closeFn    func()
	readCookie func(context.Context) (string, error)
}

// Close shuts down the browser instance this session opened.
func (s *loginSession) Close() {
	s.closeOnce.Do(func() {
		if s.closeFn != nil {
			s.closeFn()
		}
	})
}

// WaitForCookie polls until WorkosCursorSessionToken appears or the session ends.
func (s *loginSession) WaitForCookie() (string, error) {
	return waitForLoginCookie(s.ctx, s.readCookie)
}

// runLogin opens the system browser for cursor.com login and returns a session.
// The caller's context should carry cancellation (e.g. Ctrl-C).
//
// preferred is the -browser flag value ("firefox" or "chromium"/"chrome").
// When empty, Firefox is used if installed, otherwise Chromium, otherwise error.
func runLogin(ctx context.Context, preferred string) (*loginSession, error) {
	browser, err := resolveLoginBrowser(preferred)
	if err != nil {
		return nil, err
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
		return newFirefoxSession(ctx, browser.path)
	case loginEngineChromium:
		return newChromiumSession(ctx, browser.path)
	default:
		return nil, fmt.Errorf("unsupported login browser %q", browser.engine)
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

func newFirefoxSession(parent context.Context, firefoxPath string) (*loginSession, error) {
	loginCtx, cancel := context.WithCancelCause(parent)

	cmd, err := openFirefox(firefoxPath, loginURL)
	if err != nil {
		cancel(err)
		return nil, fmt.Errorf("opening browser: %w", err)
	}

	exited := make(chan struct{})
	go func() {
		defer close(exited)
		_ = cmd.Wait()
		cancel(ErrLoginBrowserClosed)
	}()

	var shutdown sync.Once
	closeFn := func() {
		shutdown.Do(func() {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			<-exited
		})
	}

	return &loginSession{
		ctx:    loginCtx,
		cancel: cancel,
		closeFn: closeFn,
		readCookie: func(context.Context) (string, error) {
			cookiesPath, err := firefoxCookiesPath()
			if err != nil {
				return "", nil
			}
			cookie, err := readFirefoxCookie(cookiesPath)
			if err != nil {
				return "", fmt.Errorf("reading cookies: %w", err)
			}
			return cookie, nil
		},
	}, nil
}

func newChromiumSession(parent context.Context, browserPath string) (*loginSession, error) {
	loginCtx, cancel := context.WithCancelCause(parent)

	// Build allocator options from scratch rather than inheriting
	// DefaultExecAllocatorOptions, which enables headless mode and
	// --enable-automation (sites like cursor.com may treat that as a bot).
	opts := chromiumLoginAllocatorOptions(browserPath)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(loginCtx, opts...)
	taskCtx, cancelTask := chromedp.NewContext(allocCtx)

	var shutdown sync.Once
	closeFn := func() {
		shutdown.Do(func() {
			cancelTask()
			cancelAlloc()
		})
	}

	wireChromiumCloseSignals(taskCtx, cancel)

	if err := chromedp.Run(taskCtx, chromedp.Navigate(loginURL)); err != nil {
		closeFn()
		cancel(err)
		return nil, fmt.Errorf("opening browser: %w", err)
	}

	return &loginSession{
		ctx:     loginCtx,
		cancel:  cancel,
		closeFn: closeFn,
		readCookie: func(context.Context) (string, error) {
			cookie, err := extractChromiumCookie(taskCtx)
			if err != nil && isChromiumSessionLost(err) {
				return "", ErrLoginBrowserClosed
			}
			return cookie, err
		},
	}, nil
}

func waitForLoginCookie(ctx context.Context, readCookie func(context.Context) (string, error)) (string, error) {
	deadline := time.Now().Add(loginTimeout)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		if err := ctx.Err(); err != nil {
			return "", loginContextErr(ctx)
		}

		cookie, err := readCookie(ctx)
		if errors.Is(err, ErrLoginBrowserClosed) {
			return "", err
		}
		if err != nil {
			return "", err
		}
		if cookie != "" {
			return cookie, nil
		}

		if time.Now().After(deadline) {
			break
		}

		select {
		case <-ctx.Done():
			return "", loginContextErr(ctx)
		case <-ticker.C:
		}
	}

	return "", fmt.Errorf("timed out after %s waiting for login", loginTimeout)
}

func loginContextErr(ctx context.Context) error {
	if cause := context.Cause(ctx); errors.Is(cause, ErrLoginBrowserClosed) {
		return ErrLoginBrowserClosed
	}
	return ctx.Err()
}

func wireChromiumCloseSignals(taskCtx context.Context, cancel context.CancelCauseFunc) {
	c := chromedp.FromContext(taskCtx)
	if c == nil || c.Browser == nil {
		return
	}

	var (
		loginTargetID  target.ID
		loginSessionID target.SessionID
		signalOnce     sync.Once
	)
	if c.Target != nil {
		loginTargetID = c.Target.TargetID
		loginSessionID = c.Target.SessionID
	}

	signalClosed := func() {
		signalOnce.Do(func() { cancel(ErrLoginBrowserClosed) })
	}

	if proc := c.Browser.Process(); proc != nil {
		go func() {
			_, _ = proc.Wait()
			signalClosed()
		}()
	}

	go func() {
		<-c.Browser.LostConnection
		signalClosed()
	}()

	go func() {
		<-taskCtx.Done()
		signalClosed()
	}()

	chromedp.ListenBrowser(taskCtx, func(ev any) {
		switch ev := ev.(type) {
		case *target.EventTargetDestroyed:
			if loginTargetID != "" && ev.TargetID == loginTargetID {
				signalClosed()
			}
		case *target.EventDetachedFromTarget:
			if loginSessionID != "" && ev.SessionID == loginSessionID {
				signalClosed()
			}
		}
	})
}

func isChromiumSessionLost(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, chromedp.ErrChannelClosed) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "target closed") ||
		strings.Contains(msg, "connection closed") ||
		strings.Contains(msg, "websocket") ||
		strings.Contains(msg, "not attached") ||
		strings.Contains(msg, "browser has gone")
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

// chromiumLoginAllocatorOptions returns chromedp options for a visible login
// window. GPU flags are applied separately; other flags are limited to skipping
// first-run prompts and selecting the browser executable.
func chromiumLoginAllocatorOptions(browserPath string) []chromedp.ExecAllocatorOption {
	flags := chromiumGPUInitFlags()
	opts := make([]chromedp.ExecAllocatorOption, 0, len(flags)+5)
	for name, value := range flags {
		opts = append(opts, chromedp.Flag(name, value))
	}
	opts = append(opts,
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Flag("headless", false),
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
