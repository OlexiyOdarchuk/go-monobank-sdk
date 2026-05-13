package installment_test

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/installment"
)

func ExampleNew() {
	cli := installment.New(
		os.Getenv("CHAST_STORE_ID"),
		os.Getenv("CHAST_SECRET"),
	)

	resp, err := cli.CreateOrder(context.Background(), &installment.CreateOrderRequest{
		// Заповніть OrderID, Amount, Items, ClientPhone тощо — див. godoc.
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("orderId=%s\n", resp.OrderID)

	// Поллінг до фінального стану — див. examples/installment.
	state, err := cli.OrderState(context.Background(), resp.OrderID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("state=%s\n", state.State)
}

// VerifyCallback перевіряє HMAC-SHA256 підпис callback-ів від ПЧ.
// Виклич перед розбором тіла; на помилку — поверни 401, не 200.
func ExampleClient_VerifyCallback() {
	cli := installment.New(
		os.Getenv("CHAST_STORE_ID"),
		os.Getenv("CHAST_SECRET"),
	)

	body := []byte(`{"orderId":"o-1","orderState":"FINISHED"}`)
	signature := "<X-Sign-Header value>"

	if err := cli.VerifyCallback(body, signature); err != nil {
		log.Fatal("invalid signature: ", err)
	}
	// Тепер тіло можна довіряти й парсити.
}
