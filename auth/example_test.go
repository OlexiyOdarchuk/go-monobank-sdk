package auth_test

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/auth"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/corporate"
)

func ExampleCorpAuthMaker_PublicKeyPEM() {
	// Перший раз: подати компанію на схвалення банком.
	// Той самий приватний ключ, що далі використовуватиметься для
	// підпису всіх запитів, відомий тільки тобі — у банк іде його
	// pubkey.

	privPEM, err := os.ReadFile("priv.pem")
	if err != nil {
		log.Fatal(err)
	}
	maker, err := auth.NewCorpAuthMaker(privPEM)
	if err != nil {
		log.Fatal(err)
	}

	pubPEM, err := maker.PublicKeyPEM()
	if err != nil {
		log.Fatal(err)
	}

	cli, err := corporate.New(maker)
	if err != nil {
		log.Fatal(err)
	}

	logo, _ := os.ReadFile("logo.png")
	resp, err := cli.Register(context.Background(), &corporate.RegistrationRequest{
		Pubkey:        pubPEM,
		Name:          `ТОВ "Acme"`,
		Description:   "PFM для співробітників",
		ContactPerson: "Олексій Одарчук",
		Phone:         "380671234567",
		Email:         "ops@example.com",
		Logo:          logo,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Registration status:", resp.Status)

	// Згодом — періодично перевіряти схвалення:
	st, _ := cli.RegistrationStatus(context.Background(), pubPEM)
	fmt.Println("Current state:", st.Status, "KeyID:", st.KeyID)
}
