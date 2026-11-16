package acquiring

// SubscriptionHealth is a coarse, action-oriented classification of a
// subscription's state, distilled from the raw status/wallet/charge
// enums. It exists to drive grace logic: a single failed charge on an
// otherwise active subscription is recoverable and should NOT revoke
// access immediately, whereas a dead card token or a cancellation is
// terminal for the current billing relationship.
type SubscriptionHealth string

// Possible [SubscriptionHealth] values.
const (
	// HealthActive: the subscription is live and the last signal was
	// healthy (a successful or still-pending charge). Grant access.
	HealthActive SubscriptionHealth = "active"

	// HealthChargeFailed: the subscription is still active but a charge
	// failed for a transient reason (insufficient funds, issuer
	// decline). Mono does not document whether or when a failed charge
	// is retried. Keep access during a grace window rather than
	// revoking on the first miss.
	HealthChargeFailed SubscriptionHealth = "charge_failed"

	// HealthWalletDead: walletData.status is [WalletFailed], which the
	// docs describe only as "the card could not be tokenized". They do
	// not say whether further charges are attempted, and no
	// subscription endpoint re-binds a card — creating a new
	// subscription is the only documented way to attach one. The SDK
	// treats this state as terminal; confirm with
	// [Client.SubscriptionStatus] before acting on it.
	HealthWalletDead SubscriptionHealth = "wallet_dead"

	// HealthCancelled: the subscription is cancelled. Terminal — revoke
	// access once the already-paid period elapses. The API does not
	// return that date after cancellation; use the NextChargeDate you
	// stored while the subscription was active.
	HealthCancelled SubscriptionHealth = "cancelled"

	// HealthUnknown: not enough signal to classify (nil input, unknown
	// enum). Treat conservatively — do not revoke on this alone.
	HealthUnknown SubscriptionHealth = "unknown"
)

// GraceEligible reports whether the state is a transient failure that
// warrants keeping access during a retry/grace window rather than
// revoking immediately. True only for [HealthChargeFailed].
func (h SubscriptionHealth) GraceEligible() bool { return h == HealthChargeFailed }

// Terminal reports whether the SDK treats the state as ending the
// billing relationship — [HealthCancelled] (cancelled) or
// [HealthWalletDead] (the card token failed; see its note on what
// the docs do and do not say).
func (h SubscriptionHealth) Terminal() bool {
	return h == HealthCancelled || h == HealthWalletDead
}

// RetainAccess is the suggested grace decision: keep serving the
// customer while the state is healthy or recoverable
// ([HealthActive], [HealthChargeFailed], or [HealthUnknown] — erring
// toward not cutting off a paying customer on ambiguous signal), and
// revoke on the terminal states.
func (h SubscriptionHealth) RetainAccess() bool {
	return !h.Terminal()
}

// ClassifySubscription maps a [SubscriptionStatusResponse] (from
// [Client.SubscriptionStatus], typically fetched when the StatusURL
// callback fires) to a [SubscriptionHealth]. Precedence, most-terminal first:
//
//	cancelled               → HealthCancelled
//	wallet token failed     → HealthWalletDead
//	active                  → HealthActive
//	anything else / nil     → HealthUnknown
//
// A non-zero Summary.TotalFailed is intentionally NOT treated as
// charge-failed here: an active subscription with past failures has
// since recovered. For per-charge failures (signaled on the
// ChargeURL) use [ClassifyCharge].
func ClassifySubscription(st *SubscriptionStatusResponse) SubscriptionHealth {
	if st == nil {
		return HealthUnknown
	}
	if st.Status == SubscriptionCancelled {
		return HealthCancelled
	}
	if st.WalletData.Status == WalletFailed {
		return HealthWalletDead
	}
	if st.Status == SubscriptionActive {
		return HealthActive
	}
	return HealthUnknown
}

// ClassifyCharge maps a single subscription charge (read from
// [Client.SubscriptionPayments], typically when the ChargeURL callback
// fires) plus the bound card's wallet status to a
// [SubscriptionHealth]. The wallet status takes precedence: the SDK
// treats a failed token as not transient (see [HealthWalletDead]).
//
//	wallet failed                          → HealthWalletDead
//	charge failed (and wallet ok)          → HealthChargeFailed (grace)
//	charge success                         → HealthActive
//	charge new/pending                     → HealthActive
//	otherwise                              → HealthUnknown
//
// walletStatus may be "" when it is not at hand — the classification
// then falls back to the payment status alone.
func ClassifyCharge(payment SubscriptionPaymentStatus, walletStatus WalletStatus) SubscriptionHealth {
	if walletStatus == WalletFailed {
		return HealthWalletDead
	}
	switch payment {
	case SubscriptionPaymentFailed:
		return HealthChargeFailed
	case SubscriptionPaymentSuccess, SubscriptionPaymentNew:
		return HealthActive
	default:
		return HealthUnknown
	}
}
