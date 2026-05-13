package jar_test

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/jar"
)

func ExampleClient_ByLongID() {
	cli := jar.New()

	info, err := cli.ByLongID(context.Background(), "5fX1aB7Y8z")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s — %d/%d UAH\n", info.Title, info.Amount, info.Goal)
}

// ByShortID повертає коротку інформацію разом із LongJarID — подальші
// читання роби через ByLongID, бо ліміти на send.monobank.ua суворіші.
func ExampleClient_ByShortID() {
	cli := jar.New()

	short, err := cli.ByShortID(context.Background(), "abc123")
	if err != nil {
		if errors.Is(err, jar.ErrNotFound) {
			fmt.Println("банка не існує")
			return
		}
		log.Fatal(err)
	}
	fmt.Printf("longJarId=%s\n", short.LongJarID)
}
