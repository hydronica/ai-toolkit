package main

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

type loginStrategy int

const (
	loginStrategyChromium loginStrategy = iota
	loginStrategyFirefox
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

// chooseLoginStrategy picks how to authenticate based on the system default
// browser when possible, otherwise any installed Chromium browser, then Firefox.
func chooseLoginStrategy() (loginStrategy, string, error) {
	if name, execPath := defaultBrowser(); execPath != "" {
		switch {
		case isChromiumBrowser(name, execPath):
			return loginStrategyChromium, execPath, nil
		case isFirefoxBrowser(name, execPath):
			return loginStrategyFirefox, execPath, nil
		}
	}

	if path := findChromiumBrowser(); path != "" {
		return loginStrategyChromium, path, nil
	}

	if path, err := firefoxCookiesPath(); err == nil {
		if _, err := os.Stat(path); err == nil {
			return loginStrategyFirefox, "", nil
		}
	}

	var noStrategy loginStrategy
	return noStrategy, "", fmt.Errorf("no supported browser found (need Chrome, Chromium, Brave, Edge, or Firefox)")
}

func defaultBrowser() (desktopName, execPath string) {
	switch runtime.GOOS {
	case "linux":
		return linuxDefaultBrowser()
	default:
		return "", ""
	}
}

func linuxDefaultBrowser() (string, string) {
	out, err := exec.Command("xdg-settings", "get", "default-web-browser").Output()
	if err != nil {
		return "", ""
	}
	desktop := strings.TrimSpace(string(out))
	if desktop == "" {
		return "", ""
	}
	execPath := desktopExecPath(findDesktopFile(desktop))
	if execPath == "" {
		execPath = guessExecFromDesktopName(desktop)
	}
	return desktop, execPath
}

func guessExecFromDesktopName(desktop string) string {
	base := strings.ToLower(strings.TrimSuffix(desktop, ".desktop"))
	for _, name := range append(chromiumExecNames, "firefox") {
		if !strings.Contains(base, strings.TrimSuffix(name, "-stable")) {
			continue
		}
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	return ""
}

func findDesktopFile(name string) string {
	searchDirs := xdgDataDirs()
	for _, dir := range searchDirs {
		candidate := filepath.Join(dir, "applications", name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func xdgDataDirs() []string {
	var dirs []string
	if home := os.Getenv("HOME"); home != "" {
		dirs = append(dirs, filepath.Join(home, ".local", "share"))
	}
	if dataHome := os.Getenv("XDG_DATA_HOME"); dataHome != "" {
		dirs = append([]string{dataHome}, dirs...)
	}
	if dataDirs := os.Getenv("XDG_DATA_DIRS"); dataDirs != "" {
		for _, dir := range strings.Split(dataDirs, ":") {
			if dir != "" {
				dirs = append(dirs, dir)
			}
		}
	} else {
		dirs = append(dirs, "/usr/local/share", "/usr/share")
	}
	if runtime.GOOS == "linux" {
		dirs = append(dirs, "/var/lib/snapd/desktop")
	}
	return dirs
}

func desktopExecPath(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Exec=") {
			continue
		}
		return parseDesktopExec(strings.TrimPrefix(line, "Exec="))
	}
	return ""
}

// parseDesktopExec returns the executable from a .desktop Exec= value.
func parseDesktopExec(execLine string) string {
	execLine = strings.TrimSpace(execLine)
	if execLine == "" {
		return ""
	}
	fields := strings.Fields(execLine)
	if len(fields) == 0 {
		return ""
	}
	exe := fields[0]
	if unquoted, err := strconv.Unquote(exe); err == nil {
		exe = unquoted
	}
	if strings.ContainsRune(exe, '%') {
		return ""
	}
	return exe
}

func isChromiumBrowser(desktopName, execPath string) bool {
	joined := strings.ToLower(desktopName + " " + execPath + " " + filepath.Base(execPath))
	for _, marker := range []string{
		"chrome", "chromium", "brave", "edge", "msedge", "vivaldi", "opera",
	} {
		if strings.Contains(joined, marker) {
			return true
		}
	}
	return false
}

func isFirefoxBrowser(desktopName, execPath string) bool {
	joined := strings.ToLower(desktopName + " " + execPath + " " + filepath.Base(execPath))
	return strings.Contains(joined, "firefox")
}

// findChromiumBrowser returns the path to a Chromium-compatible browser executable.
func findChromiumBrowser() string {
	for _, name := range chromiumExecNames {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}

	var candidates []string
	if runtime.GOOS == "windows" {
		userProfile := os.Getenv("USERPROFILE")
		candidates = []string{
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
	} else if runtime.GOOS == "darwin" {
		userHome := os.Getenv("HOME")
		candidates = []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			filepath.Join(userHome, "Applications/Google Chrome.app/Contents/MacOS/Google Chrome"),
			filepath.Join(userHome, "Applications/Brave Browser.app/Contents/MacOS/Brave Browser"),
		}
	} else {
		candidates = []string{
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
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func openURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
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
	value, err := queryFirefoxCookieDB(dbPath)
	if err == nil || !isSQLiteBusy(err) {
		return value, err
	}

	tmp, err := os.CreateTemp("", "cuse-cookies-*.sqlite")
	if err != nil {
		return "", fmt.Errorf("copying Firefox cookie database: %w", err)
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	src, err := os.ReadFile(dbPath)
	if err != nil {
		return "", fmt.Errorf("reading Firefox cookie database: %w", err)
	}
	if err := os.WriteFile(tmpPath, src, 0o600); err != nil {
		return "", fmt.Errorf("copying Firefox cookie database: %w", err)
	}
	return queryFirefoxCookieDB(tmpPath)
}

func queryFirefoxCookieDB(dbPath string) (string, error) {
	dsn := fmt.Sprintf("file:%s?mode=ro", filepath.ToSlash(dbPath))
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
