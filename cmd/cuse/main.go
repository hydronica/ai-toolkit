package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "cuse: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	showVersion := flag.Bool("version", false, "print version and exit")
	debug := flag.Bool("debug", false, "print raw API responses")
	browser := flag.String("browser", "", "browser for login: firefox or chromium (default: firefox on Linux, chromium elsewhere)")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return nil
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	envPath := resolveEnvPath()

	// Load saved cookie from .env or environment.
	if err := loadEnvFile(envPath); err != nil {
		return err
	}

	cookie := os.Getenv("CURSOR_COOKIE")

	// First attempt: try with whatever cookie we have (may be empty).
	if cookie != "" {
		result, err := fetchUsage(ctx, cookie, *debug)
		if err == nil {
			fmt.Print(result.Format(time.Now()))
			return nil
		}
		if !errors.Is(err, ErrAuthFailed) {
			return err
		}
		fmt.Fprintln(os.Stderr, "Cookie expired or invalid — starting browser login...")
	} else {
		fmt.Fprintln(os.Stderr, "No cookie found — starting browser login...")
	}

	// Fall through to browser login.
	rawCookie, err := runLogin(ctx, *browser)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}

	// Persist the new cookie.
	if err := writeEnvKey(envPath, "CURSOR_COOKIE", rawCookie); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not save cookie to %s: %v\n", envPath, err)
	} else {
		fmt.Printf("Cookie saved to %s\n", envPath)
	}

	// Retry usage fetch with the new cookie.
	result, err := fetchUsage(ctx, rawCookie, *debug)
	if err != nil {
		return fmt.Errorf("fetching usage after login: %w", err)
	}
	fmt.Print(result.Format(time.Now()))
	return nil
}

// resolveEnvPath returns the path to .env in the same directory as the executable.
// The file may not exist yet; writeEnvKey will create it on first login.
func resolveEnvPath() string {
	exe, err := os.Executable()
	if err != nil {
		return ".env"
	}
	return filepath.Join(filepath.Dir(exe), ".env")
}

const periodDisplayLayout = "2006-01-02T15:04 MST"

// Format returns the billing usage report as a string.
func (r *UsageResult) Format(now time.Time) string {
	var b strings.Builder
	remaining, elapsedPct, hasPeriod := r.periodStats(now)
	r.writeHeader(&b, hasPeriod, remaining, elapsedPct)
	r.writeUsageSection(&b, hasPeriod)
	b.WriteString("\n")
	r.writeSpendSection(&b)
	b.WriteString("\n")
	return b.String()
}

func (r *UsageResult) writeHeader(b *strings.Builder, hasPeriod bool, remaining time.Duration, elapsedPct float64) {
	fmt.Fprintln(b, "Cursor usage")
	fmt.Fprintln(b, "-------------")
	fmt.Fprintf(b, "Billing period:  %s → %s\n", formatPeriod(r.PeriodStart), formatPeriod(r.PeriodEnd))
	if hasPeriod {
		fmt.Fprintf(b, "Time remaining:  %s (%.1f%% elapsed)\n", formatTimeRemaining(remaining), elapsedPct)
	}
	if r.MembershipType != "" {
		planLine := r.MembershipType
		if r.LimitType == "team" {
			planLine += " (team limit)"
		}
		fmt.Fprintf(b, "Plan:            %s\n", planLine)
	}
}

func (r *UsageResult) writeUsageSection(b *strings.Builder, hasPeriod bool) {
	teamLimit := r.LimitType == "team" && r.RequestsLimit > 0
	showPlanLines := r.LimitType != "team"
	showRequestLines := r.RequestsLimit > 0 || r.RequestsRemaining != nil
	if !showPlanLines && !showRequestLines {
		return
	}

	line := "Total usage:"
	if teamLimit {
		line = "Included usage:"
	}
	if hasPeriod {
		if teamLimit {
			line += "  (Tip: keep usage below elapsed %)"
		} else {
			line += "     (Tip: keep usage below elapsed %)"
		}
	}
	fmt.Fprintln(b, line)

	if showPlanLines {
		fmt.Fprintf(b, "  API (named):   %.1f%%\n", r.APIPercent)
		fmt.Fprintf(b, "  Auto:          %.1f%%\n", r.AutoPercent)
	}
	if r.RequestsLimit > 0 {
		used := r.requestUsed()
		pct := requestPercent(used, r.RequestsLimit)
		if teamLimit {
			fmt.Fprintf(b, "  Usage:         %.1f%% (%.0f/%.0f)\n", pct, used, r.RequestsLimit)
		} else {
			fmt.Fprintf(b, "  Requests:      %.1f%% (%.0f/%.0f)\n", pct, used, r.RequestsLimit)
		}
	}
	if r.RequestsRemaining != nil && !teamLimit {
		fmt.Fprintf(b, "  Remaining:     %.0f\n", *r.RequestsRemaining)
	}
}

func (r *UsageResult) writeSpendSection(b *strings.Builder) {
	if r.LimitType != "team" {
		r.writeIndividualOnDemand(b)
	}
	if r.Team != nil && r.Team.Enabled {
		if r.LimitType != "team" {
			b.WriteString("\n")
		}
		heading := "Team on-demand:"
		if r.LimitType != "team" {
			heading = "Team usage:"
		}
		r.Team.writeSpend(b, heading)
	}
}

func (r *UsageResult) writeIndividualOnDemand(b *strings.Builder) {
	if !r.OnDemandEnabled {
		fmt.Fprintln(b, "On-demand:       Disabled")
		return
	}
	fmt.Fprintf(b, "On-demand:       %s spent\n", formatDollars(r.OnDemandUsed))
}

func (t *TeamUsageInfo) writeSpend(b *strings.Builder, heading string) {
	if !t.Enabled {
		return
	}
	fmt.Fprintf(b, "%-17s%s spent\n", heading, formatDollars(t.Used))
	if t.Limit != nil {
		fmt.Fprintf(b, "  Limit:         %s\n", formatDollars(*t.Limit))
	}
	if t.Remaining != nil {
		fmt.Fprintf(b, "  Remaining:     %s\n", formatDollars(*t.Remaining))
	}
}

func formatPeriod(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Local().Format(periodDisplayLayout)
}

// periodStats returns time until the billing period ends and the elapsed percentage.
func (r *UsageResult) periodStats(now time.Time) (remaining time.Duration, elapsedPct float64, ok bool) {
	if r.PeriodStart.IsZero() || r.PeriodEnd.IsZero() {
		return 0, 0, false
	}
	total := r.PeriodEnd.Sub(r.PeriodStart)
	if total <= 0 {
		return 0, 0, false
	}
	remaining = r.PeriodEnd.Sub(now)
	elapsed := now.Sub(r.PeriodStart)
	elapsedPct = math.Min(100, math.Max(0, (elapsed.Seconds()/total.Seconds())*100))
	return remaining, elapsedPct, true
}

// formatTimeRemaining formats the duration until the billing period ends.
func formatTimeRemaining(remaining time.Duration) string {
	if remaining <= 0 {
		return "0 min"
	}

	hours := remaining.Hours()
	if hours > 48 {
		days := int(math.Floor(hours / 24))
		if days == 1 {
			return "1 day"
		}
		return fmt.Sprintf("%d days", days)
	}

	if hours >= 12 {
		totalHours := int(math.Floor(hours))
		days := totalHours / 24
		remHours := totalHours % 24
		if days == 0 {
			if remHours == 1 {
				return "1 hr"
			}
			return fmt.Sprintf("%d hr", remHours)
		}
		dayWord := "days"
		if days == 1 {
			dayWord = "day"
		}
		if remHours == 0 {
			return fmt.Sprintf("%d %s", days, dayWord)
		}
		return fmt.Sprintf("%d %s %d hr", days, dayWord, remHours)
	}

	totalMinutes := int(math.Floor(remaining.Minutes()))
	hrs := totalMinutes / 60
	mins := totalMinutes % 60
	if hrs == 0 {
		if mins == 1 {
			return "1 min"
		}
		return fmt.Sprintf("%d min", mins)
	}
	if mins == 0 {
		if hrs == 1 {
			return "1 hr"
		}
		return fmt.Sprintf("%d hr", hrs)
	}
	if hrs == 1 {
		return fmt.Sprintf("1 hr %d min", mins)
	}
	return fmt.Sprintf("%d hr %d min", hrs, mins)
}

// requestUsed returns breakdown.total when present, otherwise plan.used.
func (r *UsageResult) requestUsed() float64 {
	if r.RequestsBreakdownTotal != nil {
		return *r.RequestsBreakdownTotal
	}
	return r.RequestsUsed
}

// requestPercent returns used/limit as a percentage without capping at 100.
func requestPercent(used, limit float64) float64 {
	if limit <= 0 {
		return 0
	}
	return (used / limit) * 100
}

// formatDollars formats a cent amount as a dollar string with thousands separators.
func formatDollars(cents float64) string {
	negative := cents < 0
	if negative {
		cents = -cents
	}
	rounded := int64(math.Round(cents))
	dollars := rounded / 100
	remainder := rounded % 100
	s := fmt.Sprintf("$%s.%02d", formatWithCommas(dollars), remainder)
	if negative {
		return "-" + s
	}
	return s
}

// formatWithCommas formats an integer with thousands separators.
func formatWithCommas(n int64) string {
	if n < 0 {
		return "-" + formatWithCommas(-n)
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	if s != "" {
		parts = append([]string{s}, parts...)
	}
	return strings.Join(parts, ",")
}
