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
	"time"

	_ "modernc.org/sqlite"
)

const (
	cookieDBBusyRetries = 3
	cookieDBBusyBackoff = 50 * time.Millisecond
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

	if findFirefoxBrowser() != "" {
		return loginStrategyFirefox, "", nil
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
	exe := desktopExecArgv0(execLine)
	if exe == "" {
		return ""
	}
	if unquoted, err := strconv.Unquote(exe); err == nil {
		exe = unquoted
	}
	if strings.ContainsRune(exe, '%') {
		return ""
	}
	return exe
}

// desktopExecArgv0 returns the first argument from a .desktop Exec= value,
// respecting quotes and escape sequences from the desktop entry spec.
func desktopExecArgv0(execLine string) string {
	execLine = strings.TrimSpace(execLine)
	if execLine == "" {
		return ""
	}

	var argv0 strings.Builder
	i := 0
	for i < len(execLine) {
		switch execLine[i] {
		case ' ', '\t':
			return argv0.String()
		case '"', '\'':
			quote := execLine[i]
			i++
			for i < len(execLine) {
				if execLine[i] == '\\' && i+1 < len(execLine) {
					i++
					argv0.WriteByte(unescapeDesktopExec(execLine[i]))
					i++
					continue
				}
				if execLine[i] == quote {
					i++
					break
				}
				argv0.WriteByte(execLine[i])
				i++
			}
		case '\\':
			if i+1 >= len(execLine) {
				return argv0.String()
			}
			i++
			argv0.WriteByte(unescapeDesktopExec(execLine[i]))
			i++
		default:
			argv0.WriteByte(execLine[i])
			i++
		}
	}
	return argv0.String()
}

func unescapeDesktopExec(c byte) byte {
	switch c {
	case 's':
		return ' '
	case 'n':
		return '\n'
	case 't':
		return '\t'
	case 'r':
		return '\r'
	case '\\':
		return '\\'
	default:
		return c
	}
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

// findFirefoxBrowser reports whether Firefox can be used for login, either via
// a resolvable profile or a firefox executable on PATH.
func findFirefoxBrowser() string {
	if _, err := firefoxCookiesPath(); err == nil {
		return "profile"
	}
	path, err := exec.LookPath("firefox")
	if err != nil {
		return ""
	}
	return path
}

// findChromiumBrowser returns the path to a Chromium-compatible browser executable.
func findChromiumBrowser() string {
	for _, name := range chromiumExecNames {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}

	var candidates []string
	switch runtime.GOOS {
	case "windows":
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
	case "darwin":
		userHome := os.Getenv("HOME")
		candidates = []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			filepath.Join(userHome, "Applications/Google Chrome.app/Contents/MacOS/Google Chrome"),
			filepath.Join(userHome, "Applications/Brave Browser.app/Contents/MacOS/Brave Browser"),
		}
	default:
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
