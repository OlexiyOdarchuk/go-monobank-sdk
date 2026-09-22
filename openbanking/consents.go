package openbanking

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// consentsPath is the collection every consent operation hangs off.
const consentsPath = pathPrefix + "/v2/consents/account-access"

// CreateConsentRequest asks the PSU for access to a named set of
// accounts. Every field is mandatory.
type CreateConsentRequest struct {
	// Access lists the accounts and the rights wanted on each.
	Access AccountAccess `json:"access"`
	// ConsentType must be [ConsentDetailed]; the API accepts no
	// other value on creation.
	ConsentType ConsentType `json:"consentType"`
	// RecurringIndicator distinguishes a standing consent, reusable
	// until ValidTo, from a one-shot one.
	RecurringIndicator bool `json:"recurringIndicator"`
	// ValidTo is the last day the consent may be used.
	ValidTo Date `json:"validTo"`
	// FrequencyPerDay is the number of account-information calls per
	// day the TPP commits to. The spec does not say how the bank
	// reacts once the budget is spent — see the package doc.
	FrequencyPerDay int `json:"frequencyPerDay"`
}

// ConsentLinks is the _links object of a freshly created consent.
type ConsentLinks struct {
	// StartAuthorisation is the URL behind
	// [Client.StartConsentAuthorisation]; it is informational, the
	// SDK builds the path itself.
	StartAuthorisation *Href `json:"startAuthorisation,omitempty"`
}

// CreateConsentResponse is the body of a successful
// [Client.CreateConsent].
type CreateConsentResponse struct {
	// ConsentID feeds [AccountOptions.ConsentID] once the consent
	// reaches [ConsentValid].
	ConsentID     string        `json:"consentId"`
	ConsentStatus ConsentStatus `json:"consentStatus"`
	Links         ConsentLinks  `json:"_links,omitzero"`
}

// AuthorisationLinks is the _links object of a started
// authorisation, for both consents and payments.
type AuthorisationLinks struct {
	// SCARedirect is where the PSU must be sent. For a payment
	// authorisation the spec states it appears only under the
	// [SCARedirect] approach — a legal entity authorises in its own
	// app and gets no link. For a consent authorisation the spec
	// gives no such rule, so check for nil instead of inferring the
	// approach from it.
	SCARedirect *Href `json:"scaRedirect,omitempty"`
	// SCAStatus is the polling URL for the authorisation status. The
	// consent flow does not return it; the payment flow does.
	SCAStatus *Href `json:"scaStatus,omitempty"`
}

// StartAuthorisationResponse is the body returned when an
// authorisation process is started, for both consents and payments.
type StartAuthorisationResponse struct {
	AuthorisationID string             `json:"authorisationId"`
	SCAStatus       SCAStatus          `json:"scaStatus"`
	Links           AuthorisationLinks `json:"_links,omitzero"`
}

// Consent is the stored state of an account-access consent.
type Consent struct {
	Access             AccountAccess `json:"access"`
	ConsentType        ConsentType   `json:"consentType"`
	RecurringIndicator bool          `json:"recurringIndicator"`
	ValidTo            Date          `json:"validTo"`
	FrequencyPerDay    int           `json:"frequencyPerDay"`
	ConsentStatus      ConsentStatus `json:"consentStatus"`
}

// ConsentStatusResponse is the lightweight status-only projection of
// a consent.
type ConsentStatusResponse struct {
	ConsentStatus ConsentStatus `json:"consentStatus"`
}

// CreateConsent registers an account-access consent and returns its
// id. The consent starts as [ConsentReceived] and is useless until
// the PSU authorises it — continue with
// [Client.StartConsentAuthorisation].
//
// opts.PSU.IPAddress is mandatory. Identify the customer through
// PSU.ID/IDType for a natural person or sole proprietor, and through
// PSU.CorporateID/CorporateIDType for a legal entity.
func (c *Client) CreateConsent(
	ctx context.Context, in *CreateConsentRequest, opts InitiationOptions,
) (*CreateConsentResponse, error) {
	if in == nil {
		return nil, ErrNilRequest
	}
	if opts.PSU.IPAddress == "" {
		return nil, ErrMissingPSUIPAddress
	}
	body, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, consentsPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	opts.PSU.apply(req.Header)
	setIfNotEmpty(req.Header, HeaderTPPRedirectURI, opts.TPPRedirectURI)

	var out CreateConsentResponse
	if err := c.c.Do(req, &out, http.StatusCreated); err != nil {
		return nil, err
	}

	return &out, nil
}

// StartConsentAuthorisation opens the SCA process for a consent. The
// PSU then has to confirm it: under [SCARedirect] send them to
// Links.SCARedirect, under [SCADecoupled] they confirm in their own
// banking app and nothing is returned to redirect to.
//
// Track the outcome with [Client.ConsentStatus] — the consent becomes
// usable at [ConsentValid].
func (c *Client) StartConsentAuthorisation(ctx context.Context, consentID string) (*StartAuthorisationResponse, error) {
	if badPathSegment(consentID) {
		return nil, ErrEmptyID
	}
	uri := consentsPath + "/" + url.PathEscape(consentID) + "/authorisations"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uri, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	var out StartAuthorisationResponse
	if err := c.c.Do(req, &out, http.StatusCreated); err != nil {
		return nil, err
	}

	return &out, nil
}

// Consent reads back the full consent: the accounts and rights it
// covers, its validity and its current status.
func (c *Client) Consent(ctx context.Context, consentID string) (*Consent, error) {
	if badPathSegment(consentID) {
		return nil, ErrEmptyID
	}
	uri := consentsPath + "/" + url.PathEscape(consentID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	var out Consent
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}

	return &out, nil
}

// ConsentStatus reads only the status of a consent. Prefer it to
// [Client.Consent] when polling an authorisation: the payload is a
// single field, but the call still counts against the consent's
// frequencyPerDay, so poll with a backoff rather than in a tight
// loop.
func (c *Client) ConsentStatus(ctx context.Context, consentID string) (*ConsentStatusResponse, error) {
	if badPathSegment(consentID) {
		return nil, ErrEmptyID
	}
	uri := consentsPath + "/" + url.PathEscape(consentID) + "/status"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	var out ConsentStatusResponse
	if err := c.c.Do(req, &out, http.StatusOK); err != nil {
		return nil, err
	}

	return &out, nil
}

// RevokeConsent withdraws a consent. Call it when the customer
// unsubscribes — leaving a live consent behind is what turns into a
// data-protection complaint. The bank answers 204 with no body and
// the spec does not name the state the consent lands in;
// [ConsentTerminatedByTPP] is the plausible one, so read it back with
// [Client.ConsentStatus] if you need certainty.
func (c *Client) RevokeConsent(ctx context.Context, consentID string) error {
	if badPathSegment(consentID) {
		return ErrEmptyID
	}
	uri := consentsPath + "/" + url.PathEscape(consentID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, uri, http.NoBody)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	return c.c.Do(req, nil, http.StatusNoContent)
}
