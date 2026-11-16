package corporate_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/auth"
	"github.com/OlexiyOdarchuk/go-monobank-sdk/v2/corporate"
)

func ExampleNew() {
	pem, _ := os.ReadFile("priv.pem")
	maker, err := auth.NewCorpAuthMaker(pem)
	if err != nil {
		log.Fatal(err)
	}

	cli, err := corporate.New(maker)
	if err != nil {
		log.Fatal(err)
	}
	_ = cli
}

func ExampleClient_Auth() {
	pem, _ := os.ReadFile("priv.pem")
	maker, _ := auth.NewCorpAuthMaker(pem)
	cli, _ := corporate.New(maker)

	// Ask the bank to start a client-data access flow. The client opens
	// AcceptURL in mono's app and approves; meanwhile we poll CheckAuth.
	// An empty callback URL means no X-Callback: polling only.
	tok, err := cli.Auth(context.Background(), "", auth.PermSt, auth.PermPI)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Show this URL/QR to the client:", tok.AcceptURL)

	for {
		err := cli.CheckAuth(context.Background(), tok.RequestID)
		if err == nil {
			break // approved
		}
		if corporate.IsAuthPending(err) { // 401: not approved yet
			time.Sleep(5 * time.Second)
			continue
		}
		log.Fatal(err)
	}

	info, _ := cli.ClientInfo(context.Background(), tok.RequestID)
	fmt.Println("Client:", info.Name)
}

func ExampleClient_SignatureCreate() {
	pem, _ := os.ReadFile("priv.pem")
	maker, _ := auth.NewCorpAuthMaker(pem)
	cli, _ := corporate.New(maker)

	// hash is the GOST 34.311-95 digest of the document body, in hex.
	resp, err := cli.SignatureCreate(context.Background(), &corporate.SignatureCreateRequest{
		Documents: []corporate.Document{{
			Name: "Договір на поставку товарів",
			Hash: "A421FD4D4AB19BE76EC02A0F84AC2379822943FE85EB6ED7F22B30F73CB9CAF9",
			Type: "pdf",
			Link: "https://yourapp.example.com/agreement.pdf",
		}},
		CallbackURL: "https://yourapp.example.com/sign-cb",
	})
	if err != nil {
		log.Fatal(err)
	}
	// resp.Deeplink opens the signing flow in mono's mobile app.
	fmt.Println("Sign here:", resp.Deeplink)
	fmt.Println("Poll status with:", resp.RequestID)
}
