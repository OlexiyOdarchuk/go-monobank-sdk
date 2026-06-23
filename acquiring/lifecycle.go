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
	// decline). Mono will retry on its own. Keep access during a grace
	// window rather than revoking on the first miss.
	HealthChargeFailed SubscriptionHealth = "charge_failed"

	// HealthWalletDead: the bound card token failed
	// ([WalletFailed]) — recurring charges cannot succeed until the
	// customer re-links a card. Prompt re-tokenization; a grace window
	// will not fix this on its own.
	HealthWalletDead SubscriptionHealth = "wallet_dead"

	// HealthCancelled: the subscription is cancelled. Terminal — revoke
	// access (after any already-paid period elapses).
	HealthCancelled SubscriptionHealth = "cancelled"

	// HealthUnknown: not enough signal to classify (nil input, unknown
	// enum). Treat conservatively — do not revoke on this alone.
	HealthUnknown SubscriptionHealth = "unknown"
)

// GraceEligible reports whether the state is a transient failure that
// warrants keeping access during a retry/grace window rather than
// revoking immediately. True only for [HealthChargeFailed].
func (h SubscriptionHealth) GraceEligible() bool { return h == HealthChargeFailed }

// Terminal reports whether the state ends the billing relationship
// for the current token — [HealthCancelled] (gone) or
// [HealthWalletDead] (needs a new card). Either way, automatic
// charging will not resume without merchant/customer action.
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

// ClassifySubscription maps a [SubscriptionStatusResponse] (what you
// poll, or receive on the StatusURL callback) to a
// [SubscriptionHealth]. Precedence, most-terminal first:
//
//	cancelled               → HealthCancelled
//	wallet token failed     → HealthWalletDead
//	active                  → HealthActive
//	anything else / nil     → HealthUnknown
//
// A non-zero Summary.TotalFailed is intentionally NOT treated as
// charge-failed here: an active subscription with past failures has
// since recovered. Per-charge failures arrive on the ChargeURL — use
// [ClassifyCharge] for those.
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

// ClassifyCharge maps a single subscription charge event (delivered
// on the ChargeURL callback, or read from
// [Client.SubscriptionPayments]) plus the bound card's wallet status
// to a [SubscriptionHealth]. The wallet status takes precedence: a
// failed token means the failure is not transient.
//
//	wallet failed                          → HealthWalletDead
//	charge failed (and wallet ok)          → HealthChargeFailed (grace)
//	charge success                         → HealthActive
//	charge new/pending                     → HealthActive
//	otherwise                              → HealthUnknown
//
// walletStatus may be "" when the event does not carry it — the
// classification then falls back to the payment status alone.
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
