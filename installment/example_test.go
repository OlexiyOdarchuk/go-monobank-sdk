package installment_test

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/installment"
)

func ExampleNew() {
	cli, err := installment.New(
		os.Getenv("CHAST_STORE_ID"),
		os.Getenv("CHAST_SECRET"),
	)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := cli.CreateOrder(context.Background(), &installment.CreateOrderRequest{
		// Fill in OrderID, Amount, Items, ClientPhone etc. — see godoc.
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("orderId=%s\n", resp.OrderID)

	// Poll until a terminal state — see examples/installment.
	state, err := cli.OrderState(context.Background(), resp.OrderID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("state=%s\n", state.State)
}

// VerifyCallback validates the HMAC-SHA256 signature on incoming
// installment callbacks. Call it before parsing the body; on error,
// respond with 401, not 200.
func ExampleClient_VerifyCallback() {
	cli, err := installment.New(
		os.Getenv("CHAST_STORE_ID"),
		os.Getenv("CHAST_SECRET"),
	)
	if err != nil {
		log.Fatal(err)
	}

	body := []byte(`{"orderId":"o-1","orderState":"FINISHED"}`)
	signature := "<X-Sign-Header value>"

	if err := cli.VerifyCallback(body, signature); err != nil {
		log.Fatal("invalid signature: ", err)
	}
	// Now the body can be trusted and parsed.
}
