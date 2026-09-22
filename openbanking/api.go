package openbanking

import "context"

// API is the interface of the Open Banking client. It exists
// separately from *[Client] so callers can mock it via
// mockgen/testify-mock in their own tests — which matters more here
// than elsewhere in the SDK, since a real call needs a QWAC issued by
// a qualified trust-service provider. [Client] implements this
// interface (verified by the compile-time assert below).
//
// Grouped by topic, in the order of a typical integration.
type API interface {
	// Account-access consents.
	CreateConsent(ctx context.Context, in *CreateConsentRequest, opts InitiationOptions) (*CreateConsentResponse, error)
	StartConsentAuthorisation(ctx context.Context, consentID string) (*StartAuthorisationResponse, error)
	Consent(ctx context.Context, consentID string) (*Consent, error)
	ConsentStatus(ctx context.Context, consentID string) (*ConsentStatusResponse, error)
	RevokeConsent(ctx context.Context, consentID string) error

	// Account information (AIS).
	Accounts(ctx context.Context, opts AccountsOptions) (AccountList, error)
	Balances(ctx context.Context, accountID string, opts AccountOptions) (*BalancesResponse, error)
	Transactions(ctx context.Context, accountID string, opts TransactionsOptions) (*TransactionsResponse, error)

	// Payment initiation (PIS).
	CreatePayment(ctx context.Context, product PaymentProduct, in *CreatePaymentRequest,
		opts InitiationOptions) (*CreatePaymentResponse, error)
	Payment(ctx context.Context, product PaymentProduct, paymentID string) (*Payment, error)
	PaymentStatus(ctx context.Context, product PaymentProduct, paymentID string) (*PaymentStatusResponse, error)
	CancelPayment(ctx context.Context, product PaymentProduct, paymentID string) error
	StartPaymentAuthorisation(ctx context.Context, product PaymentProduct, paymentID string,
		opts AuthorisationOptions) (*StartAuthorisationResponse, error)
	PaymentAuthorisations(ctx context.Context, product PaymentProduct, paymentID string) ([]string, error)
	PaymentAuthorisationStatus(ctx context.Context, product PaymentProduct,
		paymentID, authorisationID string) (*AuthorisationStatusResponse, error)

	// Test integration (sandbox only).
	RequestTestCertificate(ctx context.Context, in *CertificateRequest) (*CertificateRequestResponse, error)
	TestCertificateStatus(ctx context.Context, csrPEM string) (*CertificateStatusResponse, error)
}

// Compile-time assert: *Client satisfies [API].
var _ API = (*Client)(nil)
