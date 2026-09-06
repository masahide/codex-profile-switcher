package openaiusage

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testQuery() UsageQuery {
	return UsageQuery{
		StartTime:  time.Date(2026, 9, 6, 0, 0, 0, 0, time.UTC),
		EndTime:    time.Date(2026, 9, 6, 3, 20, 0, 0, time.UTC),
		ProjectIDs: []string{"proj_aaa", "proj_bbb"},
	}
}

func testClient(server *httptest.Server) *Client {
	return &Client{
		HTTPClient: server.Client(),
		BaseURL:    server.URL,
		AdminKey: func() string {
			return "admin-key-used-only-in-test"
		},
		UserAgent: "cx/test",
		Timeout:   time.Second,
	}
}

func TestCompletionUsageRequestAndNestedResponse(t *testing.T) {
	var requestCount atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		if r.URL.Path != UsageEndpointPath {
			t.Errorf("path = %q, want %q", r.URL.Path, UsageEndpointPath)
		}
		if got, want := r.Header.Get("Authorization"), "Bearer admin-key-used-only-in-test"; got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("User-Agent"), "cx/test"; got != want {
			t.Errorf("User-Agent = %q, want %q", got, want)
		}
		query := r.URL.Query()
		if got, want := query.Get("start_time"), "1788652800"; got != want {
			t.Errorf("start_time = %q, want %q", got, want)
		}
		if got, want := query.Get("end_time"), "1788664800"; got != want {
			t.Errorf("end_time = %q, want %q", got, want)
		}
		if got, want := query.Get("bucket_width"), "1d"; got != want {
			t.Errorf("bucket_width = %q, want %q", got, want)
		}
		if got, want := query["group_by"], []string{"model", "service_tier"}; !reflect.DeepEqual(got, want) {
			t.Errorf("group_by = %v, want %v", got, want)
		}
		if got, want := query["project_ids"], []string{"proj_aaa", "proj_bbb"}; !reflect.DeepEqual(got, want) {
			t.Errorf("project_ids = %v, want %v", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "object": "page",
  "data": [{
    "object": "bucket",
    "start_time": 1788652800,
    "end_time": 1788664800,
    "results": [{
      "input_tokens": 1000,
      "input_cached_tokens": 400,
      "output_tokens": 500,
      "num_model_requests": 5,
      "project_id": "proj_aaa",
      "model": "gpt-5.6-sol",
      "batch": null,
      "service_tier": "default"
    }]
  }],
  "has_more": false
}`))
	}))
	defer server.Close()

	results, err := testClient(server).CompletionUsage(context.Background(), testQuery())
	if err != nil {
		t.Fatalf("CompletionUsage returned error: %v", err)
	}
	if requestCount.Load() != 1 {
		t.Fatalf("request count = %d, want 1", requestCount.Load())
	}
	if len(results) != 1 {
		t.Fatalf("result count = %d, want 1", len(results))
	}
	if results[0].InputTokens != 1000 || results[0].OutputTokens != 500 || results[0].InputCachedTokens != 400 {
		t.Fatalf("unexpected token result: %+v", results[0])
	}
}

func TestCompletionUsagePagination(t *testing.T) {
	var pages []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		pages = append(pages, page)
		w.Header().Set("Content-Type", "application/json")
		if page == "" {
			_, _ = w.Write([]byte(`{"data":[{"results":[{"model":"gpt-5.6-sol","input_tokens":10,"output_tokens":1}]}],"has_more":true,"next_page":"cursor-A"}`))
			return
		}
		if page != "cursor-A" {
			t.Errorf("unexpected page cursor %q", page)
		}
		_, _ = w.Write([]byte(`{"data":[{"results":[{"model":"gpt-5.6-terra","input_tokens":20,"output_tokens":2}]}],"has_more":false}`))
	}))
	defer server.Close()

	results, err := testClient(server).CompletionUsage(context.Background(), testQuery())
	if err != nil {
		t.Fatalf("CompletionUsage returned error: %v", err)
	}
	if !reflect.DeepEqual(pages, []string{"", "cursor-A"}) {
		t.Fatalf("pages = %v", pages)
	}
	if len(results) != 2 || results[1].InputTokens != 20 {
		t.Fatalf("results = %+v", results)
	}
}

func TestCompletionUsageMissingAdminKeyDoesNotRequest(t *testing.T) {
	var requestCount atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
	}))
	defer server.Close()
	t.Setenv(AdminAPIKeyEnvironment, "")
	t.Setenv("OPENAI_API_KEY", "project-key-must-not-be-used")

	client := testClient(server)
	client.AdminKey = nil
	_, err := client.CompletionUsage(context.Background(), testQuery())
	if !errors.Is(err, ErrAdminKeyRequired) {
		t.Fatalf("error = %v, want ErrAdminKeyRequired", err)
	}
	if requestCount.Load() != 0 {
		t.Fatalf("request count = %d, want 0", requestCount.Load())
	}
	if strings.Contains(err.Error(), "project-key-must-not-be-used") {
		t.Fatal("project API key appeared in error")
	}
}

func TestCompletionUsageHTTPStatusErrorsAreSafe(t *testing.T) {
	for _, test := range []struct {
		name      string
		status    int
		want      string
		requestID string
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, want: "authentication failed"},
		{name: "forbidden", status: http.StatusForbidden, want: "does not have permission"},
		{name: "rate limit", status: http.StatusTooManyRequests, want: "rate limit exceeded"},
		{name: "server", status: http.StatusBadGateway, want: "temporarily unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("x-request-id", "req_safe_123")
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte("server body must not be exposed"))
			}))
			defer server.Close()

			_, err := testClient(server).CompletionUsage(context.Background(), testQuery())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if !strings.Contains(err.Error(), "req_safe_123") {
				t.Fatalf("error = %v, want request ID", err)
			}
			if strings.Contains(err.Error(), "server body") || strings.Contains(err.Error(), "admin-key-used-only-in-test") {
				t.Fatalf("unsafe content appeared in error: %v", err)
			}
		})
	}
}

func TestCompletionUsageMalformedAndOversizedResponses(t *testing.T) {
	t.Run("malformed", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("not-json"))
		}))
		defer server.Close()
		_, err := testClient(server).CompletionUsage(context.Background(), testQuery())
		if err == nil || !strings.Contains(err.Error(), "invalid OpenAI Usage API response") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("oversized", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"data":[]}`))
		}))
		defer server.Close()
		client := testClient(server)
		client.MaxResponseBytes = 4
		_, err := client.CompletionUsage(context.Background(), testQuery())
		if !errors.Is(err, ErrResponseTooLarge) {
			t.Fatalf("error = %v, want ErrResponseTooLarge", err)
		}
	})
}

func TestCompletionUsageTimeout(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	client := testClient(server)
	client.Timeout = 20 * time.Millisecond
	_, err := client.CompletionUsage(context.Background(), testQuery())
	if !errors.Is(err, ErrRequestTimeout) {
		t.Fatalf("error = %v, want ErrRequestTimeout", err)
	}
}

func TestCompletionUsageRequiresHTTPS(t *testing.T) {
	client := &Client{BaseURL: "http://127.0.0.1:12345", AdminKey: func() string { return "key" }}
	_, err := client.CompletionUsage(context.Background(), testQuery())
	if err == nil || !strings.Contains(err.Error(), "requires an HTTPS") {
		t.Fatalf("error = %v", err)
	}
}

func TestAPIErrorDoesNotDecodeServerBody(t *testing.T) {
	var encoded APIError
	if err := json.Unmarshal([]byte(`{"StatusCode":401}`), &encoded); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if encoded.Error() == "" {
		t.Fatal("APIError.Error returned empty string")
	}
}
