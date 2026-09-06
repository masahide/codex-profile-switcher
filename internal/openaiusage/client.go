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
	ErrAdminKeyRequired = errors.New("OPENAI_ADMIN_KEY is required for `cx quota`\nThe Organization Usage API requires an OpenAI Admin API key.\nThe key is used only for this request and is not stored by cx")
	ErrResponseTooLarge = errors.New("OpenAI Usage API response exceeds the maximum allowed size")
	ErrRequestTimeout   = errors.New("OpenAI Usage API request timed out")
)

// APIError contains only safe status metadata. The response body is never
// included because it may contain arbitrary server-controlled content.
type APIError struct {
	StatusCode int
	RequestID  string
}

func (e *APIError) Error() string {
	var message string
	switch {
	case e.StatusCode == http.StatusUnauthorized:
		message = "OpenAI Usage API authentication failed. Check OPENAI_ADMIN_KEY."
	case e.StatusCode == http.StatusForbidden:
		message = "OPENAI_ADMIN_KEY does not have permission to read organization usage. Use an appropriate Organization Admin API key."
	case e.StatusCode == http.StatusTooManyRequests:
		message = "OpenAI Usage API rate limit exceeded."
	case e.StatusCode >= 500 && e.StatusCode <= 599:
		message = "OpenAI Usage API is temporarily unavailable."
	default:
		message = fmt.Sprintf("OpenAI Usage API returned HTTP status %d.", e.StatusCode)
	}
	if requestID := safeRequestID(e.RequestID); requestID != "" {
		message += " (request ID: " + requestID + ")"
	}
	return message
}

// Client is a small HTTP client for the documented completions usage endpoint.
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
	base := c.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("invalid OpenAI Usage API base URL: %w", err)
	}
	if parsed.User != nil || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("invalid OpenAI Usage API base URL")
	}
	if parsed.Scheme != "https" {
		return nil, errors.New("OpenAI Usage API requires an HTTPS base URL")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + UsageEndpointPath
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

func validateQuery(query UsageQuery) error {
	if query.StartTime.IsZero() || query.EndTime.IsZero() {
		return errors.New("OpenAI Usage API query requires start and end times")
	}
	if query.EndTime.Before(query.StartTime) {
		return errors.New("OpenAI Usage API query end time precedes start time")
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

func validateUsageResult(result CompletionUsageResult) error {
	if result.InputTokens < 0 || result.OutputTokens < 0 || result.InputCachedTokens < 0 || result.NumModelRequests < 0 {
		return errors.New("OpenAI Usage API returned a negative usage value")
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
