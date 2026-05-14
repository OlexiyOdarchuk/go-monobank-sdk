// Package bank — типи даних, які повертає Open API (спільні для personal
// та corporate клієнтів), плюс два неавторизовані публічні endpoint-и:
// курси валют (/bank/currency) та серверний ключ (/bank/sync).
//
// [Client] задовольняє інтерфейс webhook.KeyProvider, тож той самий
// інстанс можна використати і для рутинного отримання курсів, і як
// джерело ключів для webhook-handler-а:
//
//	keys := bank.New()
//	rates, _ := keys.Rates(ctx)
//	h, _    := webhook.NewHandler(ctx, webhook.Options{Keys: keys, ...})
//
// [ClientInfo], [Account], [Jar] і [Transaction] використовуються
// personal і corporate клієнтами однаково (кожен лише додає власний
// тип авторизації — форми відповідей ідентичні).
package bank

import (
	"encoding/json"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/currency"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/mcc"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/money"
	"github.com/vtopc/epoch"
)

// MaxStatementWindow — найбільший інтервал, який /personal/statement
// приймає за один виклик (31 доба). Для ширших діапазонів є
// TransactionsRange на personal/corporate клієнтах — він автоматично
// нарізає на 31-денні вікна.
const MaxStatementWindow = 31 * 24 * time.Hour

// ClientInfo — відповідь /personal/client-info: що банк знає про
// поточного клієнта (ім'я, рахунки, банки, поточна підписка на webhook).
type ClientInfo struct {
	ID         string   `json:"clientId"`
	Name       string   `json:"name"`
	WebHookURL string   `json:"webHookUrl"`
	Accounts   Accounts `json:"accounts"`
	Jars       Jars     `json:"jars"`
}

// Account — один банківський рахунок клієнта. Balance і CreditLimit —
// типізовані [money.Money]; на wire-рівні Mono шле їх як голі int64
// мінорних одиниць, а валюту — окремим полем currencyCode. Custom
// UnmarshalJSON прозоро прив'язує [currency.Code] до сум.
type Account struct {
	AccountID    string        `json:"id"`
	SendID       string        `json:"sendId"`
	Balance      money.Money   `json:"balance"`
	CreditLimit  money.Money   `json:"creditLimit"`
	Currency     currency.Code `json:"currencyCode"`
	CashbackType string        `json:"cashbackType"` // enum: None, UAH, Miles
	CardMasks    []string      `json:"maskedPan"`    // маски номерів карток
	Type         CardType      `json:"type"`
	IBAN         string        `json:"iban"`
}

// UnmarshalJSON декодує Account і додатково проставляє Code у
// money-полях зі сусіднього Currency.
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

// Jar — рахунок-«банка» (накопичення з ціллю). Balance і Goal — у
// типізованих [money.Money] (див. примітку про wire-формат у [Account]).
type Jar struct {
	ID          string        `json:"id"`
	SendID      string        `json:"sendId"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	Currency    currency.Code `json:"currencyCode"`
	Balance     money.Money   `json:"balance"`
	Goal        money.Money   `json:"goal"`
}

// UnmarshalJSON декодує Jar і додатково проставляє Code у money-полях
// зі сусіднього Currency.
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

// CardType — візуальний/продуктовий тип картки, прив'язаної до рахунку.
type CardType string

// Можливі значення CardType. Це продуктові лінійки Mono.
const (
	Black    CardType = "black"
	White    CardType = "white"
	Platinum CardType = "platinum"
	Iron     CardType = "iron"
	FOP      CardType = "fop" // ФОП (картка для фізичних осіб-підприємців)
	Yellow   CardType = "yellow"
	EAid     CardType = "eAid" // єПідтримка
	Diia     CardType = "diia" // Дія.Картка
)

// Accounts — slice типу [Account].
type Accounts []Account

// Jars — slice типу [Jar].
type Jars []Jar

// Transaction — один запис виписки. Грошові поля — типізовані
// [money.Money]. Currency — це валюта операції (для OperationAmount).
// Amount описаний як «у валюті рахунку», але Mono не повертає account
// currency у тілі транзакції — Code у всіх money-полях проставляється з
// Currency. Для крос-валютних операцій (Amount у валюті, відмінній від
// Currency) це треба тримати на увазі.
type Transaction struct {
	ID          string        `json:"id"`
	Time        epoch.Seconds `json:"time"`
	Description string        `json:"description"`
	MCC         int32         `json:"mcc"`
	OriginalMCC int32         `json:"originalMcc"`
	Hold        bool          `json:"hold"`
	// Amount — сума у валюті рахунку.
	Amount money.Money `json:"amount"`
	// OperationAmount — сума у валюті транзакції (Currency) або
	// після подвійної конверсії.
	OperationAmount money.Money `json:"operationAmount"`
	// Currency — ISO 4217 числовий код валюти транзакції.
	Currency       currency.Code `json:"currencyCode"`
	CommissionRate money.Money   `json:"commissionRate"`
	CashbackAmount money.Money   `json:"cashbackAmount"`
	Balance        money.Money   `json:"balance"`
	Comment        string        `json:"comment"`
	// Тільки для зняття готівки.
	ReceiptID string `json:"receiptId"`
	// Тільки для рахунків ФОП.
	InvoiceID string `json:"invoiceId"`
	// Тільки для рахунків ФОП.
	EDRPOU string `json:"counterEdrpou"`
	// Тільки для рахунків ФОП.
	IBAN string `json:"counterIban"`
}

// UnmarshalJSON декодує Transaction і прив'язує Currency до всіх
// money-полів.
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

// Transactions — slice типу [Transaction].
type Transactions []Transaction

// MCCCode повертає типізований MCC транзакції — зручно одразу викликати
// .Category() для групування витрат.
func (t Transaction) MCCCode() mcc.Code { return mcc.Code(t.MCC) }

// OriginalMCCCode — MCC до того, як Mono перемапила його (наприклад, для
// логіки cashback). Якщо MCC і OriginalMCC різні — є сенс зважити на
// «оригінал» при категоризації витрат.
func (t Transaction) OriginalMCCCode() mcc.Code { return mcc.Code(t.OriginalMCC) }
