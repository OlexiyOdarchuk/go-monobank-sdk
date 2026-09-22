package openbanking_test

import (
	"context"
	"fmt"
	"log"
	"time"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk/v2"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/openbanking"
)

func ExampleNew() {
	// Authentication is the TLS handshake: without a valid QWAC the
	// bank drops the connection before any HTTP is exchanged.
	httpCli, err := openbanking.NewMTLSHTTPClientFromFiles("qwac.pem", "qwac.key", "")
	if err != nil {
		log.Fatal(err)
	}
	cli := openbanking.New(
		monobank.WithBaseURL(openbanking.BaseURLStage),
		monobank.WithHTTPClient(httpCli),
	)
	defer cli.Close()

	st, err := cli.ConsentStatus(context.Background(), "2409105jHHToBoHe3Lqw")
	if err != nil {
		// Not log.Fatal: os.Exit would skip the deferred Close.
		fmt.Println("consent status:", err)
		return
	}
	fmt.Println(st.ConsentStatus)
}

func ExampleClient_CreateConsent() {
	var cli *openbanking.Client // see [openbanking.New]

	out, err := cli.CreateConsent(context.Background(), &openbanking.CreateConsentRequest{
		Access: openbanking.AccountAccess{
			Cards: []openbanking.AccountAccessRights{{
				Account: openbanking.Account{
					IBAN:     "UA413220010000026206347644036",
					Currency: "UAH",
				},
				Rights: []openbanking.AccessRight{
					openbanking.AccessAccountDetails,
					openbanking.AccessBalances,
					openbanking.AccessTransactions,
				},
			}},
		},
		ConsentType:        openbanking.ConsentDetailed,
		RecurringIndicator: true,
		ValidTo:            openbanking.NewDate(time.Now().AddDate(0, 6, 0)),
		FrequencyPerDay:    4,
	}, openbanking.InitiationOptions{
		PSU: openbanking.PSU{
			ID:        "UA413220010000026206347644036",
			IDType:    openbanking.PSUIDTypeIBAN,
			IPAddress: "8.8.8.8", // the customer's address, not the server's
		},
		TPPRedirectURI: "https://yourapp.example.com/consent-done",
	})
	if err != nil {
		log.Fatal(err)
	}

	// The consent is [openbanking.ConsentReceived] until the customer
	// authorises it.
	auth, err := cli.StartConsentAuthorisation(context.Background(), out.ConsentID)
	if err != nil {
		log.Fatal(err)
	}
	if auth.Links.SCARedirect != nil {
		fmt.Println("send the customer to", auth.Links.SCARedirect.Href)
	}
}

func ExampleClient_Transactions() {
	var cli *openbanking.Client // see [openbanking.New]

	opts := openbanking.TransactionsOptions{
		AccountOptions: openbanking.AccountOptions{ConsentID: "2409105jHHToBoHe3Lqw"},
		BookingStatus:  openbanking.BookingBooked,
		DateFrom:       openbanking.NewDate(time.Now().AddDate(0, -1, 0)),
	}
	for {
		page, err := cli.Transactions(context.Background(), "hH1eKFyoP", opts)
		if err != nil {
			log.Fatal(err)
		}
		if page.Transactions != nil {
			for _, tx := range page.Transactions.Booked {
				fmt.Println(tx.BookingDate, tx.TransactionAmount.Value, tx.TransactionAmount.Currency)
			}
		}
		next, ok := openbanking.NextPageID(page)
		if !ok {
			break
		}
		opts.PageID = next
	}
}

func ExampleClient_CreatePayment() {
	var cli *openbanking.Client // see [openbanking.New]

	pay, err := cli.CreatePayment(context.Background(), openbanking.CreditTransfers,
		&openbanking.CreatePaymentRequest{
			InstructedAmount: openbanking.Amount{Currency: "UAH", Value: "100.02"},
			CreditorAccount: openbanking.PaymentAccountReference{
				IBAN:     "UA173220010000026002700000121",
				Currency: "UAH",
			},
			Creditor: openbanking.Creditor{
				Name:           `ТОВ "Ромашка"`,
				CreditorID:     "12345678",
				CreditorIDType: openbanking.CreditorUSRC,
			},
			DebtorAccount: openbanking.PaymentAccountReference{
				IBAN:     "UA413220010000026206347644036",
				Currency: "UAH",
			},
			RemittanceInformationUnstructured: []string{"Оплата за послуги згідно рахунку 314415"},
		},
		openbanking.InitiationOptions{
			PSU:            openbanking.PSU{IPAddress: "8.8.8.8"},
			TPPRedirectURI: "https://yourapp.example.com/payment-done",
		})
	if err != nil {
		log.Fatal(err)
	}

	// Nothing has moved yet: the payment needs SCA.
	auth, err := cli.StartPaymentAuthorisation(context.Background(), openbanking.CreditTransfers,
		pay.PaymentID, openbanking.AuthorisationOptions{})
	if err != nil {
		log.Fatal(err)
	}
	if auth.Links.SCARedirect == nil {
		// DECOUPLED: the customer confirms in their own banking app,
		// so poll instead of redirecting.
		fmt.Println("waiting for in-app confirmation")
	}
}

func ExampleMessages() {
	var cli *openbanking.Client // see [openbanking.New]

	_, err := cli.Balances(context.Background(), "hH1eKFyoP",
		openbanking.AccountOptions{ConsentID: "2409105jHHToBoHe3Lqw"})
	for _, msg := range openbanking.Messages(err) {
		if msg.Code == openbanking.CodeCurrencyMismatch {
			fmt.Println("currency mismatch:", msg.Text)
		}
	}
}
