package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type TokenProvider func(ctx context.Context, scopes []string) (string, error)

// RequestOptions controls behavior that must be selected for an individual
// Graph request.
type RequestOptions struct {
	// DisableRetries prevents automatic retries for non-idempotent requests.
	DisableRetries bool
}

// TransportError means the HTTP transport failed after the request was handed
// to http.Client. For non-idempotent writes, the server-side outcome may be
// indeterminate.
type TransportError struct {
	Err error
}

func (e *TransportError) Error() string {
	return fmt.Sprintf("request graph: %v", e.Err)
}

func (e *TransportError) Unwrap() error {
	return e.Err
}

// ResponseReadError means Graph returned an HTTP response but its body could
// not be read completely.
type ResponseReadError struct {
	Err error
}

func (e *ResponseReadError) Error() string {
	return fmt.Sprintf("read response: %v", e.Err)
}

func (e *ResponseReadError) Unwrap() error {
	return e.Err
}

// Client wraps Microsoft Graph HTTP behavior including auth, retry, and paging.
type Client struct {
	BaseURL       string
	HTTPClient    *http.Client
	TokenProvider TokenProvider
	MaxRetries429 int
	MaxRetries5xx int
	BaseDelay     time.Duration
	Breaker       *CircuitBreaker
}

// NewClient builds a Graph client with conservative retry and breaker defaults.
func NewClient(tokenProvider TokenProvider) *Client {
	return &Client{
		BaseURL:       "https://graph.microsoft.com/v1.0",
		HTTPClient:    http.DefaultClient,
		TokenProvider: tokenProvider,
		MaxRetries429: MaxRateLimitRetries,
		MaxRetries5xx: Max5xxRetries,
		BaseDelay:     RateLimitBaseDelay,
		Breaker:       NewCircuitBreaker(),
	}
}

func (c *Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}

	return http.DefaultClient
}

func (c *Client) baseURL() string {
	if strings.TrimSpace(c.BaseURL) == "" {
		return "https://graph.microsoft.com/v1.0"
	}

	return strings.TrimRight(c.BaseURL, "/")
}

func (c *Client) DoJSON(ctx context.Context, method string, path string, query url.Values, body any, scopes []string, out any) error {
	return c.DoJSONWithOptions(ctx, method, path, query, body, scopes, out, RequestOptions{})
}

func (c *Client) DoJSONWithOptions(ctx context.Context, method string, path string, query url.Values, body any, scopes []string, out any, options RequestOptions) error {
	_, b, err := c.DoWithOptions(ctx, method, path, query, body, scopes, nil, options)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}

	if len(b) == 0 {
		return nil
	}

	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("decode response json: %w", err)
	}

	return nil
}

func (c *Client) Do(ctx context.Context, method string, path string, query url.Values, body any, scopes []string, headers http.Header) (*http.Response, []byte, error) {
	return c.DoWithOptions(ctx, method, path, query, body, scopes, headers, RequestOptions{})
}

func (c *Client) DoWithOptions(ctx context.Context, method string, path string, query url.Values, body any, scopes []string, headers http.Header, options RequestOptions) (*http.Response, []byte, error) {
	if c.TokenProvider == nil {
		return nil, nil, errors.New("missing token provider")
	}

	if c.Breaker == nil {
		resp, payload, _, err := c.doWithRetry(ctx, method, path, query, body, scopes, headers, options)
		return resp, payload, err
	}

	var resp *http.Response
	var payload []byte
	err := c.Breaker.Execute(func() (bool, error) {
		var recordFailure bool
		var callErr error
		resp, payload, recordFailure, callErr = c.doWithRetry(ctx, method, path, query, body, scopes, headers, options)
		return recordFailure, callErr
	})
	if err != nil {
		return resp, payload, err
	}

	return resp, payload, nil
}

func (c *Client) doWithRetry(
	ctx context.Context,
	method string,
	path string,
	query url.Values,
	body any,
	scopes []string,
	headers http.Header,
	options RequestOptions,
) (*http.Response, []byte, bool, error) {
	retries429 := 0
	retries5xx := 0
	maxRetries429 := c.MaxRetries429
	maxRetries5xx := c.MaxRetries5xx
	if options.DisableRetries {
		maxRetries429 = 0
		maxRetries5xx = 0
	}

	for {
		resp, b, err := c.doOnce(ctx, method, path, query, body, scopes, headers)
		if err != nil {
			return resp, nil, true, err
		}

		if resp.StatusCode < 400 {
			return resp, b, false, nil
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			if retries429 >= maxRetries429 {
				apiErr := parseAPIError(resp.StatusCode, b)
				return resp, b, false, apiErr
			}

			delay := retryDelay(resp.Header.Get("Retry-After"), retries429, c.BaseDelay)
			if err := sleepContext(ctx, delay); err != nil {
				return nil, nil, false, err
			}
			retries429++
			continue
		}

		if resp.StatusCode >= 500 {
			if retries5xx >= maxRetries5xx {
				apiErr := parseAPIError(resp.StatusCode, b)
				return resp, b, true, apiErr
			}

			if err := sleepContext(ctx, ServerErrorRetryDelay); err != nil {
				return nil, nil, false, err
			}
			retries5xx++
			continue
		}

		apiErr := parseAPIError(resp.StatusCode, b)
		return resp, b, false, apiErr
	}
}

func (c *Client) doOnce(ctx context.Context, method string, path string, query url.Values, body any, scopes []string, headers http.Header) (*http.Response, []byte, error) {
	endpoint, err := c.resolveURL(path, query)
	if err != nil {
		return nil, nil, err
	}

	var payload io.Reader
	rawBody := false
	if body != nil {
		switch typed := body.(type) {
		case []byte:
			payload = bytes.NewReader(typed)
			rawBody = true
		case string:
			payload = strings.NewReader(typed)
			rawBody = true
		default:
			encoded, err := json.Marshal(body)
			if err != nil {
				return nil, nil, fmt.Errorf("encode request json: %w", err)
			}
			payload = bytes.NewReader(encoded)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, payload)
	if err != nil {
		return nil, nil, fmt.Errorf("build request: %w", err)
	}

	token, err := c.TokenProvider(ctx, scopes)
	if err != nil {
		return nil, nil, fmt.Errorf("acquire token: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		if rawBody {
			req.Header.Set("Content-Type", "application/octet-stream")
		} else {
			req.Header.Set("Content-Type", "application/json")
		}
	}
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, nil, &TransportError{Err: err}
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, nil, &ResponseReadError{Err: err}
	}

	return resp, b, nil
}

func (c *Client) resolveURL(path string, query url.Values) (string, error) {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		u, err := url.Parse(path)
		if err != nil {
			return "", fmt.Errorf("parse path: %w", err)
		}
		base, err := url.Parse(c.baseURL())
		if err != nil {
			return "", fmt.Errorf("parse base URL: %w", err)
		}
		if !sameHost(base, u) {
			return "", fmt.Errorf("cross-host URL is not allowed: %s", u.Host)
		}
		if len(query) > 0 {
			u.RawQuery = query.Encode()
		}
		return u.String(), nil
	}

	u, err := url.Parse(c.baseURL() + "/" + strings.TrimLeft(path, "/"))
	if err != nil {
		return "", fmt.Errorf("build request URL: %w", err)
	}
	if len(query) > 0 {
		u.RawQuery = query.Encode()
	}

	return u.String(), nil
}

func sameHost(base *url.URL, target *url.URL) bool {
	if base == nil || target == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(base.Host), strings.TrimSpace(target.Host))
}

func (c *Client) Paginate(ctx context.Context, path string, query url.Values, scopes []string, max int) ([]map[string]any, string, error) {
	items := make([]map[string]any, 0)
	nextPath := path
	nextQuery := query

	for {
		_, b, err := c.Do(ctx, http.MethodGet, nextPath, nextQuery, nil, scopes, nil)
		if err != nil {
			return nil, "", err
		}

		var payload map[string]any
		if err := json.Unmarshal(b, &payload); err != nil {
			return nil, "", fmt.Errorf("decode paged response: %w", err)
		}

		if rawItems, ok := payload["value"].([]any); ok {
			for _, raw := range rawItems {
				if item, ok := raw.(map[string]any); ok {
					items = append(items, item)
					if max > 0 && len(items) >= max {
						next := toString(payload["@odata.nextLink"])
						return items[:max], next, nil
					}
				}
			}
		}

		next := toString(payload["@odata.nextLink"])
		if next == "" {
			return items, "", nil
		}

		nextPath = next
		nextQuery = nil
	}
}

func retryDelay(retryAfter string, attempt int, base time.Duration) time.Duration {
	if v := strings.TrimSpace(retryAfter); v != "" {
		if sec, err := strconv.Atoi(v); err == nil {
			if sec < 0 {
				return 0
			}
			return time.Duration(sec) * time.Second
		}

		if t, err := http.ParseTime(v); err == nil {
			d := time.Until(t)
			if d < 0 {
				return 0
			}
			return d
		}
	}

	if base <= 0 {
		return 0
	}

	step := base * time.Duration(1<<attempt)
	if step <= 0 {
		return 0
	}
	jitterRange := step / 2
	if jitterRange <= 0 {
		return step
	}
	jitter := time.Duration(rand.Int64N(int64(jitterRange))) //nolint:gosec // jitter only
	return step + jitter
}

func parseAPIError(status int, body []byte) error {
	var payload struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && (payload.Error.Code != "" || payload.Error.Message != "") {
		return &APIError{Status: status, Code: payload.Error.Code, Message: payload.Error.Message}
	}

	message := strings.TrimSpace(string(body))
	if message == "" {
		message = http.StatusText(status)
	}
	return &APIError{Status: status, Message: message}
}

func toString(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
