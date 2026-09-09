package main

import (
	"errors"
	"fmt"
	"testing"

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

func TestLoginBrowserDiscovery(t *testing.T) {
	t.Logf("firefox: %q", findFirefoxBrowser())
	t.Logf("chromium: %q", findChromiumBrowser())
	browser, err := resolveLoginBrowser("")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("default login: engine=%s path=%s", browser.engine, browser.path)
}
