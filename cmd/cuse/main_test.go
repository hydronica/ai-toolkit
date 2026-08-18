package main

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"os"
	"testing"
	"time"

	"github.com/hydronica/trial"
)

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
		return requestUsed(r), nil
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
	fn := func(body string) (parseUsageOutput, error) {
		got, err := parseUsage([]byte(body))
		if err != nil {
			return parseUsageOutput{}, err
		}
		return parseUsageOutput{
			result: got,
			output: capturePrintTable(got),
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
	}
	trial.New(fn, cases).SubTest(t)
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
