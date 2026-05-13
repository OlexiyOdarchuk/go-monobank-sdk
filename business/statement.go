package business

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Statement повертає операції на рахунку account за період [from, to]
// (включно). to-нульовий означає «до теперішнього часу». limit і
// direction мапляться у відповідні query-параметри; передай
// StatementDirection("") для дефолту API.
//
// На відміну від Personal/Corporate Open API, тут немає 31-денного
// обмеження — corp-api повертає скільки попросиш у межах limit.
// https://corp-api.monobank.ua/docs/#operation/get-statement
func (c *Client) Statement(ctx context.Context, account string, from, to time.Time,
	direction StatementDirection, limit int) ([]StatementItem, error) {

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

// Operation повертає одну операцію за id Mono і твоїм externalReference.
// Треба передати обидва — банк звіряє відповідність. Корисно, щоб
// підтвердити, що твій платіж справді дійшов у виписку.
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
