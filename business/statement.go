package business

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Statement returns operations on account within [from, to]
// (inclusive). A zero to means "up to now". limit and direction map
// to the matching query parameters; pass StatementDirection("") for
// the API default.
//
// Unlike the Personal/Corporate Open API, there is no 31-day
// constraint here — corp-api returns as much as you ask for within
// limit.
// https://corp-api.monobank.ua/docs/#operation/get-statement
func (c *Client) Statement(ctx context.Context, account string, from, to time.Time,
	direction StatementDirection, limit int) ([]StatementItem, error) {

	// Refuse to construct a URL with the zero value of time.Time,
	// which encodes to Unix=-6795364578 and silently asks the bank for
	// a window stretching to the year -290308. A "to" of zero is
	// allowed and means "up to now".
	if from.IsZero() || from.Unix() < 0 {
		return nil, fmt.Errorf("%w: from must be a real (post-epoch) time", ErrInvalidTimeRange)
	}
	if !to.IsZero() && !to.After(from) {
		return nil, fmt.Errorf("%w: to must be strictly after from", ErrInvalidTimeRange)
	}

	uri := "/ext/v1/statement/" + url.PathEscape(account) + "/" + strconv.FormatInt(from.Unix(), 10)
	if !to.IsZero() {
		uri += "/" + strconv.FormatInt(to.Unix(), 10)
	}
	q := url.Values{}
	if direction != "" {
		q.Set("direction", string(direction))
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if s := q.Encode(); s != "" {
		uri += "?" + s
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out []StatementItem
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return out, nil
}

// Operation returns a single operation by Mono's id and your
// externalReference. Both must be supplied — the bank checks they
// match. Useful for confirming that your payment really landed in
// the statement.
// https://corp-api.monobank.ua/docs/#operation/get-payment-from-statement
func (c *Client) Operation(ctx context.Context, id, externalReference string) (*StatementItem, error) {
	q := url.Values{}
	q.Set("id", id)
	q.Set("externalReference", externalReference)
	uri := "/ext/v1/statement?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out StatementItem
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}
