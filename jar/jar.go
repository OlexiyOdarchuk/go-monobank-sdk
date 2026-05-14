// Package jar is the client for two public (no-auth) monobank
// endpoints that return information about "jars":
//
//   - GET  https://api.monobank.ua/bank/jar/{longJarId} returns the
//     full info for a jar by its "long" identifier.
//   - POST https://send.monobank.ua/api/handler looks up a jar by the
//     clientId from a share link (https://send.monobank.ua/<id>);
//     the response carries longJarId/extJarId for later use with
//     [Client.ByLongID].
//
// Both endpoints are documented only in community notes (see the
// pinned message in the @api_mono_chat channel). This is a read-only
// API: jars cannot be created or edited through it.
//
// The rate limits on send.monobank.ua are more aggressive than those
// on /bank/jar — for repeated requests, cache longJarId and use only
// /bank/jar.
package jar

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// Default endpoints — override via [WithAPIBaseURL] /
// [WithSendBaseURL].
const (
	defaultAPIBaseURL  = "https://api.monobank.ua"
	defaultSendBaseURL = "https://send.monobank.ua"
)

// ErrNotFound indicates the jar does not exist or the ID is invalid.
var ErrNotFound = errors.New("jar: not found")

// ErrEmptyJarID indicates that an empty longJarID was passed to
// [Client.ByLongID].
var ErrEmptyJarID = errors.New("jar: empty longJarID")

// ErrEmptyClientID indicates that an empty clientID was passed to
// [Client.ByShortID].
var ErrEmptyClientID = errors.New("jar: empty clientID")

// ErrInsecureBaseURL indicates [WithAPIBaseURL] / [WithSendBaseURL]
// received an http:// URL on a non-loopback host. Jar endpoints are
// public, but a man-in-the-middle on an http connection can spoof
// balances and owner names. Use [WithInsecureBaseURL] to opt in for
// debugging through a recorded proxy.
var ErrInsecureBaseURL = errors.New("jar: base URL must be https for non-loopback hosts")

// MaxResponseBytes is the cap on response size. Jar-endpoint bodies
// are tiny (<10 KiB); 1 MiB leaves plenty of headroom.
const MaxResponseBytes = 1 << 20

// APIError is the structured error from the monobank response body
// (errCode/errText).
type APIError struct {
	StatusCode int
	ErrCode    string `json:"errCode"`
	ErrText    string `json:"errText"`
}

// Error implements error.
func (e *APIError) Error() string {
	return fmt.Sprintf("jar: %d %s: %s", e.StatusCode, e.ErrCode, e.ErrText)
}

// Info is the stable subset of fields from /bank/jar/{longJarId}.
//
// Amount is in minor units of the currency (kopecks for UAH).
// Currency is the ISO 4217 numeric code (980 = UAH).
type Info struct {
	JarID     string `json:"jarId"`
	Title     string `json:"title"`
	OwnerName string `json:"ownerName"`
	OwnerIcon string `json:"ownerIcon"`
	Amount    int64  `json:"amount"`
	Goal      int64  `json:"goal"`
	Currency  int    `json:"currency"`
}

// SendInfo is the subset of fields from send.monobank.ua/api/handler
// (c="hello"). This endpoint returns a different format from
// /bank/jar — the field names differ, so we keep a separate type.
type SendInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Avatar      string `json:"avatar"`
	JarAmount   int64  `json:"jarAmount"`
	JarGoal     int64  `json:"jarGoal"`
	JarStatus   string `json:"jarStatus"`
	IsTrusted   bool   `json:"isTrusted"`
	// LongJarID/ExtJarID is the field for subsequent interaction with
	// /bank/jar. Different API versions ship it under different names;
	// we fill it from extJarId or longJarId, whichever appears in the
	// body first (see UnmarshalJSON).
	LongJarID string `json:"-"`
}

// UnmarshalJSON extracts longJarId/extJarId — different versions
// use different field names — and embeds the rest of SendInfo in a
// single decode pass.
func (s *SendInfo) UnmarshalJSON(data []byte) error {
	type raw SendInfo
	aux := struct {
		raw
		LongJarID string `json:"longJarId"`
		ExtJarID  string `json:"extJarId"`
	}{}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*s = SendInfo(aux.raw)
	switch {
	case aux.LongJarID != "":
		s.LongJarID = aux.LongJarID
	case aux.ExtJarID != "":
		s.LongJarID = aux.ExtJarID
	}
	return nil
}

// Option is a functional option for the [New] constructor.
type Option func(*Client)

// WithHTTPClient installs a custom *http.Client (default is an
// *http.Client with a 15-second timeout).
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.h = h }
}

// WithAPIBaseURL overrides the base URL for api.monobank.ua. Mainly
// for tests via httptest.Server. The jar endpoint is public, but a
// custom http proxy between the client and Mono can substitute the
// response (owner info, jar balance), so [New] refuses http:// URLs
// pointing at non-loopback hosts with [ErrInsecureBaseURL]. Opt out
// via [WithInsecureBaseURL] for a debugging proxy.
func WithAPIBaseURL(u string) Option {
	return func(c *Client) { c.apiBase = u }
}

// WithSendBaseURL overrides the base URL for send.monobank.ua. Same
// scheme guard as [WithAPIBaseURL].
func WithSendBaseURL(u string) Option {
	return func(c *Client) { c.sendBase = u }
}

// WithInsecureBaseURL allows http:// URLs on non-loopback hosts for
// [WithAPIBaseURL] / [WithSendBaseURL]. Default false. Order is
// irrelevant — [New] resolves the bypass before evaluating the
// scheme guard.
func WithInsecureBaseURL(allow bool) Option {
	return func(c *Client) { c.allowInsecureBaseURL = allow }
}

// Client is the read-only client for the two public jar endpoints.
// Build one via [New]. No authorization — both endpoints are public.
type Client struct {
	h                    *http.Client
	apiBase              string
	sendBase             string
	allowInsecureBaseURL bool
}

// New returns a client with a 15-second default timeout. It returns
// an error when [WithAPIBaseURL] / [WithSendBaseURL] were given an
// http:// URL on a non-loopback host (override with
// [WithInsecureBaseURL]).
func New(opts ...Option) (*Client, error) {
	c := &Client{
		h:        &http.Client{Timeout: 15 * time.Second},
		apiBase:  defaultAPIBaseURL,
		sendBase: defaultSendBaseURL,
	}
	for _, o := range opts {
		o(c)
	}
	if !c.allowInsecureBaseURL {
		if isInsecureBaseURL(c.apiBase) {
			return nil, fmt.Errorf("%w: apiBase=%q", ErrInsecureBaseURL, c.apiBase)
		}
		if isInsecureBaseURL(c.sendBase) {
			return nil, fmt.Errorf("%w: sendBase=%q", ErrInsecureBaseURL, c.sendBase)
		}
	}
	return c, nil
}

// isInsecureBaseURL reports whether u uses a non-https scheme on a
// non-loopback host. Mirrors monobank.isInsecureBaseURL.
func isInsecureBaseURL(u string) bool {
	parsed, err := url.Parse(u)
	if err != nil || parsed == nil {
		return false
	}
	if parsed.Scheme == "https" || parsed.Scheme == "" {
		return false
	}
	host := parsed.Hostname()
	if host == "localhost" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return false
	}
	return true
}

// ByLongID returns the current info for a jar by its long (longJarId
// / extJarId) identifier. You can find this ID in the jar widget URL
// or obtain it from [Client.ByShortID].
func (c *Client) ByLongID(ctx context.Context, longJarID string) (*Info, error) {
	if longJarID == "" {
		return nil, ErrEmptyJarID
	}
	u := c.apiBase + "/bank/jar/" + url.PathEscape(longJarID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("jar: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.h.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jar: do request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("jar: read response body: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, decodeAPIError(resp.StatusCode, body)
	}
	var out Info
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("jar: decode response: %w", err)
	}
	return &out, nil
}

// jarShortIDMode is the value of the "Pc" field expected by
// send.monobank.ua/api/handler. Mono treats it as a request-flow
// marker; "random" comes from the bank's own client-side code and
// is the only value confirmed to work in production.
const jarShortIDMode = "random"

// shortIDRequest is the body of POST to send.monobank.ua/api/handler.
type shortIDRequest struct {
	C        string `json:"c"`
	ClientID string `json:"clientId"`
	Pc       string `json:"Pc"`
}

// ByShortID looks up jar info by the short clientId (the one in the
// share-link URL https://send.monobank.ua/{clientId}). This call
// goes to send.monobank.ua, whose limits are stricter — cache the
// result.
//
// Useful for a one-off lookup of longJarId from a short link. For
// regular balance refreshes use [Client.ByLongID].
func (c *Client) ByShortID(ctx context.Context, clientID string) (*SendInfo, error) {
	if clientID == "" {
		return nil, ErrEmptyClientID
	}
	body, err := json.Marshal(shortIDRequest{C: "hello", ClientID: clientID, Pc: jarShortIDMode})
	if err != nil {
		return nil, fmt.Errorf("jar: marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.sendBase+"/api/handler", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("jar: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.h.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jar: do request: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("jar: read response body: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, decodeAPIError(resp.StatusCode, respBody)
	}
	// send.monobank.ua can return { "errCode": "..." } with status
	// 200. The previous check looked for ANY JSON shape carrying an
	// "errCode" field — too loose, since a real success response can
	// legitimately ship an empty "errCode": "" or be confused with a
	// future field. Use a strict probe: parse only the error shape
	// and require BOTH errCode and errText to be present.
	var probe struct {
		ErrCode string `json:"errCode"`
		ErrText string `json:"errText"`
	}
	if json.Unmarshal(respBody, &probe) == nil && probe.ErrCode != "" && probe.ErrText != "" {
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			ErrCode:    probe.ErrCode,
			ErrText:    probe.ErrText,
		}
	}
	var out SendInfo
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("jar: decode response: %w", err)
	}
	return &out, nil
}

func decodeAPIError(status int, body []byte) error {
	var e APIError
	if err := json.Unmarshal(body, &e); err != nil {
		return fmt.Errorf("jar: http %d: %s", status, string(body))
	}
	e.StatusCode = status
	return &e
}
