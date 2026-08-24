package main

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/hydronica/trial"
)

// testNow is a fixed clock for deterministic period/elapsed output in tests.
var testNow = time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)

func TestFormatTimeRemaining(t *testing.T) {
	fn := func(remaining time.Duration) (string, error) {
		return formatTimeRemaining(remaining), nil
	}
	cases := trial.Cases[time.Duration, string]{
		"zero":                   {Input: 0, Expected: "0 min"},
		"negative":               {Input: -time.Hour, Expected: "0 min"},
		"days only":              {Input: 12 * 24 * time.Hour, Expected: "12 days"},
		"one day over 48h":       {Input: 50 * time.Hour, Expected: "2 days"},
		"just over 48h":          {Input: 49 * time.Hour, Expected: "2 days"},
		"48h middle tier":        {Input: 48 * time.Hour, Expected: "2 days"},
		"days and hours":         {Input: 30 * time.Hour, Expected: "1 day 6 hr"},
		"hours only middle tier": {Input: 18 * time.Hour, Expected: "18 hr"},
		"12h middle tier":        {Input: 12 * time.Hour, Expected: "12 hr"},
		"hours and minutes":      {Input: 5*time.Hour + 15*time.Minute, Expected: "5 hr 15 min"},
		"minutes only":           {Input: 45 * time.Minute, Expected: "45 min"},
		"one hour":               {Input: time.Hour, Expected: "1 hr"},
		"one minute":             {Input: time.Minute, Expected: "1 min"},
	}
	trial.New(fn, cases).SubTest(t)
}

func TestFormatDollars(t *testing.T) {
	fn := func(cents float64) (string, error) {
		return formatDollars(cents), nil
	}
	cases := trial.Cases[float64, string]{
		"zero":      {Input: 0, Expected: "$0.00"},
		"99 cents":  {Input: 99, Expected: "$0.99"},
		"thousands": {Input: 200000, Expected: "$2,000.00"},
		"large":     {Input: 1234567, Expected: "$12,345.67"},
		"negative":  {Input: -200000, Expected: "-$2,000.00"},
	}
	trial.New(fn, cases).SubTest(t)
}

func TestRequestUsed(t *testing.T) {
	fn := func(r *UsageResult) (float64, error) {
		return r.requestUsed(), nil
	}
	cases := trial.Cases[*UsageResult, float64]{
		"fallback to plan used": {
			Input:    &UsageResult{RequestsUsed: 1124, RequestsLimit: 2000},
			Expected: 1124,
		},
		"uses breakdown total": {
			Input: &UsageResult{
				RequestsUsed:           2000,
				RequestsLimit:          2000,
				RequestsBreakdownTotal: floatPtr(2386),
			},
			Expected: 2386,
		},
	}
	trial.New(fn, cases).SubTest(t)
}

type requestPercentInput struct {
	used  float64
	limit float64
}

func TestRequestPercent(t *testing.T) {
	fn := func(in requestPercentInput) (float64, error) {
		return requestPercent(in.used, in.limit), nil
	}
	cases := trial.Cases[requestPercentInput, float64]{
		"under limit": {Input: requestPercentInput{1124, 2000}, Expected: 56.2},
		"over limit":  {Input: requestPercentInput{2500, 2000}, Expected: 125.0},
		"zero used":   {Input: requestPercentInput{0, 2000}, Expected: 0},
		"zero limit":  {Input: requestPercentInput{100, 0}, Expected: 0},
	}
	trial.New(fn, cases).Comparer(func(actual, expected interface{}) (bool, string) {
		got := actual.(float64)
		want := expected.(float64)
		if math.Abs(got-want) <= 0.05 {
			return true, ""
		}
		return false, fmt.Sprintf("got %v want %v", got, want)
	}).SubTest(t)
}

type parseUsageOutput struct {
	result *UsageResult
	output string
}

func TestParseUsageAndOutput(t *testing.T) {
	if loc, err := time.LoadLocation("UTC"); err == nil {
		time.Local = loc
	}

	fn := func(body string) (parseUsageOutput, error) {
		got, err := parseUsage([]byte(body))
		if err != nil {
			return parseUsageOutput{}, err
		}
		return parseUsageOutput{
			result: got,
			output: got.Format(testNow),
		}, nil
	}
	cases := trial.Cases[string, parseUsageOutput]{
		"on_demand_enabled": {
			Input: `{
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
			Expected: parseUsageOutput{
				result: &UsageResult{
					MembershipType:  "enterprise",
					TotalPercent:    56.2,
					AutoPercent:     0,
					APIPercent:      100,
					RequestsUsed:    1124,
					RequestsLimit:   2000,
					OnDemandEnabled: true,
					OnDemandUsed:    200000,
				},
				output: `Cursor usage
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
		},
		"on_demand_disabled": {
			Input: `{
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
			Expected: parseUsageOutput{
				result: &UsageResult{
					MembershipType:  "pro",
					TotalPercent:    30.4,
					AutoPercent:     42.5,
					APIPercent:      18.3,
					RequestsUsed:    350,
					OnDemandEnabled: false,
				},
				output: `Cursor usage
-------------
Billing period:  — → —
Plan:            pro
Total usage:
  API (named):   18.3%
  Auto:          42.5%

On-demand:       Disabled

`,
			},
		},
		"team_usage_nested_on_demand": {
			Input: `{
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
			Expected: parseUsageOutput{
				result: &UsageResult{
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
				output: `Cursor usage
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
		},
		"breakdown_total_for_requests": {
			Input: `{
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
			Expected: parseUsageOutput{
				result: &UsageResult{
					MembershipType:         "pro",
					TotalPercent:           12.24,
					AutoPercent:            0,
					APIPercent:             53.02,
					RequestsUsed:           2000,
					RequestsLimit:          2000,
					RequestsBreakdownTotal: floatPtr(2386),
					OnDemandEnabled:        false,
				},
				output: `Cursor usage
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
		},
		"free plan without individual usage": {
			Input: `{
				"membershipType": "free",
				"individualUsage": false
			}`,
			Expected: parseUsageOutput{
				result: &UsageResult{
					MembershipType: "free",
				},
				output: `Cursor usage
-------------
Billing period:  — → —
Plan:            free
Total usage:
  API (named):   0.0%
  Auto:          0.0%

On-demand:       Disabled

`,
			},
		},
		"enterprise_team_limit": {
			Input: `{
  "billingCycleStart": "2026-08-01T00:00:00.000Z",
  "billingCycleEnd": "2026-09-01T00:00:00.000Z",
  "membershipType": "enterprise",
  "limitType": "team",
  "isUnlimited": false,
  "autoModelSelectedDisplayMessage": "Youve used 0% of your included total usage,namedModelSelectedDisplayMessage:Youve used 0% of your included API usage",
  "individualUsage": {
    "overall": {
      "enabled": true,
      "used": 233,
      "limit": 50000,
      "remaining": 49767
    }
  },
  "teamUsage": {
    "onDemand": {
      "enabled": true,
      "used": 1234,
      "limit": 12345,
      "remaining": 11111
    }
  }
}`,
			Expected: parseUsageOutput{
				result: &UsageResult{
					PeriodStart:       time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
					PeriodEnd:         time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
					MembershipType:    "enterprise",
					LimitType:         "team",
					AutoPercent:       0,
					APIPercent:        0,
					RequestsUsed:      233,
					RequestsLimit:     50000,
					RequestsRemaining: floatPtr(49767),
					Team: &TeamUsageInfo{
						Enabled:   true,
						Used:      1234,
						Limit:     floatPtr(12345),
						Remaining: floatPtr(11111),
					},
				},
				output: `Cursor usage
-------------
Billing period:  2026-08-01T00:00 UTC → 2026-09-01T00:00 UTC
Time remaining:  16 days (48.4% elapsed)
Plan:            enterprise (team limit)
Included usage:
  Spent:         $2.33 of $500.00 (0.5%)

Team on-demand:  $12.34 spent
  Limit:         $123.45
  Remaining:     $111.11

`,
			},
		},
	}
	trial.New(fn, cases).Comparer(func(actual, expected interface{}) (bool, string) {
		got := actual.(parseUsageOutput)
		want := expected.(parseUsageOutput)
		if diff := cmp.Diff(want.result, got.result); diff != "" {
			return false, "result:" + diff
		}
		if got.output != want.output {
			return false, fmt.Sprintf("output:\n%s", cmp.Diff(want.output, got.output))
		}
		return true, ""
	}).SubTest(t)
}

type periodStatsInput struct {
	start time.Time
	end   time.Time
	now   time.Time
}

type periodStatsOutput struct {
	remaining  time.Duration
	elapsedPct float64
	ok         bool
}

func TestPeriodStats(t *testing.T) {
	fn := func(in periodStatsInput) (periodStatsOutput, error) {
		r := &UsageResult{PeriodStart: in.start, PeriodEnd: in.end}
		remaining, elapsedPct, ok := r.periodStats(in.now)
		return periodStatsOutput{remaining, elapsedPct, ok}, nil
	}
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	cases := trial.Cases[periodStatsInput, periodStatsOutput]{
		"zero period start": {
			Input:    periodStatsInput{now: testNow},
			Expected: periodStatsOutput{},
		},
		"mid period": {
			Input:    periodStatsInput{start: start, end: end, now: testNow},
			Expected: periodStatsOutput{remaining: 16 * 24 * time.Hour, elapsedPct: 48.4, ok: true},
		},
		"past end": {
			Input:    periodStatsInput{start: start, end: end, now: end.Add(time.Hour)},
			Expected: periodStatsOutput{remaining: -time.Hour, elapsedPct: 100, ok: true},
		},
		"before start": {
			Input:    periodStatsInput{start: start, end: end, now: start.Add(-time.Hour)},
			Expected: periodStatsOutput{remaining: 31*24*time.Hour + time.Hour, elapsedPct: 0, ok: true},
		},
	}
	trial.New(fn, cases).Comparer(func(actual, expected interface{}) (bool, string) {
		got := actual.(periodStatsOutput)
		want := expected.(periodStatsOutput)
		if got.ok != want.ok {
			return false, fmt.Sprintf("ok: got %v want %v", got.ok, want.ok)
		}
		if got.remaining != want.remaining {
			return false, fmt.Sprintf("remaining: got %v want %v", got.remaining, want.remaining)
		}
		if math.Abs(got.elapsedPct-want.elapsedPct) > 0.05 {
			return false, fmt.Sprintf("elapsedPct: got %v want %v", got.elapsedPct, want.elapsedPct)
		}
		return true, ""
	}).SubTest(t)
}

func floatPtr(v float64) *float64 {
	return &v
}
