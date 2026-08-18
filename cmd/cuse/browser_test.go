package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/hydronica/trial"
	_ "modernc.org/sqlite"
)

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

func TestFirefoxCookiesPath(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("test manipulates HOME which only affects Linux Firefox paths")
	}
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

func TestFindFirefoxBrowser(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "firefox")
	if runtime.GOOS == "windows" {
		fake += ".exe"
	}
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	got := findFirefoxBrowser()
	if got != fake {
		t.Fatalf("findFirefoxBrowser() = %q, want %q", got, fake)
	}
}

func TestReadFirefoxCookie(t *testing.T) {
	fn := func(layout string) (string, error) {
		dir, err := os.MkdirTemp("", "cuse-cookies-*")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(dir)

		dbPath := filepath.Join(dir, "cookies.sqlite")
		switch layout {
		case "missing":
			return readFirefoxCookie(dbPath)
		case "present":
			db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath))
			if err != nil {
				return "", err
			}
			if _, err := db.Exec(`CREATE TABLE moz_cookies (
				id INTEGER PRIMARY KEY,
				name TEXT,
				value TEXT,
				host TEXT,
				path TEXT
			)`); err != nil {
				db.Close()
				return "", err
			}
			if _, err := db.Exec(
				`INSERT INTO moz_cookies (name, value, host, path) VALUES (?, ?, ?, ?)`,
				cookieName, "session-token-value", ".cursor.com", "/",
			); err != nil {
				db.Close()
				return "", err
			}
			if err := db.Close(); err != nil {
				return "", err
			}
			return readFirefoxCookie(dbPath)
		default:
			return "", fmt.Errorf("unknown layout %q", layout)
		}
	}
	cases := trial.Cases[string, string]{
		"missing database waits for login": {
			Input:    "missing",
			Expected: "",
		},
		"reads session token": {
			Input:    "present",
			Expected: "session-token-value",
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
		return queryFirefoxCookieDB(dbPath, false)
	}
	cases := trial.Cases[string, string]{
		"reads session token": {
			Input:    "session-token-value",
			Expected: "session-token-value",
		},
	}
	trial.New(fn, cases).SubTest(t)
}

func TestSnapshotFirefoxCookieDB(t *testing.T) {
	fn := func(withSidecars bool) (bool, error) {
		dir, err := os.MkdirTemp("", "cuse-cookies-*")
		if err != nil {
			return false, err
		}
		defer os.RemoveAll(dir)

		dbPath := filepath.Join(dir, "cookies.sqlite")
		db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath))
		if err != nil {
			return false, err
		}
		if _, err := db.Exec(`CREATE TABLE moz_cookies (
			id INTEGER PRIMARY KEY,
			name TEXT,
			value TEXT,
			host TEXT,
			path TEXT
		)`); err != nil {
			db.Close()
			return false, err
		}
		if _, err := db.Exec(
			`INSERT INTO moz_cookies (name, value, host, path) VALUES (?, ?, ?, ?)`,
			cookieName, "session-token-value", ".cursor.com", "/",
		); err != nil {
			db.Close()
			return false, err
		}
		if err := db.Close(); err != nil {
			return false, err
		}

		if withSidecars {
			if err := os.WriteFile(dbPath+"-wal", []byte("wal"), 0o600); err != nil {
				return false, err
			}
			if err := os.WriteFile(dbPath+"-shm", []byte("shm"), 0o600); err != nil {
				return false, err
			}
		}

		tmpPath, cleanup, err := snapshotFirefoxCookieDB(dbPath)
		if err != nil {
			return false, err
		}
		defer cleanup()

		for _, suffix := range []string{"", "-wal", "-shm"} {
			wantExists := suffix == "" || withSidecars
			_, err := os.Stat(tmpPath + suffix)
			switch {
			case wantExists && err != nil:
				return false, fmt.Errorf("expected snapshot file %q", tmpPath+suffix)
			case !wantExists && err == nil:
				return false, fmt.Errorf("unexpected snapshot file %q", tmpPath+suffix)
			case !wantExists && !os.IsNotExist(err):
				return false, err
			}
		}

		got, err := queryFirefoxCookieDB(tmpPath, true)
		if err != nil {
			return false, err
		}
		return got == "session-token-value", nil
	}
	cases := trial.Cases[bool, bool]{
		"copies main database only": {
			Input:    false,
			Expected: true,
		},
		"copies wal and shm sidecars": {
			Input:    true,
			Expected: true,
		},
	}
	trial.New(fn, cases).SubTest(t)
}
