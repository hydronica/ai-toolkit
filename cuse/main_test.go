package main

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"os"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestFormatTimeRemaining(t *testing.T) {
	tests := []struct {
		name      string
		remaining time.Duration
		want      string
	}{
		{"zero", 0, "0 min"},
		{"negative", -time.Hour, "0 min"},
		{"days only", 12 * 24 * time.Hour, "12 days"},
		{"one day over 48h", 50 * time.Hour, "2 days"},
		{"just over 48h", 49 * time.Hour, "2 days"},
		{"48h middle tier", 48 * time.Hour, "2 days"},
		{"days and hours", 30 * time.Hour, "1 day 6 hr"},
		{"hours only middle tier", 18 * time.Hour, "18 hr"},
		{"12h middle tier", 12 * time.Hour, "12 hr"},
		{"hours and minutes", 5*time.Hour + 15*time.Minute, "5 hr 15 min"},
		{"minutes only", 45 * time.Minute, "45 min"},
		{"one hour", time.Hour, "1 hr"},
		{"one minute", time.Minute, "1 min"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatTimeRemaining(tt.remaining); got != tt.want {
				t.Fatalf("formatTimeRemaining(%v) = %q, want %q", tt.remaining, got, tt.want)
			}
		})
	}
}

func TestFormatDollars(t *testing.T) {
	tests := []struct {
		cents float64
		want  string
	}{
		{0, "$0.00"},
		{99, "$0.99"},
		{200000, "$2,000.00"},
		{1234567, "$12,345.67"},
		{-200000, "-$2,000.00"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%.0f", tt.cents), func(t *testing.T) {
			if got := formatDollars(tt.cents); got != tt.want {
				t.Fatalf("formatDollars(%v) = %q, want %q", tt.cents, got, tt.want)
			}
		})
	}
}

func TestRequestUsed(t *testing.T) {
	tests := []struct {
		name string
		r    *UsageResult
		want float64
	}{
		{
			name: "fallback to plan used",
			r:    &UsageResult{RequestsUsed: 1124, RequestsLimit: 2000},
			want: 1124,
		},
		{
			name: "uses breakdown total",
			r: &UsageResult{
				RequestsUsed:           2000,
				RequestsLimit:          2000,
				RequestsBreakdownTotal: floatPtr(2386),
			},
			want: 2386,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := requestUsed(tt.r); got != tt.want {
				t.Fatalf("requestUsed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRequestPercent(t *testing.T) {
	tests := []struct {
		used, limit float64
		want        float64
	}{
		{1124, 2000, 56.2},
		{2500, 2000, 125.0},
		{0, 2000, 0},
		{100, 0, 0},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%.0f/%.0f", tt.used, tt.limit), func(t *testing.T) {
			got := requestPercent(tt.used, tt.limit)
			if math.Abs(got-tt.want) > 0.05 {
				t.Fatalf("requestPercent(%v, %v) = %v, want %v", tt.used, tt.limit, got, tt.want)
			}
		})
	}
}

func TestParseUsageAndOutput(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		want       *UsageResult
		wantOutput string
	}{
		{
			name: "on_demand_enabled",
			body: `{
				"membershipType": "enterprise",
				"individualUsage": {
					"plan": {
						"used": 1124,
						"limit": 2000,
						"autoPercentUsed": 0,
						"apiPercentUsed": 100,
						"totalPercentUsed": 56.2
					},
					"onDemand": {
						"enabled": true,
						"used": 200000
					}
				}
			}`,
			want: &UsageResult{
				MembershipType:  "enterprise",
				TotalPercent:    56.2,
				AutoPercent:     0,
				APIPercent:      100,
				RequestsUsed:    1124,
				RequestsLimit:   2000,
				OnDemandEnabled: true,
				OnDemandUsed:    200000,
			},
			wantOutput: `Cursor usage
-------------
Billing period:  — → —
Plan:            enterprise
Total usage:
  API (named):   100.0%
  Auto:          0.0%
  Requests:      56.2% (1124/2000)

On-demand:       $2,000.00 spent

`,
		},
		{
			name: "on_demand_disabled",
			body: `{
				"membershipType": "pro",
				"individualUsage": {
					"plan": {
						"used": 350,
						"limit": 0,
						"autoPercentUsed": 42.5,
						"apiPercentUsed": 18.3,
						"totalPercentUsed": 30.4
					},
					"onDemand": {
						"enabled": false,
						"used": 0
					}
				}
			}`,
			want: &UsageResult{
				MembershipType:  "pro",
				TotalPercent:    30.4,
				AutoPercent:     42.5,
				APIPercent:      18.3,
				RequestsUsed:    350,
				OnDemandEnabled: false,
			},
			wantOutput: `Cursor usage
-------------
Billing period:  — → —
Plan:            pro
Total usage:
  API (named):   18.3%
  Auto:          42.5%

On-demand:       Disabled

`,
		},
		{
			name: "team_usage_nested_on_demand",
			body: `{
				"membershipType": "enterprise",
				"individualUsage": {
					"plan": {
						"used": 800,
						"limit": 2000,
						"autoPercentUsed": 10,
						"apiPercentUsed": 25,
						"totalPercentUsed": 40
					},
					"onDemand": {
						"enabled": true,
						"used": 200000
					}
				},
				"teamUsage": {
					"onDemand": {
						"enabled": true,
						"used": 50000,
						"limit": 100000,
						"remaining": 50000
					}
				}
			}`,
			want: &UsageResult{
				MembershipType:  "enterprise",
				TotalPercent:    40,
				AutoPercent:     10,
				APIPercent:      25,
				RequestsUsed:    800,
				RequestsLimit:   2000,
				OnDemandEnabled: true,
				OnDemandUsed:    200000,
				Team: &TeamUsageInfo{
					Enabled:   true,
					Used:      50000,
					Limit:     floatPtr(100000),
					Remaining: floatPtr(50000),
				},
			},
			wantOutput: `Cursor usage
-------------
Billing period:  — → —
Plan:            enterprise
Total usage:
  API (named):   25.0%
  Auto:          10.0%
  Requests:      40.0% (800/2000)

On-demand:       $2,000.00 spent

Team usage:      $500.00 spent
  Limit:         $1,000.00
  Remaining:     $500.00

`,
		},
		{
			name: "breakdown_total_for_requests",
			body: `{
				"membershipType": "pro",
				"individualUsage": {
					"plan": {
						"used": 2000,
						"limit": 2000,
						"breakdown": {
							"included": 2000,
							"bonus": 386,
							"total": 2386
						},
						"autoPercentUsed": 0,
						"apiPercentUsed": 53.02,
						"totalPercentUsed": 12.24
					},
					"onDemand": {
						"enabled": false,
						"used": 0
					}
				}
			}`,
			want: &UsageResult{
				MembershipType:         "pro",
				TotalPercent:           12.24,
				AutoPercent:            0,
				APIPercent:             53.02,
				RequestsUsed:           2000,
				RequestsLimit:          2000,
				RequestsBreakdownTotal: floatPtr(2386),
				OnDemandEnabled:        false,
			},
			wantOutput: `Cursor usage
-------------
Billing period:  — → —
Plan:            pro
Total usage:
  API (named):   53.0%
  Auto:          0.0%
  Requests:      119.3% (2386/2000)

On-demand:       Disabled

`,
		},
		{
			body: `{
				"membershipType": "free",
				"individualUsage": false
			}`,
			want: &UsageResult{
				MembershipType: "free",
			},
			wantOutput: `Cursor usage
-------------
Billing period:  — → —
Plan:            free
Total usage:
  API (named):   0.0%
  Auto:          0.0%

On-demand:       Disabled

`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseUsage([]byte(tt.body))
			if err != nil {
				t.Fatalf("parseUsage: %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("parseUsage mismatch (-want +got):\n%s", diff)
			}

			output := capturePrintTable(got)
			if output != tt.wantOutput {
				t.Fatalf("printTable output mismatch\n--- got ---\n%s--- want ---\n%s", output, tt.wantOutput)
			}
		})
	}
}

func floatPtr(v float64) *float64 {
	return &v
}

func capturePrintTable(r *UsageResult) string {
	oldStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	os.Stdout = writePipe

	printTable(r)

	writePipe.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, readPipe)
	return buf.String()
}
