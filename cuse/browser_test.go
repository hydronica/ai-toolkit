package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hydronica/trial"
	_ "modernc.org/sqlite"
)

func TestParseDesktopExec(t *testing.T) {
	fn := func(in string) (string, error) {
		return parseDesktopExec(in), nil
	}
	cases := trial.Cases[string, string]{
		"firefox snap":          {Input: "/snap/bin/firefox %u", Expected: "/snap/bin/firefox"},
		"chromium with flags":   {Input: "/usr/bin/chromium --enable-features=foo %U", Expected: "/usr/bin/chromium"},
		"quoted path":           {Input: `"/opt/google/chrome" %U`, Expected: "/opt/google/chrome"},
		"field code only":       {Input: "firefox %u", Expected: "firefox"},
		"empty":                 {Input: "", Expected: ""},
	}
	trial.New(fn, cases).SubTest(t)
}

func TestFirefoxDefaultProfile(t *testing.T) {
	fn := func(ini string) (string, error) {
		dir, err := os.MkdirTemp("", "cuse-profile-*")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(dir)

		iniPath := filepath.Join(dir, "profiles.ini")
		if err := os.WriteFile(iniPath, []byte(ini), 0o600); err != nil {
			return "", err
		}
		return firefoxDefaultProfile(iniPath)
	}
	cases := trial.Cases[string, string]{
		"prefers install default": {
			Input: `[Install4F96D1932A9F858E]
Default=profiles/abc.default-release
Locked=1

[Profile0]
Name=default
IsRelative=1
Path=profiles/xyz.default
Default=1
`,
			Expected: "profiles/abc.default-release",
		},
		"falls back to profile default": {
			Input: `[Profile0]
Name=default
IsRelative=1
Path=profiles/xyz.default
Default=1
`,
			Expected: "profiles/xyz.default",
		},
	}
	trial.New(fn, cases).SubTest(t)
}

type browserDetectInput struct {
	desktopName string
	execPath    string
}

func TestIsFirefoxBrowser(t *testing.T) {
	fn := func(in browserDetectInput) (bool, error) {
		return isFirefoxBrowser(in.desktopName, in.execPath), nil
	}
	cases := trial.Cases[browserDetectInput, bool]{
		"firefox snap desktop": {
			Input:    browserDetectInput{"firefox_firefox.desktop", "/snap/bin/firefox"},
			Expected: true,
		},
		"chrome desktop": {
			Input:    browserDetectInput{"google-chrome.desktop", "/usr/bin/google-chrome"},
			Expected: false,
		},
	}
	trial.New(fn, cases).SubTest(t)
}

func TestIsChromiumBrowser(t *testing.T) {
	fn := func(in browserDetectInput) (bool, error) {
		return isChromiumBrowser(in.desktopName, in.execPath), nil
	}
	cases := trial.Cases[browserDetectInput, bool]{
		"chromium desktop": {
			Input:    browserDetectInput{"chromium_chromium.desktop", "/usr/bin/chromium"},
			Expected: true,
		},
		"firefox desktop": {
			Input:    browserDetectInput{"firefox_firefox.desktop", "/snap/bin/firefox"},
			Expected: false,
		},
	}
	trial.New(fn, cases).SubTest(t)
}

func TestFirefoxCookiesPath(t *testing.T) {
	fn := func(layout string) (string, error) {
		home, err := os.MkdirTemp("", "cuse-home-*")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(home)

		switch layout {
		case "snap":
			base := filepath.Join(home, "snap", "firefox", "common", ".mozilla", "firefox")
			profileDir := filepath.Join(base, "profiles", "abc.default")
			if err := os.MkdirAll(profileDir, 0o755); err != nil {
				return "", err
			}
			ini := `[Profile0]
Name=default
IsRelative=1
Path=profiles/abc.default
Default=1
`
			if err := os.WriteFile(filepath.Join(base, "profiles.ini"), []byte(ini), 0o600); err != nil {
				return "", err
			}
		default:
			return "", fmt.Errorf("unknown layout %q", layout)
		}

		oldHome := os.Getenv("HOME")
		os.Setenv("HOME", home)
		defer os.Setenv("HOME", oldHome)

		got, err := firefoxCookiesPath()
		if err != nil {
			return "", err
		}
		rel, err := filepath.Rel(home, got)
		if err != nil {
			return "", err
		}
		return rel, nil
	}
	cases := trial.Cases[string, string]{
		"firefox snap profile layout": {
			Input: "snap",
			Expected: filepath.Join(
				"snap", "firefox", "common", ".mozilla", "firefox",
				"profiles", "abc.default", "cookies.sqlite",
			),
		},
	}
	trial.New(fn, cases).SubTest(t)
}

func TestQueryFirefoxCookieDB(t *testing.T) {
	fn := func(tokenValue string) (string, error) {
		dir, err := os.MkdirTemp("", "cuse-cookies-*")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(dir)

		dbPath := filepath.Join(dir, "cookies.sqlite")
		db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath))
		if err != nil {
			return "", err
		}
		defer db.Close()

		if _, err := db.Exec(`CREATE TABLE moz_cookies (
			id INTEGER PRIMARY KEY,
			name TEXT,
			value TEXT,
			host TEXT,
			path TEXT
		)`); err != nil {
			return "", err
		}
		if _, err := db.Exec(
			`INSERT INTO moz_cookies (name, value, host, path) VALUES (?, ?, ?, ?)`,
			cookieName, tokenValue, ".cursor.com", "/",
		); err != nil {
			return "", err
		}
		if err := db.Close(); err != nil {
			return "", err
		}
		return queryFirefoxCookieDB(dbPath)
	}
	cases := trial.Cases[string, string]{
		"reads session token": {
			Input:    "session-token-value",
			Expected: "session-token-value",
		},
	}
	trial.New(fn, cases).SubTest(t)
}
