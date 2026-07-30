package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrAuthFailed is returned when the API reports an authentication failure
// (expired cookie, invalid token, or HTTP 401). Callers check with errors.Is.
var ErrAuthFailed = errors.New("auth failed")

const (
	baseURL   = "https://cursor.com"
	userAgent = "Mozilla/5.0 (Windows NT 10.0; rv:109.0) Gecko/20100101 Firefox/115.0"
)

// UsageResult holds the billing data we show to the user.
type UsageResult struct {
	PeriodStart      time.Time
	PeriodEnd        time.Time
	MembershipType   string
	TotalPercent     float64
	AutoPercent      float64
	APIPercent       float64
	RequestsUsed             float64
	RequestsLimit            float64
	RequestsBreakdownTotal   *float64 // when set, used for request % instead of RequestsUsed
	OnDemandEnabled  bool
	OnDemandUsed     float64 // cents
	Team             *TeamUsageInfo
}

// TeamUsageInfo holds team-level on-demand spend from usage-summary.
type TeamUsageInfo struct {
	Enabled   bool
	Used      float64 // cents
	Limit     *float64
	Remaining *float64
}

// usageSummaryResponse maps the fields we care about from /api/usage-summary.
type usageSummaryResponse struct {
	BillingCycleStart string `json:"billingCycleStart"`
	BillingCycleEnd   string `json:"billingCycleEnd"`
	MembershipType    string `json:"membershipType"`
	// Error field present when the API rejects the cookie.
	Error           string          `json:"error"`
	Description     string          `json:"description"`
	IndividualUsage json.RawMessage `json:"individualUsage"`
	TeamUsage       json.RawMessage `json:"teamUsage"`
}

type usageBreakdown struct {
	Included float64 `json:"included"`
	Bonus    float64 `json:"bonus"`
	Total    float64 `json:"total"`
}

// planUsage is nested inside individualUsage.plan.
type planUsage struct {
	Used             float64         `json:"used"`
	Limit            float64         `json:"limit"`
	Remaining        float64         `json:"remaining"`
	Breakdown        *usageBreakdown `json:"breakdown"`
	AutoPercentUsed  float64         `json:"autoPercentUsed"`
	APIPercentUsed   float64         `json:"apiPercentUsed"`
	TotalPercentUsed float64         `json:"totalPercentUsed"`
}

type onDemandUsage struct {
	Enabled bool    `json:"enabled"`
	Used    float64 `json:"used"`
}

type individualUsage struct {
	Plan     planUsage     `json:"plan"`
	OnDemand onDemandUsage `json:"onDemand"`
}

// teamUsageBlock matches teamUsage when fields are at the top level.
type teamUsageBlock struct {
	Enabled   bool     `json:"enabled"`
	Used      float64  `json:"used"`
	Limit     *float64 `json:"limit"`
	Remaining *float64 `json:"remaining"`
}

type teamUsageWrapper struct {
	OnDemand teamUsageBlock `json:"onDemand"`
}

// fetchUsage calls /api/usage-summary and returns a UsageResult.
// It returns ErrAuthFailed (wrapped) when the cookie is rejected by the API
// (HTTP 401 or an error field in the JSON body).
func fetchUsage(ctx context.Context, cookie string, debug bool) (*UsageResult, error) {
	cookie = normalizeCookie(cookie)

	usageBody, err := doGet(ctx, cookie, baseURL+"/api/usage-summary", debug)
	if err != nil {
		return nil, fmt.Errorf("fetching usage-summary: %w", err)
	}

	return parseUsage(usageBody)
}

// normalizeCookie ensures the value is prefixed with "WorkosCursorSessionToken=".
func normalizeCookie(v string) string {
	v = strings.TrimSpace(v)
	v = strings.Trim(v, "\r")
	if strings.HasPrefix(v, "WorkosCursorSessionToken=") {
		return v
	}
	return "WorkosCursorSessionToken=" + v
}

func doGet(ctx context.Context, cookie, url string, debug bool) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	setRequestHeaders(req, cookie)
	return executeRequest(req, debug)
}

func setRequestHeaders(req *http.Request, cookie string) {
	req.Header.Set("Cookie", cookie)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
}

func executeRequest(req *http.Request, debug bool) ([]byte, error) {
	if debug {
		fmt.Printf("[debug] %s %s\n", req.Method, req.URL)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading body: %w", err)
	}
	if debug {
		fmt.Printf("[debug] status %d body:\n %s\n", resp.StatusCode, body)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("HTTP 401: %w", ErrAuthFailed)
	}
	return body, nil
}

// parseUsage parses the usage-summary response into a UsageResult.
// Returns ErrAuthFailed if the body contains an error field.
func parseUsage(usageBody []byte) (*UsageResult, error) {
	var summary usageSummaryResponse
	if err := json.Unmarshal(usageBody, &summary); err != nil {
		return nil, fmt.Errorf("parsing usage-summary: %w", err)
	}
	if summary.Error != "" {
		msg := summary.Description
		if msg == "" {
			msg = summary.Error
		}
		return nil, fmt.Errorf("API error %q: %w", msg, ErrAuthFailed)
	}

	result := &UsageResult{}
	result.PeriodStart = parseAPITime(summary.BillingCycleStart)
	result.PeriodEnd = parseAPITime(summary.BillingCycleEnd)
	result.MembershipType = summary.MembershipType
	if tu, ok := parseTeamUsage(summary.TeamUsage); ok {
		result.Team = &tu
	}

	if len(summary.IndividualUsage) > 0 {
		var iu individualUsage
		if err := json.Unmarshal(summary.IndividualUsage, &iu); err == nil {
			result.TotalPercent = iu.Plan.TotalPercentUsed
			result.AutoPercent = iu.Plan.AutoPercentUsed
			result.APIPercent = iu.Plan.APIPercentUsed
			result.RequestsUsed = iu.Plan.Used
			result.RequestsLimit = iu.Plan.Limit
			if iu.Plan.Breakdown != nil {
				total := iu.Plan.Breakdown.Total
				result.RequestsBreakdownTotal = &total
			}
			result.OnDemandEnabled = iu.OnDemand.Enabled
			result.OnDemandUsed = iu.OnDemand.Used
		}
	}

	return result, nil
}

func parseAPITime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04:05.000Z", s)
		if err != nil {
			return time.Time{}
		}
	}
	return t
}

func parseTeamUsage(raw json.RawMessage) (TeamUsageInfo, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return TeamUsageInfo{}, false
	}
	var flat teamUsageBlock
	if err := json.Unmarshal(raw, &flat); err == nil {
		if info, ok := teamUsageFromBlock(flat); ok {
			return info, true
		}
	}
	var nested teamUsageWrapper
	if err := json.Unmarshal(raw, &nested); err == nil {
		if info, ok := teamUsageFromBlock(nested.OnDemand); ok {
			return info, true
		}
	}
	return TeamUsageInfo{}, false
}

func teamUsageFromBlock(b teamUsageBlock) (TeamUsageInfo, bool) {
	if !b.Enabled && b.Used == 0 && b.Limit == nil && b.Remaining == nil {
		return TeamUsageInfo{}, false
	}
	return TeamUsageInfo{
		Enabled:   b.Enabled,
		Used:      b.Used,
		Limit:     b.Limit,
		Remaining: b.Remaining,
	}, true
}
