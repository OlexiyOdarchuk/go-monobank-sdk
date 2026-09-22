package openbanking

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// accountsPath is the account-information collection.
const accountsPath = pathPrefix + "/v2/accounts"

// AccountsResponse wraps the account list. [AccountList] rather than
// a plain slice so that logging it stays redacted.
type AccountsResponse struct {
	Accounts AccountList `json:"accounts"`
}

// BalancesResponse is the result of [Client.Balances].
type BalancesResponse struct {
	Account  AccountReference `json:"account"`
	Balances []Balance        `json:"balances"`
}

// TransactionsResponse is one page of a statement, plus the account
// it belongs to.
type TransactionsResponse struct {
	Account AccountReference `json:"account"`
	// Transactions is absent when the page carries no entries.
	Transactions *AccountReport `json:"transactions,omitempty"`
}

// Accounts lists the accounts a consent unlocks;
// [AccountDetails.ResourceID] is the identifier the other two
// account-information calls take. The spec does not state how the
// list relates to the rights the consent granted, so do not assume
// every account here also carries [AccessBalances].
//
// Set opts.WithBalance to have the balances embedded instead of
// calling [Client.Balances] per account — one request against
// frequencyPerDay instead of N+1.
func (c *Client) Accounts(ctx context.Context, opts AccountsOptions) (AccountList, error) {
	if opts.ConsentID == "" {
		return nil, ErrMissingConsentID
	}
	uri := accountsPath
	if opts.WithBalance {
		uri += "?withBalance=true"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	opts.apply(req.Header)

	var out AccountsResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}

	return out.Accounts, nil
}

// Balances returns every balance of one account. accountID is the
// [AccountDetails.ResourceID] from [Client.Accounts], not an IBAN.
//
// The consent must grant [AccessBalances] on that account. The spec
// does not say whether a consent without that right is refused or
// simply answered with nothing, so handle both.
func (c *Client) Balances(ctx context.Context, accountID string, opts AccountOptions) (*BalancesResponse, error) {
	if badPathSegment(accountID) {
		return nil, ErrEmptyID
	}
	if opts.ConsentID == "" {
		return nil, ErrMissingConsentID
	}
	uri := accountsPath + "/" + url.PathEscape(accountID) + "/balances"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	opts.apply(req.Header)

	var out BalancesResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}

	return &out, nil
}

// Transactions returns one page of an account's statement. The
// consent must grant [AccessTransactions] on that account.
//
// opts.BookingStatus is mandatory. Paging is cursor-based: when
// Transactions.Links.Next is set, pull its pageId query parameter
// (see [NextPageID]) into opts.PageID and repeat the call.
func (c *Client) Transactions(
	ctx context.Context, accountID string, opts TransactionsOptions,
) (*TransactionsResponse, error) {
	if badPathSegment(accountID) {
		return nil, ErrEmptyID
	}
	if opts.ConsentID == "" {
		return nil, ErrMissingConsentID
	}
	if opts.BookingStatus == "" {
		return nil, ErrMissingBookingStatus
	}

	q := url.Values{}
	q.Set("bookingStatus", string(opts.BookingStatus))
	if !opts.DateFrom.IsZero() {
		q.Set("dateFrom", opts.DateFrom.String())
	}
	if !opts.DateTo.IsZero() {
		q.Set("dateTo", opts.DateTo.String())
	}
	if opts.PageID != "" {
		q.Set("pageId", opts.PageID)
	}

	uri := accountsPath + "/" + url.PathEscape(accountID) + "/transactions?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	opts.apply(req.Header)

	var out TransactionsResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}

	return &out, nil
}

// NextPageID extracts the pageId cursor from a statement's next-page
// link, so callers do not have to parse the href by hand:
//
//	for {
//		page, err := cli.Transactions(ctx, id, opts)
//		if err != nil {
//			return err
//		}
//		// … consume page.Transactions.Booked …
//		next, ok := openbanking.NextPageID(page)
//		if !ok {
//			break
//		}
//		opts.PageID = next
//	}
//
// It reports false when the page is the last one, when the link is
// missing, or when the href is not parsable — all of which mean
// "stop", never "retry".
func NextPageID(page *TransactionsResponse) (string, bool) {
	if page == nil || page.Transactions == nil || page.Transactions.Links.Next == nil {
		return "", false
	}
	u, err := url.Parse(page.Transactions.Links.Next.Href)
	if err != nil {
		return "", false
	}
	id := u.Query().Get("pageId")

	return id, id != ""
}
