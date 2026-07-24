package prometheus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxResponseSize = 32 << 20

var (
	ErrInvalidAddress = errors.New("invalid Prometheus address")
	ErrUnavailable    = errors.New("Prometheus is unavailable")
	ErrQuery          = errors.New("Prometheus query failed")
	ErrTimeout        = errors.New("Prometheus query timed out")
)

type Sample struct {
	Timestamp float64
	Value     float64
}

type Series struct {
	Labels  map[string]string
	Samples []Sample
}

type Range struct {
	Start time.Time
	End   time.Time
	Step  time.Duration
}

type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
	timeout    time.Duration
}

func NewClient(address string, timeout time.Duration) (*Client, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, fmt.Errorf("%w: address is empty", ErrInvalidAddress)
	}
	baseURL, err := url.Parse(address)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidAddress, err)
	}
	if (baseURL.Scheme != "http" && baseURL.Scheme != "https") || baseURL.Host == "" {
		return nil, fmt.Errorf("%w: expected an http or https URL", ErrInvalidAddress)
	}
	if baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return nil, fmt.Errorf("%w: query parameters and fragments are not supported", ErrInvalidAddress)
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("%w: timeout must be positive", ErrInvalidAddress)
	}

	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{},
		timeout:    timeout,
	}, nil
}

func (c *Client) Query(ctx context.Context, query string, at time.Time) ([]Series, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("%w: query is required", ErrQuery)
	}
	params := url.Values{"query": []string{query}}
	if !at.IsZero() {
		params.Set("time", formatTimestamp(at))
	}
	return c.do(ctx, "/api/v1/query", params)
}

func (c *Client) QueryRange(ctx context.Context, query string, queryRange Range) ([]Series, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("%w: query is required", ErrQuery)
	}
	if queryRange.Start.IsZero() || queryRange.End.IsZero() {
		return nil, fmt.Errorf("%w: start and end are required", ErrQuery)
	}
	if !queryRange.Start.Before(queryRange.End) {
		return nil, fmt.Errorf("%w: start must be before end", ErrQuery)
	}
	if queryRange.Step <= 0 {
		return nil, fmt.Errorf("%w: step must be positive", ErrQuery)
	}

	params := url.Values{
		"query": []string{query},
		"start": []string{formatTimestamp(queryRange.Start)},
		"end":   []string{formatTimestamp(queryRange.End)},
		"step":  []string{formatSeconds(queryRange.Step)},
	}
	return c.do(ctx, "/api/v1/query_range", params)
}

type apiResponse struct {
	Status    string  `json:"status"`
	Data      apiData `json:"data"`
	ErrorType string  `json:"errorType"`
	Error     string  `json:"error"`
}

type apiData struct {
	ResultType string          `json:"resultType"`
	Result     json.RawMessage `json:"result"`
}

type apiSeries struct {
	Metric map[string]string   `json:"metric"`
	Value  []json.RawMessage   `json:"value"`
	Values [][]json.RawMessage `json:"values"`
}

func (c *Client) do(ctx context.Context, endpointPath string, params url.Values) ([]Series, error) {
	requestURL := *c.baseURL
	requestURL.Path = strings.TrimRight(requestURL.Path, "/") + endpointPath
	requestURL.RawPath = ""
	requestURL.RawQuery = params.Encode()

	queryCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(queryCtx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: create request: %v", ErrQuery, err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, requestFailure(queryCtx, c.timeout, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
	if err != nil {
		return nil, requestFailure(queryCtx, c.timeout, err)
	}
	if len(body) > maxResponseSize {
		return nil, fmt.Errorf("%w: response exceeds %d bytes", ErrQuery, maxResponseSize)
	}

	var envelope apiResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
		if resp.StatusCode >= http.StatusInternalServerError {
			return nil, fmt.Errorf("%w: HTTP %d returned invalid JSON", ErrUnavailable, resp.StatusCode)
		}
		return nil, fmt.Errorf("%w: decode response: %v", ErrQuery, err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		message := prometheusErrorMessage(envelope, resp.Status)
		if resp.StatusCode >= http.StatusInternalServerError {
			return nil, fmt.Errorf("%w: %s", ErrUnavailable, message)
		}
		return nil, fmt.Errorf("%w: %s", ErrQuery, message)
	}
	if envelope.Status != "success" {
		return nil, fmt.Errorf("%w: %s", ErrQuery, prometheusErrorMessage(envelope, resp.Status))
	}

	series, err := decodeResult(envelope.Data)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrQuery, err)
	}
	return series, nil
}

func decodeResult(data apiData) ([]Series, error) {
	switch data.ResultType {
	case "vector", "matrix":
		var rawSeries []apiSeries
		if err := json.Unmarshal(data.Result, &rawSeries); err != nil {
			return nil, fmt.Errorf("decode %s result: %w", data.ResultType, err)
		}
		result := make([]Series, 0, len(rawSeries))
		for _, raw := range rawSeries {
			pairs := raw.Values
			if data.ResultType == "vector" {
				pairs = [][]json.RawMessage{raw.Value}
			}
			samples, err := decodeSamples(pairs)
			if err != nil {
				return nil, err
			}
			result = append(result, Series{Labels: raw.Metric, Samples: samples})
		}
		return result, nil
	case "scalar":
		var pair []json.RawMessage
		if err := json.Unmarshal(data.Result, &pair); err != nil {
			return nil, fmt.Errorf("decode scalar result: %w", err)
		}
		samples, err := decodeSamples([][]json.RawMessage{pair})
		if err != nil {
			return nil, err
		}
		return []Series{{Labels: map[string]string{}, Samples: samples}}, nil
	default:
		return nil, fmt.Errorf("unsupported result type %q", data.ResultType)
	}
}

func decodeSamples(pairs [][]json.RawMessage) ([]Sample, error) {
	result := make([]Sample, 0, len(pairs))
	for i, pair := range pairs {
		if len(pair) != 2 {
			return nil, fmt.Errorf("sample %d must contain a timestamp and value", i)
		}
		timestamp, err := decodeFloat(pair[0])
		if err != nil || math.IsNaN(timestamp) || math.IsInf(timestamp, 0) {
			return nil, fmt.Errorf("sample %d has an invalid timestamp", i)
		}
		value, err := decodeFloat(pair[1])
		if err != nil {
			return nil, fmt.Errorf("sample %d has an invalid value: %w", i, err)
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			continue
		}
		result = append(result, Sample{Timestamp: timestamp, Value: value})
	}
	return result, nil
}

func decodeFloat(raw json.RawMessage) (float64, error) {
	var text string
	if len(raw) > 0 && raw[0] == '"' {
		if err := json.Unmarshal(raw, &text); err != nil {
			return 0, err
		}
	} else {
		text = string(raw)
	}
	return strconv.ParseFloat(text, 64)
}

func requestFailure(ctx context.Context, timeout time.Duration, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("%w after %s", ErrTimeout, timeout)
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return fmt.Errorf("Prometheus query canceled: %w", context.Canceled)
	}
	return fmt.Errorf("%w: %v", ErrUnavailable, err)
}

func prometheusErrorMessage(response apiResponse, fallback string) string {
	message := strings.TrimSpace(response.Error)
	if message == "" {
		message = strings.TrimSpace(fallback)
	}
	if response.ErrorType != "" {
		return response.ErrorType + ": " + message
	}
	return message
}

func formatTimestamp(value time.Time) string {
	seconds := float64(value.Unix()) + float64(value.Nanosecond())/float64(time.Second)
	return strconv.FormatFloat(seconds, 'f', -1, 64)
}

func formatSeconds(value time.Duration) string {
	return strconv.FormatFloat(value.Seconds(), 'f', -1, 64)
}
