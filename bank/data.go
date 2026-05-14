// Package bank exposes the data types returned by the Open API
// (shared by the personal and corporate clients), plus the two
// unauthorized public endpoints: currency rates (/bank/currency) and
// the server key (/bank/sync).
//
// [Client] satisfies the webhook.KeyProvider interface, so the same
// instance can be used both for routine rate fetching and as a key
// source for the webhook handler:
//
//	keys := bank.New()
//	rates, _ := keys.Rates(ctx)
//	h, _    := webhook.NewHandler(ctx, webhook.Options{Keys: keys, ...})
//
// [ClientInfo], [Account], [Jar] and [Transaction] are used
// identically by the personal and corporate clients (each one only
// adds its own authorization — the response shapes are identical).
package bank

import (
	"encoding/json"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/currency"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/mcc"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/money"
	"github.com/vtopc/epoch"
)

// MaxStatementWindow is the largest interval /personal/statement
// accepts per call (31 days). For wider ranges, use TransactionsRange
// on the personal/corporate clients — it automatically slices into
// 31-day windows.
const MaxStatementWindow = 31 * 24 * time.Hour

// ClientInfo is the response of /personal/client-info: what the bank
// knows about the current client (name, accounts, jars, current
// webhook subscription).
type ClientInfo struct {
	ID         string   `json:"clientId"`
	Name       string   `json:"name"`
	WebHookURL string   `json:"webHookUrl"`
	Accounts   Accounts `json:"accounts"`
	Jars       Jars     `json:"jars"`
}

// LogValue implements [slog.LogValuer] so that logging a ClientInfo
// does not splash full name, IBANs and card masks into the log. Each
// sensitive field is replaced by a redacted summary (length / count
// only) — enough for debugging that the SDK fetched something, not
// enough to leak banking secrecy if the log winds up in a corporate
// log aggregator.
//
// Without this, slog at Debug level would render the raw struct,
// including every account's IBAN and every CardMask. Convert
// manually via the named fields if you really need the cleartext
// values in a controlled context.
func (c ClientInfo) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("clientId", c.ID),
		slog.String("name", maskName(c.Name)),
		slog.String("webHookUrl", redactURL(c.WebHookURL)),
		slog.Int("accounts", len(c.Accounts)),
		slog.Int("jars", len(c.Jars)),
	)
}

// LogValue implements [slog.LogValuer] for a single Account, hiding
// the IBAN and card masks.
func (a Account) LogValue() slog.Value {
	masks := make([]string, len(a.CardMasks))
	for i, m := range a.CardMasks {
		masks[i] = redactCardMask(m)
	}
	return slog.GroupValue(
		slog.String("id", a.AccountID),
		slog.String("type", string(a.Type)),
		slog.String("currency", a.Currency.String()),
		slog.String("iban", redactIBAN(a.IBAN)),
		slog.Any("cardMasks", masks),
		slog.String("balance", a.Balance.String()),
	)
}

// maskName keeps the first character and turns the rest into stars,
// so the field length is visible without leaking the actual name.
func maskName(name string) string {
	if name == "" {
		return ""
	}
	r := []rune(name)
	if len(r) <= 1 {
		return "*"
	}
	return string(r[0]) + strings.Repeat("*", len(r)-1)
}

// redactURL keeps the scheme and host of a webhook URL but masks the
// path so a tenant-specific secret in the path (which Mono tolerates)
// does not leak into logs.
func redactURL(u string) string {
	if u == "" {
		return ""
	}
	parsed, err := url.Parse(u)
	if err != nil || parsed == nil || parsed.Host == "" {
		return "***"
	}
	if parsed.Path == "" || parsed.Path == "/" {
		return parsed.Scheme + "://" + parsed.Host
	}
	return parsed.Scheme + "://" + parsed.Host + "/***"
}

// redactIBAN keeps the country code (first 2 chars) and the last 4
// digits — the same shape banks themselves use in user-facing UIs.
func redactIBAN(iban string) string {
	if iban == "" {
		return ""
	}
	if len(iban) <= 6 {
		return "***"
	}
	return iban[:2] + "***" + iban[len(iban)-4:]
}

// redactCardMask keeps the last 4 digits of a card mask. Mono's mask
// already replaces the middle digits, but the BIN (first 6) plus the
// last 4 still uniquely identify the issuer + a specific card to
// anyone with a small BIN table.
func redactCardMask(m string) string {
	if len(m) <= 4 {
		return "****"
	}
	return "****" + m[len(m)-4:]
}

// Account is a single client bank account. Balance and CreditLimit
// are typed [money.Money]; on the wire Mono sends them as bare int64
// minor units with the currency in a separate currencyCode field.
// A custom UnmarshalJSON transparently attaches the [currency.Code]
// to the amounts.
type Account struct {
	AccountID    string        `json:"id"`
	SendID       string        `json:"sendId"`
	Balance      money.Money   `json:"balance"`
	CreditLimit  money.Money   `json:"creditLimit"`
	Currency     currency.Code `json:"currencyCode"`
	CashbackType string        `json:"cashbackType"` // enum: None, UAH, Miles
	CardMasks    []string      `json:"maskedPan"`    // masked card numbers
	Type         CardType      `json:"type"`
	IBAN         string        `json:"iban"`
}

// UnmarshalJSON decodes Account and additionally sets Code on the
// money-typed fields from the adjacent Currency.
func (a *Account) UnmarshalJSON(data []byte) error {
	type raw Account
	var r raw
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	*a = Account(r)
	a.Balance.Code = a.Currency
	a.CreditLimit.Code = a.Currency
	return nil
}

// Jar is a "jar" account (a savings goal). Balance and Goal use the
// typed [money.Money] (see the wire-format note on [Account]).
type Jar struct {
	ID          string        `json:"id"`
	SendID      string        `json:"sendId"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	Currency    currency.Code `json:"currencyCode"`
	Balance     money.Money   `json:"balance"`
	Goal        money.Money   `json:"goal"`
}

// UnmarshalJSON decodes Jar and additionally sets Code on the
// money-typed fields from the adjacent Currency.
func (j *Jar) UnmarshalJSON(data []byte) error {
	type raw Jar
	var r raw
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	*j = Jar(r)
	j.Balance.Code = j.Currency
	j.Goal.Code = j.Currency
	return nil
}

// CardType is the visual / product type of a card tied to an account.
type CardType string

// Possible values of CardType. These are Mono product lines.
const (
	Black    CardType = "black"
	White    CardType = "white"
	Platinum CardType = "platinum"
	Iron     CardType = "iron"
	FOP      CardType = "fop" // FOP (card for sole proprietors)
	Yellow   CardType = "yellow"
	EAid     CardType = "eAid" // єПідтримка
	Diia     CardType = "diia" // Дія.Картка
)

// Accounts is a slice of [Account].
type Accounts []Account

// Jars is a slice of [Jar].
type Jars []Jar

// Transaction is one statement entry. The monetary fields are typed
// [money.Money]. Currency is the operation's currency (for
// OperationAmount). Amount is described as "in the account currency",
// but Mono does not return the account currency in the transaction
// body — Code on every money field is set from Currency. Keep this in
// mind for cross-currency operations (Amount in a currency different
// from Currency).
type Transaction struct {
	ID          string        `json:"id"`
	Time        epoch.Seconds `json:"time"`
	Description string        `json:"description"`
	MCC         int32         `json:"mcc"`
	OriginalMCC int32         `json:"originalMcc"`
	Hold        bool          `json:"hold"`
	// Amount is the value in the account currency.
	Amount money.Money `json:"amount"`
	// OperationAmount is the value in the transaction currency
	// (Currency) or after double conversion.
	OperationAmount money.Money `json:"operationAmount"`
	// Currency is the ISO 4217 numeric code of the transaction
	// currency.
	Currency       currency.Code `json:"currencyCode"`
	CommissionRate money.Money   `json:"commissionRate"`
	CashbackAmount money.Money   `json:"cashbackAmount"`
	Balance        money.Money   `json:"balance"`
	Comment        string        `json:"comment"`
	// Cash withdrawals only.
	ReceiptID string `json:"receiptId"`
	// FOP accounts only.
	InvoiceID string `json:"invoiceId"`
	// FOP accounts only.
	EDRPOU string `json:"counterEdrpou"`
	// FOP accounts only.
	IBAN string `json:"counterIban"`
}

// UnmarshalJSON decodes Transaction and attaches Currency to every
// money field.
func (t *Transaction) UnmarshalJSON(data []byte) error {
	type raw Transaction
	var r raw
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	*t = Transaction(r)
	c := t.Currency
	t.Amount.Code = c
	t.OperationAmount.Code = c
	t.CommissionRate.Code = c
	t.CashbackAmount.Code = c
	t.Balance.Code = c
	return nil
}

// Transactions is a slice of [Transaction].
type Transactions []Transaction

// validMCC reports whether an int32 is a syntactically valid ISO
// 18245 MCC (1..9999). Negative numbers (a wire glitch on a signed
// int32) and zero are rejected — there is no MCC 0.
func validMCC(n int32) bool { return n >= 1 && n <= 9999 }

// MCCCode returns the typed MCC of the transaction. Handy for
// chaining .Category() to group spending. Values outside the valid
// range 1..9999 (a wire glitch or a future field expansion) are
// reported as Code(0), which Category() folds into
// [mcc.CategoryUnknown].
func (t Transaction) MCCCode() mcc.Code {
	if !validMCC(t.MCC) {
		return mcc.Code(0)
	}
	return mcc.Code(t.MCC)
}

// OriginalMCCCode returns the MCC before Mono remapped it (for
// example, for cashback logic). Same validity check as [MCCCode].
func (t Transaction) OriginalMCCCode() mcc.Code {
	if !validMCC(t.OriginalMCC) {
		return mcc.Code(0)
	}
	return mcc.Code(t.OriginalMCC)
}
