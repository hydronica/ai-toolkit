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
			printTable(result)
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
	rawCookie, err := runLogin(ctx)
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
	printTable(result)
	return nil
}

// resolveEnvPath returns the path to bin/.env.
// It tries, in order:
//  1. ./.env relative to the current working directory.
//  2. .env relative to the directory containing the executable.
func resolveEnvPath() string {

	if _, err := os.Stat(".env"); err == nil {
		return ".env"
	}
	if exe, err := os.Executable(); err == nil {
		dir, _ := filepath.Split(exe)
		alt := filepath.Join(dir, ".env")
		if _, err := os.Stat(alt); err == nil {
			return alt
		}
	}
	// Return the cwd-relative path as default even if it doesn't exist yet;
	// writeEnvKey will create it.
	return ".env"
}

const periodDisplayLayout = "2006-01-02T15:04 MST"

// printTable prints the billing usage report to stdout.
func printTable(r *UsageResult) {
	fmt.Println("Cursor usage")
	fmt.Println("-------------")
	fmt.Printf("Billing period:  %s → %s\n", formatPeriod(r.PeriodStart), formatPeriod(r.PeriodEnd))
	if r.MembershipType != "" {
		fmt.Printf("Plan:            %s\n", r.MembershipType)
	}

	line := "Total usage:"
	if !r.PeriodStart.IsZero() && !r.PeriodEnd.IsZero() {
		periodPct, ok := periodElapsed(r.PeriodStart, r.PeriodEnd)
		if ok {
			line += fmt.Sprintf("     (period elapsed: %.1f%% — stay under this)", periodPct)
		}
	}
	fmt.Println(line)
	fmt.Printf("  API (named):   %.1f%%\n", r.APIPercent)
	fmt.Printf("  Auto:          %.1f%%\n", r.AutoPercent)

	fmt.Println()
	if !r.OnDemandEnabled {
		fmt.Println("On-demand:       Disabled")
	} else if r.OnDemandUsed > 0 {
		fmt.Printf("On-demand:       $%.2f spent\n", r.OnDemandUsed/100)
	} else {
		fmt.Println("On-demand:       $0.00 spent")
	}

	if r.Team != nil {
		fmt.Println()
		printTeamUsage(r.Team)
	}
	fmt.Println()
}

func printTeamUsage(t *TeamUsageInfo) {
	if !t.Enabled {
		return
	}
	if t.Used > 0 {
		fmt.Printf("Team usage:      $%.2f spent\n", t.Used/100)
	} else {
		fmt.Println("Team usage:      $0.00 spent")
	}
	if t.Limit != nil {
		fmt.Printf("  Limit:         $%.2f\n", *t.Limit/100)
	}
	if t.Remaining != nil {
		fmt.Printf("  Remaining:     $%.2f\n", *t.Remaining/100)
	}
}

func formatPeriod(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Local().Format(periodDisplayLayout)
}

// periodElapsed returns what percentage of the billing period has elapsed.
func periodElapsed(start, end time.Time) (float64, bool) {
	total := end.Sub(start).Seconds()
	if total <= 0 {
		return 0, false
	}
	elapsed := time.Since(start).Seconds()
	pct := math.Min(100, math.Max(0, (elapsed/total)*100))
	return pct, true
}
