package main

import (
	"bytes"
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
	PeriodStart     string
	PeriodEnd       string
	TotalPercent    float64
	AutoPercent     float64
	APIPercent      float64
	OnDemandEnabled bool
	OnDemandUsed    float64 // cents
}

// usageSummaryResponse maps the fields we care about from /api/usage-summary.
type usageSummaryResponse struct {
	BillingCycleStart string `json:"billingCycleStart"`
	BillingCycleEnd   string `json:"billingCycleEnd"`
	// Error field present when the API rejects the cookie.
	Error       string          `json:"error"`
	Description string          `json:"description"`
	IndividualUsage json.RawMessage `json:"individualUsage"`
}

// planUsage is nested inside individualUsage.plan.
type planUsage struct {
	Used             float64 `json:"used"`
	Limit            float64 `json:"limit"`
	Remaining        float64 `json:"remaining"`
	AutoPercentUsed  float64 `json:"autoPercentUsed"`
	APIPercentUsed   float64 `json:"apiPercentUsed"`
	TotalPercentUsed float64 `json:"totalPercentUsed"`
}

type onDemandUsage struct {
	Enabled bool    `json:"enabled"`
	Used    float64 `json:"used"`
}

type individualUsage struct {
	Plan     planUsage     `json:"plan"`
	OnDemand onDemandUsage `json:"onDemand"`
}

// invoiceResponse maps fields we care about from /api/dashboard/get-monthly-invoice.
// Only used for billing period dates as a fallback.
type invoiceResponse struct {
	PeriodStart      string `json:"period_start"`
	PeriodEnd        string `json:"period_end"`
	Start            string `json:"start"`
	End              string `json:"end"`
	CurrentPeriodEnd string `json:"current_period_end"`
}

// fetchUsage calls the two Cursor dashboard API endpoints and returns a
// combined UsageResult. It returns ErrAuthFailed (wrapped) when the cookie is
// rejected by the API (HTTP 401 or an error field in the JSON body).
func fetchUsage(ctx context.Context, cookie string, debug bool) (*UsageResult, error) {
	cookie = normalizeCookie(cookie)

	usageBody, err := doGet(ctx, cookie, baseURL+"/api/usage-summary", debug)
	if err != nil {
		return nil, fmt.Errorf("fetching usage-summary: %w", err)
	}

	invoiceBody, err := doPost(ctx, cookie, baseURL+"/api/dashboard/get-monthly-invoice", debug)
	if err != nil {
		return nil, fmt.Errorf("fetching monthly-invoice: %w", err)
	}

	return parseUsage(usageBody, invoiceBody)
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
	req.Header.Set("Cookie", cookie)
	req.Header.Set("User-Agent", userAgent)
	return executeRequest(req, debug)
}

func doPost(ctx context.Context, cookie, url string, debug bool) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBufferString("{}"))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", cookie)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/json")
	return executeRequest(req, debug)
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
		fmt.Printf("[debug] status %d body: %s\n", resp.StatusCode, body)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("HTTP 401: %w", ErrAuthFailed)
	}
	return body, nil
}

// parseUsage combines the two API responses into a UsageResult.
// Returns ErrAuthFailed if the usage-summary body contains an error field.
func parseUsage(usageBody, invoiceBody []byte) (*UsageResult, error) {
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

	var invoice invoiceResponse
	_ = json.Unmarshal(invoiceBody, &invoice) // invoice failure is non-fatal

	result := &UsageResult{}

	// Period dates: prefer invoice, fall back to usage-summary.
	result.PeriodStart = firstNonEmpty(invoice.PeriodStart, invoice.Start, summary.BillingCycleStart)
	result.PeriodEnd = firstNonEmpty(invoice.PeriodEnd, invoice.End, invoice.CurrentPeriodEnd, summary.BillingCycleEnd)

	if len(summary.IndividualUsage) > 0 {
		var iu individualUsage
		if err := json.Unmarshal(summary.IndividualUsage, &iu); err == nil {
			result.TotalPercent = iu.Plan.TotalPercentUsed
			result.AutoPercent = iu.Plan.AutoPercentUsed
			result.APIPercent = iu.Plan.APIPercentUsed
			result.OnDemandEnabled = iu.OnDemand.Enabled
			result.OnDemandUsed = iu.OnDemand.Used
		}
	}

	result.PeriodStart = stripTime(result.PeriodStart)
	result.PeriodEnd = stripTime(result.PeriodEnd)

	return result, nil
}

func stripTime(s string) string {
	if i := strings.IndexByte(s, 'T'); i != -1 {
		return s[:i]
	}
	return s
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
