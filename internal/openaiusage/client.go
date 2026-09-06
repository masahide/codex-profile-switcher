package openaiusage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultRequestTimeout = 10 * time.Second
	defaultMaxBodyBytes   = 8 << 20
	maxPaginationPages    = 10000
)

var (
	ErrAdminKeyRequired = errors.New("OPENAI_ADMIN_KEY is required for `cx quota`\nThe Organization Usage and Costs APIs require an OpenAI Admin API key.\nThe key is used only for these requests and is not stored by cx")
	ErrResponseTooLarge = errors.New("OpenAI Usage API response exceeds the maximum allowed size")
	ErrRequestTimeout   = errors.New("OpenAI Usage API request timed out")
)

// APIError contains only safe status metadata. The response body is never
// included because it may contain arbitrary server-controlled content.
type APIError struct {
	StatusCode int
	RequestID  string
	APIName    string
}

func (e *APIError) Error() string {
	apiName := e.apiName()
	var message string
	switch {
	case e.StatusCode == http.StatusUnauthorized:
		message = apiName + " authentication failed. Check OPENAI_ADMIN_KEY."
	case e.StatusCode == http.StatusForbidden:
		message = apiName + " authorization failed. OPENAI_ADMIN_KEY does not have permission to read organization data. Use an appropriate Organization Admin API key."
	case e.StatusCode == http.StatusTooManyRequests:
		message = apiName + " rate limit exceeded."
	case e.StatusCode >= 500 && e.StatusCode <= 599:
		message = apiName + " is temporarily unavailable."
	default:
		message = fmt.Sprintf("%s returned HTTP status %d.", apiName, e.StatusCode)
	}
	if requestID := safeRequestID(e.RequestID); requestID != "" {
		message += " (request ID: " + requestID + ")"
	}
	return message
}

func (e *APIError) apiName() string {
	if name := strings.TrimSpace(e.APIName); name != "" {
		return name
	}
	return "OpenAI Usage API"
}

// Client is a small HTTP client for the documented organization Usage and
// Costs endpoints.
// AdminKey is a function so the key can be read just-in-time from the
// environment and never needs to be persisted in a Client configuration file.
type Client struct {
	HTTPClient       *http.Client
	BaseURL          string
	UserAgent        string
	AdminKey         func() string
	MaxResponseBytes int64
	Timeout          time.Duration
}

func NewClient(version string) *Client {
	if version == "" {
		version = "dev"
	}
	return &Client{
		HTTPClient:       &http.Client{},
		BaseURL:          DefaultBaseURL,
		UserAgent:        "cx/" + version,
		MaxResponseBytes: defaultMaxBodyBytes,
		Timeout:          defaultRequestTimeout,
	}
}

// CompletionUsage gets all pages and flattens each bucket's results.
func (c *Client) CompletionUsage(ctx context.Context, query UsageQuery) ([]CompletionUsageResult, error) {
	adminKey := c.adminKey()
	if adminKey == "" {
		return nil, ErrAdminKeyRequired
	}
	if err := validateQuery(query); err != nil {
		return nil, err
	}

	baseURL, err := c.endpointURL()
	if err != nil {
		return nil, err
	}
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	requestContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	page := query.Page
	seenPages := make(map[string]struct{})
	allResults := make([]CompletionUsageResult, 0)
	for pageNumber := 0; ; pageNumber++ {
		if pageNumber >= maxPaginationPages {
			return nil, errors.New("OpenAI Usage API pagination exceeded the safety limit")
		}
		if page != "" {
			if _, seen := seenPages[page]; seen {
				return nil, errors.New("OpenAI Usage API returned a repeated pagination cursor")
			}
			seenPages[page] = struct{}{}
		}

		pageQuery := query
		pageQuery.Page = page
		requestURL, err := buildRequestURL(baseURL, pageQuery)
		if err != nil {
			return nil, err
		}
		request, err := http.NewRequestWithContext(requestContext, http.MethodGet, requestURL.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("create OpenAI Usage API request: %w", err)
		}
		request.Header.Set("Authorization", "Bearer "+adminKey)
		request.Header.Set("User-Agent", c.userAgent())
		request.Header.Set("Accept", "application/json")

		response, err := httpClient.Do(request)
		if err != nil {
			if requestContext.Err() == context.DeadlineExceeded || errors.Is(err, context.DeadlineExceeded) {
				return nil, ErrRequestTimeout
			}
			return nil, fmt.Errorf("OpenAI Usage API request failed: %w", err)
		}
		pageResponse, err := decodePage(response, c.maxResponseBytes())
		if err != nil {
			if requestContext.Err() == context.DeadlineExceeded {
				return nil, ErrRequestTimeout
			}
			return nil, err
		}
		for _, bucket := range pageResponse.Data {
			for _, result := range bucket.Results {
				if err := validateUsageResult(result); err != nil {
					return nil, err
				}
				allResults = append(allResults, result)
			}
		}
		if !pageResponse.HasMore {
			return allResults, nil
		}
		if pageResponse.NextPage == "" {
			return nil, errors.New("OpenAI Usage API indicated more pages without a next_page cursor")
		}
		page = pageResponse.NextPage
	}
}

// Costs gets all pages and flattens each bucket's line-item results.
func (c *Client) Costs(ctx context.Context, query UsageQuery) ([]CostResult, error) {
	adminKey := c.adminKey()
	if adminKey == "" {
		return nil, ErrAdminKeyRequired
	}
	if err := validateQueryFor(query, "Costs"); err != nil {
		return nil, err
	}

	baseURL, err := c.endpointURLFor(CostsEndpointPath, "Costs")
	if err != nil {
		return nil, err
	}
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	requestContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	page := query.Page
	seenPages := make(map[string]struct{})
	allResults := make([]CostResult, 0)
	for pageNumber := 0; ; pageNumber++ {
		if pageNumber >= maxPaginationPages {
			return nil, errors.New("OpenAI Costs API pagination exceeded the safety limit")
		}
		if page != "" {
			if _, seen := seenPages[page]; seen {
				return nil, errors.New("OpenAI Costs API returned a repeated pagination cursor")
			}
			seenPages[page] = struct{}{}
		}

		pageQuery := query
		pageQuery.Page = page
		requestURL, err := buildCostsRequestURL(baseURL, pageQuery)
		if err != nil {
			return nil, err
		}
		request, err := http.NewRequestWithContext(requestContext, http.MethodGet, requestURL.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("create OpenAI Costs API request: %w", err)
		}
		request.Header.Set("Authorization", "Bearer "+adminKey)
		request.Header.Set("User-Agent", c.userAgent())
		request.Header.Set("Accept", "application/json")

		response, err := httpClient.Do(request)
		if err != nil {
			if requestContext.Err() == context.DeadlineExceeded || errors.Is(err, context.DeadlineExceeded) {
				return nil, ErrRequestTimeout
			}
			return nil, fmt.Errorf("OpenAI Costs API request failed: %w", err)
		}
		pageResponse, err := decodeCostsPage(response, c.maxResponseBytes())
		if err != nil {
			if requestContext.Err() == context.DeadlineExceeded {
				return nil, ErrRequestTimeout
			}
			return nil, err
		}
		for _, bucket := range pageResponse.Data {
			for _, result := range bucket.Results {
				if err := validateCostResult(result); err != nil {
					return nil, err
				}
				allResults = append(allResults, result)
			}
		}
		if !pageResponse.HasMore {
			return allResults, nil
		}
		if pageResponse.NextPage == "" {
			return nil, errors.New("OpenAI Costs API indicated more pages without a next_page cursor")
		}
		page = pageResponse.NextPage
	}
}

func (c *Client) adminKey() string {
	if c.AdminKey != nil {
		return c.AdminKey()
	}
	return os.Getenv(AdminAPIKeyEnvironment)
}

func (c *Client) userAgent() string {
	if c.UserAgent != "" {
		return c.UserAgent
	}
	return "cx/dev"
}

func (c *Client) maxResponseBytes() int64 {
	if c.MaxResponseBytes > 0 {
		return c.MaxResponseBytes
	}
	return defaultMaxBodyBytes
}

func (c *Client) endpointURL() (*url.URL, error) {
	return c.endpointURLFor(UsageEndpointPath, "Usage")
}

func (c *Client) endpointURLFor(endpointPath, serviceName string) (*url.URL, error) {
	base := c.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("invalid OpenAI %s API base URL: %w", serviceName, err)
	}
	if parsed.User != nil || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("invalid OpenAI %s API base URL", serviceName)
	}
	if parsed.Scheme != "https" {
		return nil, fmt.Errorf("OpenAI %s API requires an HTTPS base URL", serviceName)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + endpointPath
	return parsed, nil
}

func buildRequestURL(endpoint *url.URL, query UsageQuery) (*url.URL, error) {
	result := *endpoint
	values := result.Query()
	values.Set("start_time", strconv.FormatInt(query.StartTime.Unix(), 10))
	values.Set("end_time", strconv.FormatInt(query.EndTime.Unix(), 10))
	values.Set("bucket_width", "1d")
	values.Add("group_by", "model")
	values.Add("group_by", "service_tier")
	for _, projectID := range query.ProjectIDs {
		values.Add("project_ids", projectID)
	}
	if query.Page != "" {
		values.Set("page", query.Page)
	}
	if query.Limit > 0 {
		values.Set("limit", strconv.Itoa(query.Limit))
	}
	result.RawQuery = values.Encode()
	return &result, nil
}

func buildCostsRequestURL(endpoint *url.URL, query UsageQuery) (*url.URL, error) {
	result := *endpoint
	values := result.Query()
	values.Set("start_time", strconv.FormatInt(query.StartTime.Unix(), 10))
	values.Set("end_time", strconv.FormatInt(query.EndTime.Unix(), 10))
	values.Set("bucket_width", "1d")
	values.Add("group_by", "line_item")
	for _, projectID := range query.ProjectIDs {
		values.Add("project_ids", projectID)
	}
	if query.Page != "" {
		values.Set("page", query.Page)
	}
	if query.Limit > 0 {
		values.Set("limit", strconv.Itoa(query.Limit))
	}
	result.RawQuery = values.Encode()
	return &result, nil
}

func validateQuery(query UsageQuery) error {
	return validateQueryFor(query, "Usage")
}

func validateQueryFor(query UsageQuery, serviceName string) error {
	if query.StartTime.IsZero() || query.EndTime.IsZero() {
		return fmt.Errorf("OpenAI %s API query requires start and end times", serviceName)
	}
	if query.EndTime.Before(query.StartTime) {
		return fmt.Errorf("OpenAI %s API query end time precedes start time", serviceName)
	}
	return nil
}

type usagePage struct {
	Data     []usageBucket `json:"data"`
	HasMore  bool          `json:"has_more"`
	NextPage string        `json:"next_page"`
}

type usageBucket struct {
	Results []CompletionUsageResult `json:"results"`
}

type costsPage struct {
	Data     []costsBucket `json:"data"`
	HasMore  bool          `json:"has_more"`
	NextPage string        `json:"next_page"`
}

type costsBucket struct {
	Results []CostResult `json:"results"`
}

func decodePage(response *http.Response, maxBytes int64) (usagePage, error) {
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return usagePage{}, &APIError{
			StatusCode: response.StatusCode,
			RequestID:  response.Header.Get("x-request-id"),
		}
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return usagePage{}, fmt.Errorf("read OpenAI Usage API response: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return usagePage{}, ErrResponseTooLarge
	}
	var page usagePage
	if err := json.Unmarshal(body, &page); err != nil {
		return usagePage{}, fmt.Errorf("invalid OpenAI Usage API response: %w", err)
	}
	return page, nil
}

func decodeCostsPage(response *http.Response, maxBytes int64) (costsPage, error) {
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return costsPage{}, &APIError{
			StatusCode: response.StatusCode,
			RequestID:  response.Header.Get("x-request-id"),
			APIName:    "OpenAI Costs API",
		}
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return costsPage{}, fmt.Errorf("read OpenAI Costs API response: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return costsPage{}, fmt.Errorf("%w: OpenAI Costs API response exceeds the maximum allowed size", ErrResponseTooLarge)
	}
	var page costsPage
	if err := json.Unmarshal(body, &page); err != nil {
		return costsPage{}, fmt.Errorf("invalid OpenAI Costs API response: %w", err)
	}
	return page, nil
}

func validateUsageResult(result CompletionUsageResult) error {
	if result.InputTokens < 0 || result.OutputTokens < 0 || result.InputCachedTokens < 0 || result.NumModelRequests < 0 {
		return errors.New("OpenAI Usage API returned a negative usage value")
	}
	return nil
}

func validateCostResult(result CostResult) error {
	if strings.TrimSpace(result.Amount.Currency) != "" && strings.ContainsAny(result.Amount.Currency, "\r\n") {
		return errors.New("OpenAI Costs API returned an invalid currency")
	}
	if strings.ContainsAny(result.LineItem, "\r\n") || strings.ContainsAny(result.ProjectID, "\r\n") {
		return errors.New("OpenAI Costs API returned invalid result metadata")
	}
	return nil
}

func safeRequestID(requestID string) string {
	requestID = strings.TrimSpace(requestID)
	if strings.ContainsAny(requestID, "\r\n") {
		return ""
	}
	return requestID
}
