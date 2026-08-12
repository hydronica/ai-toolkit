package main

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	cookieDBBusyRetries = 3
	cookieDBBusyBackoff = 50 * time.Millisecond
)

// chromiumExecNames are common Chromium-based browser binary names on PATH.
var chromiumExecNames = []string{
	"google-chrome",
	"google-chrome-stable",
	"google-chrome-beta",
	"chromium",
	"chromium-browser",
	"brave-browser",
	"brave-browser-stable",
	"microsoft-edge",
	"microsoft-edge-stable",
	"msedge",
	"vivaldi",
	"vivaldi-stable",
	"opera",
}

func firstOnPath(names ...string) string {
	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

func firstExisting(paths ...string) string {
	for _, p := range paths {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// findFirefoxBrowser returns the Firefox executable path if available.
// Firefox creates a default profile on first run if none exists.
func findFirefoxBrowser() string {
	if path := firstOnPath("firefox"); path != "" {
		return path
	}
	return firstExisting(firefoxInstallPathsFn()...)
}

// firefoxInstallPathsFn is swappable in tests to isolate from the host system.
var firefoxInstallPathsFn = firefoxInstallPaths

func firefoxInstallPaths() []string {
	switch runtime.GOOS {
	case "windows":
		userProfile := os.Getenv("USERPROFILE")
		return []string{
			`C:\Program Files\Mozilla Firefox\firefox.exe`,
			`C:\Program Files (x86)\Mozilla Firefox\firefox.exe`,
			filepath.Join(userProfile, `AppData\Local\Mozilla Firefox\firefox.exe`),
		}
	case "darwin":
		return []string{
			"/Applications/Firefox.app/Contents/MacOS/firefox",
		}
	default:
		return []string{
			"/usr/bin/firefox",
			"/snap/bin/firefox",
			"/usr/lib/firefox/firefox",
		}
	}
}

// findChromiumBrowser returns the path to a Chromium-compatible browser executable.
func findChromiumBrowser() string {
	if path := firstOnPath(chromiumExecNames...); path != "" {
		return path
	}
	return firstExisting(chromiumInstallPaths()...)
}

func chromiumInstallPaths() []string {
	switch runtime.GOOS {
	case "windows":
		userProfile := os.Getenv("USERPROFILE")
		return []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			filepath.Join(userProfile, `AppData\Local\Google\Chrome\Application\chrome.exe`),
			`C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe`,
			`C:\Program Files (x86)\BraveSoftware\Brave-Browser\Application\brave.exe`,
			filepath.Join(userProfile, `AppData\Local\BraveSoftware\Brave-Browser\Application\brave.exe`),
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
			filepath.Join(userProfile, `AppData\Local\Microsoft\Edge\Application\msedge.exe`),
		}
	case "darwin":
		userHome := os.Getenv("HOME")
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			filepath.Join(userHome, "Applications/Google Chrome.app/Contents/MacOS/Google Chrome"),
			filepath.Join(userHome, "Applications/Brave Browser.app/Contents/MacOS/Brave Browser"),
		}
	default:
		return []string{
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/brave-browser",
			"/usr/bin/brave-browser-stable",
			"/usr/bin/microsoft-edge",
			"/usr/bin/microsoft-edge-stable",
			"/snap/bin/chromium",
			"/snap/bin/google-chrome",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
		}
	}
}

// openFirefox launches Firefox with the given URL using the specified executable.
func openFirefox(firefoxPath, url string) error {
	if runtime.GOOS == "darwin" && strings.HasSuffix(firefoxPath, ".app/Contents/MacOS/firefox") {
		appPath := strings.TrimSuffix(firefoxPath, "/Contents/MacOS/firefox")
		return exec.Command("open", "-a", appPath, url).Start()
	}
	return exec.Command(firefoxPath, url).Start()
}

func firefoxCookiesPath() (string, error) {
	base, err := firefoxBaseDir()
	if err != nil {
		return "", err
	}
	profileRel, err := firefoxDefaultProfile(filepath.Join(base, "profiles.ini"))
	if err != nil {
		return "", err
	}
	return filepath.Join(base, profileRel, "cookies.sqlite"), nil
}

func firefoxBaseDir() (string, error) {
	var candidates []string
	switch runtime.GOOS {
	case "windows":
		candidates = []string{filepath.Join(os.Getenv("APPDATA"), "Mozilla", "Firefox")}
	case "darwin":
		home := os.Getenv("HOME")
		if home == "" {
			return "", fmt.Errorf("HOME is not set")
		}
		candidates = []string{filepath.Join(home, "Library", "Application Support", "Firefox")}
	default:
		home := os.Getenv("HOME")
		if home == "" {
			return "", fmt.Errorf("HOME is not set")
		}
		candidates = []string{
			filepath.Join(home, ".mozilla", "firefox"),
			filepath.Join(home, "snap", "firefox", "common", ".mozilla", "firefox"),
		}
	}

	for _, base := range candidates {
		if base == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(base, "profiles.ini")); err == nil {
			return base, nil
		}
	}
	return "", fmt.Errorf("could not find Firefox profile directory")
}

type firefoxProfile struct {
	path      string
	isDefault bool
}

func firefoxDefaultProfile(iniPath string) (string, error) {
	data, err := os.ReadFile(iniPath)
	if err != nil {
		return "", fmt.Errorf("reading Firefox profiles.ini: %w", err)
	}

	var installDefault string
	var profiles []firefoxProfile
	var current firefoxProfile
	var inProfile bool

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			if inProfile && current.path != "" {
				profiles = append(profiles, current)
			}
			inProfile = strings.HasPrefix(line, "[Profile")
			current = firefoxProfile{}
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		switch {
		case !inProfile && key == "Default":
			installDefault = val
		case inProfile && key == "Path":
			current.path = val
		case inProfile && key == "Default":
			current.isDefault = val == "1"
		}
	}
	if inProfile && current.path != "" {
		profiles = append(profiles, current)
	}

	if installDefault != "" {
		return installDefault, nil
	}
	for _, p := range profiles {
		if p.isDefault {
			return p.path, nil
		}
	}
	if len(profiles) > 0 {
		return profiles[0].path, nil
	}
	return "", fmt.Errorf("no Firefox profile found in %s", iniPath)
}

func readFirefoxCookie(dbPath string) (string, error) {
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("reading Firefox cookie database: %w", err)
	}

	value, err := queryFirefoxCookieDB(dbPath, false)
	if err == nil || !isSQLiteBusy(err) {
		return value, err
	}

	for range cookieDBBusyRetries {
		time.Sleep(cookieDBBusyBackoff)
		value, err = queryFirefoxCookieDB(dbPath, false)
		if err == nil || !isSQLiteBusy(err) {
			return value, err
		}
	}

	tmpPath, cleanup, err := snapshotFirefoxCookieDB(dbPath)
	if err != nil {
		return "", err
	}
	defer cleanup()

	return queryFirefoxCookieDB(tmpPath, true)
}

func snapshotFirefoxCookieDB(dbPath string) (string, func(), error) {
	tmp, err := os.CreateTemp("", "cuse-cookies-*.sqlite")
	if err != nil {
		return "", nil, fmt.Errorf("copying Firefox cookie database: %w", err)
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()

	cleanup := func() {
		for _, path := range []string{tmpPath, tmpPath + "-wal", tmpPath + "-shm"} {
			_ = os.Remove(path)
		}
	}

	if err := copyFile(dbPath, tmpPath); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("copying Firefox cookie database: %w", err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := copyFileIfExists(dbPath+suffix, tmpPath+suffix); err != nil {
			cleanup()
			return "", nil, fmt.Errorf("copying Firefox cookie database sidecar: %w", err)
		}
	}
	return tmpPath, cleanup, nil
}

func copyFileIfExists(src, dst string) error {
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return copyFile(src, dst)
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o600)
}

func firefoxCookieDBDSN(dbPath string, immutable bool) string {
	dsn := fmt.Sprintf("file:%s?mode=ro", filepath.ToSlash(dbPath))
	if immutable {
		dsn += "&immutable=1"
	}
	return dsn
}

func queryFirefoxCookieDB(dbPath string, immutable bool) (string, error) {
	dsn := firefoxCookieDBDSN(dbPath, immutable)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return "", fmt.Errorf("opening Firefox cookie database: %w", err)
	}
	defer db.Close()

	var value string
	err = db.QueryRow(
		`SELECT value FROM moz_cookies
		 WHERE name = ?
		   AND (host = 'cursor.com' OR host = '.cursor.com' OR host LIKE '%.cursor.com')
		 LIMIT 1`,
		cookieName,
	).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("querying Firefox cookie database: %w", err)
	}
	return value, nil
}

func isSQLiteBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "locked") || strings.Contains(msg, "busy")
}
