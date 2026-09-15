package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/hydronica/trial"
)

func TestResolveLoginBrowser(t *testing.T) {
	type input struct {
		preferred    string
		firefoxPath  string
		chromiumPath string
	}

	origFirefox := loginFindFirefox
	origChromium := loginFindChromium
	t.Cleanup(func() {
		loginFindFirefox = origFirefox
		loginFindChromium = origChromium
	})

	fn := func(in input) (loginBrowser, error) {
		loginFindFirefox = func() string { return in.firefoxPath }
		loginFindChromium = func() string { return in.chromiumPath }
		return resolveLoginBrowser(in.preferred)
	}
	cases := trial.Cases[input, loginBrowser]{
		"default prefers firefox": {
			Input: input{
				firefoxPath:  "/usr/bin/firefox",
				chromiumPath: "/usr/bin/chromium",
			},
			Expected: loginBrowser{engine: loginEngineFirefox, path: "/usr/bin/firefox"},
		},
		"default falls back to chromium": {
			Input: input{
				chromiumPath: "/usr/bin/chromium",
			},
			Expected: loginBrowser{engine: loginEngineChromium, path: "/usr/bin/chromium"},
		},
		"explicit firefox missing": {
			Input: input{
				preferred:    "firefox",
				chromiumPath: "/usr/bin/chromium",
			},
			ExpectedErr: errors.New("Firefox not found"),
		},
		"chrome alias": {
			Input: input{
				preferred:    "chrome",
				firefoxPath:  "/usr/bin/firefox",
				chromiumPath: "/usr/bin/google-chrome",
			},
			Expected: loginBrowser{engine: loginEngineChromium, path: "/usr/bin/google-chrome"},
		},
		"invalid browser": {
			Input: input{
				preferred: "safari",
			},
			ExpectedErr: errors.New(`invalid -browser value "safari" (use "firefox" or "chromium")`),
		},
		"no browsers installed": {
			Input:       input{},
			ExpectedErr: errors.New("no browser found for login (install Firefox or a Chromium-based browser)"),
		},
	}
	trial.New(fn, cases).Comparer(func(actual, expected interface{}) (bool, string) {
		got := actual.(loginBrowser)
		want := expected.(loginBrowser)
		if got == want {
			return true, ""
		}
		return false, fmt.Sprintf("got %+v want %+v", got, want)
	}).SubTest(t)
}

func TestChromiumGPUInitFlags(t *testing.T) {
	fn := func(check string) (string, error) {
		flags := chromiumGPUInitFlags()
		switch check {
		case "disable-gpu":
			if flags["disable-gpu"] != true {
				return "", errors.New("expected disable-gpu")
			}
		case "swiftshader":
			if flags["enable-unsafe-swiftshader"] != true {
				return "", errors.New("expected enable-unsafe-swiftshader for Chromium 139+")
			}
		case "angle-path":
			if flags["use-gl"] != "angle" || flags["use-angle"] != "swiftshader" {
				return "", fmt.Errorf("expected SwiftShader ANGLE path, got use-gl=%v use-angle=%v", flags["use-gl"], flags["use-angle"])
			}
		case "no-compositing":
			if flags["disable-gpu-compositing"] == true {
				return "", errors.New("disable-gpu-compositing causes red compositor artifacts")
			}
		default:
			return "", fmt.Errorf("unknown check %q", check)
		}
		return "ok", nil
	}
	cases := trial.Cases[string, string]{
		"disables gpu":                    {Input: "disable-gpu", Expected: "ok"},
		"enables swiftshader":             {Input: "swiftshader", Expected: "ok"},
		"uses swiftshader angle path":     {Input: "angle-path", Expected: "ok"},
		"rejects disable-gpu-compositing": {Input: "no-compositing", Expected: "ok"},
	}
	trial.New(fn, cases).SubTest(t)
}

func TestWaitForLoginCookieBrowserClosed(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(ErrLoginBrowserClosed)
	_, err := waitForLoginCookie(ctx, func(context.Context) (string, error) {
		return "", nil
	}, nil)
	if !errors.Is(err, ErrLoginBrowserClosed) {
		t.Fatalf("got %v want ErrLoginBrowserClosed", err)
	}
}

func TestFirefoxSessionIgnoresLauncherExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses shell script")
	}

	script := filepath.Join(t.TempDir(), "fake-firefox.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	session, err := newFirefoxSession(context.Background(), script, nil)
	if err != nil {
		t.Fatalf("newFirefoxSession: %v", err)
	}
	defer session.Close()

	deadline := time.Now().Add(time.Second)
	for session.ctx.Err() == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if session.ctx.Err() != nil {
		t.Fatalf("context cancelled on launcher exit: %v", context.Cause(session.ctx))
	}
}

func TestWaitForLoginCookieIgnoresStale(t *testing.T) {
	stale := "old-session-token"
	calls := 0
	ctx := context.Background()
	got, err := waitForLoginCookie(ctx, func(context.Context) (string, error) {
		calls++
		if calls < 2 {
			return stale, nil
		}
		return "fresh-session-token", nil
	}, []string{stale})
	if err != nil {
		t.Fatalf("waitForLoginCookie: %v", err)
	}
	if got != "fresh-session-token" {
		t.Fatalf("got %q want fresh-session-token", got)
	}
	if calls != 2 {
		t.Fatalf("readCookie calls = %d want 2", calls)
	}
}

func TestAppendRejectCookie(t *testing.T) {
	fn := func(in struct {
		reject []string
		cookie string
	}) ([]string, error) {
		return appendRejectCookie(in.reject, in.cookie), nil
	}
	cases := trial.Cases[struct {
		reject []string
		cookie string
	}, []string]{
		"adds token without prefix": {
			Input: struct {
				reject []string
				cookie string
			}{
				cookie: "WorkosCursorSessionToken=abc123",
			},
			Expected: []string{"abc123"},
		},
		"deduplicates": {
			Input: struct {
				reject []string
				cookie string
			}{
				reject: []string{"abc123"},
				cookie: "abc123",
			},
			Expected: []string{"abc123"},
		},
		"skips empty": {
			Input: struct {
				reject []string
				cookie string
			}{
				reject: []string{"keep"},
				cookie: "",
			},
			Expected: []string{"keep"},
		},
	}
	trial.New(fn, cases).SubTest(t)
}

func TestIsChromiumSessionLost(t *testing.T) {
	if !isChromiumSessionLost(chromedp.ErrChannelClosed) {
		t.Fatal("expected channel closed to count as session lost")
	}
	if isChromiumSessionLost(nil) {
		t.Fatal("nil error should not count as session lost")
	}
}

// TestHelperBlockUntilKilled is invoked as a subprocess by TestLoginSessionClose.
func TestHelperBlockUntilKilled(t *testing.T) {
	if os.Getenv("CUSE_LOGIN_TEST_HELPER") != "block" {
		t.Skip("helper subprocess only")
	}
	select {}
}

func TestLoginSessionClose(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperBlockUntilKilled$")
	cmd.Env = append(os.Environ(), "CUSE_LOGIN_TEST_HELPER=block")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	loginCtx, cancel := context.WithCancelCause(context.Background())
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		_ = cmd.Wait()
		cancel(ErrLoginBrowserClosed)
	}()

	var shutdown sync.Once
	session := &loginSession{
		ctx: loginCtx,
		closeFn: func() {
			shutdown.Do(func() {
				if cmd.Process != nil {
					_ = cmd.Process.Kill()
				}
				<-exited
			})
		},
		readCookie: func(context.Context) (string, error) { return "", nil },
	}

	session.Close()

	select {
	case <-loginCtx.Done():
		if !errors.Is(context.Cause(loginCtx), ErrLoginBrowserClosed) {
			t.Fatalf("cause: %v", context.Cause(loginCtx))
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for session to end")
	}
}
