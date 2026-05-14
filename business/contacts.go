package business

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// ErrNilRequest is returned from mutating endpoints when a nil body
// is passed.
var ErrNilRequest = errors.New("request body is nil")

// Contacts returns a page of the payroll-contacts directory. Pass
// limit=0 / offset=0 to use the API defaults (limit=100). For
// free-form search use [Client.SearchContacts].
// https://corp-api.monobank.ua/docs/#operation/get-salary-contacts
func (c *Client) Contacts(ctx context.Context, limit, offset int) (*ContactsPage, error) {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}
	uri := "/ext/v1/salary-contacts"
	if s := q.Encode(); s != "" {
		uri += "?" + s
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out ContactsPage
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// SearchContacts performs a full-text search across the payroll-
// contacts directory. query matches the INN (tax ID), IBAN, document
// number, full name, or card PAN. Pagination is limit/offset (0
// means API defaults).
// https://corp-api.monobank.ua/docs/#operation/search-salary-contacts
func (c *Client) SearchContacts(ctx context.Context, query string, limit, offset int) (*ContactsPage, error) {
	q := url.Values{}
	if query != "" {
		q.Set("query", query)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}
	uri := "/ext/v1/salary-contacts/search"
	if s := q.Encode(); s != "" {
		uri += "?" + s
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out ContactsPage
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// ContactByID returns a single contact by its UUID.
// https://corp-api.monobank.ua/docs/#operation/get-salary-contact-by-id
func (c *Client) ContactByID(ctx context.Context, id string) (*Contact, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"/ext/v1/salary-contacts/"+url.PathEscape(id), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	var out Contact
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateContact adds a new payroll contact to the directory. Either
// INN or the pair (DocumentType + DocumentNumber) is enough to
// identify the person.
// https://corp-api.monobank.ua/docs/#operation/create-salary-contact
func (c *Client) CreateContact(ctx context.Context, in *CreateContactRequest) error {
	if in == nil {
		return ErrNilRequest
	}
	body, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"/ext/v1/salary-contacts", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	return c.c.Do(req, nil, http.StatusOK)
}

// DeleteContact removes a contact by its UUID. For batch removal
// see [Client.DeleteContactsBatch].
// https://corp-api.monobank.ua/docs/#operation/delete-salary-contact-by-id
func (c *Client) DeleteContact(ctx context.Context, id string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		"/ext/v1/salary-contacts/"+url.PathEscape(id), http.NoBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	return c.c.Do(req, nil, http.StatusOK)
}

// DeleteContactsBatch removes a batch of contacts by their UUIDs.
// Faster than a sequential loop of [Client.DeleteContact] — one
// HTTP request.
// https://corp-api.monobank.ua/docs/#operation/delete-salary-contacts-batch
func (c *Client) DeleteContactsBatch(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return ErrNilRequest
	}
	body, err := json.Marshal(struct {
		IDs []string `json:"ids"`
	}{IDs: ids})
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		"/ext/v1/salary-contacts/batch", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	return c.c.Do(req, nil, http.StatusOK)
}
