package jar_test

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/jar"
)

func ExampleClient_ByLongID() {
	cli, err := jar.New()
	if err != nil {
		log.Fatal(err)
	}

	info, err := cli.ByLongID(context.Background(), "5fX1aB7Y8z")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s — %d/%d UAH\n", info.Title, info.Amount, info.Goal)
}

// ByShortID returns short jar info plus a LongJarID — use ByLongID
// for subsequent reads because send.monobank.ua has stricter limits.
func ExampleClient_ByShortID() {
	cli, err := jar.New()
	if err != nil {
		log.Fatal(err)
	}

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
