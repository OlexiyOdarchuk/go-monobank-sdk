// Command jar демонструє публічний lookup банок (jars) monobank: повна
// інформація по longJarId (з URL віджета банки) та резолв коротких
// share-посилань send.monobank.ua/<clientId> у longJarId.
//
// Usage:
//
//	# знаючи longJarId з URL віджета:
//	LONG_JAR_ID=2zQL6sqnKgTYi7e69271YYWKTXTfMK8g go run ./examples/jar
//
//	# або знаючи лише short clientId з send.monobank.ua/<id>:
//	JAR_CLIENT_ID=3zs3UByNna go run ./examples/jar
//
// Обидва endpoint-и публічні (без авторизації) і read-only. Ліміти на
// send.monobank.ua суворіші — для регулярних оновлень балансу кешуй
// longJarId і ходи лише в ByLongID.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/jar"
)

func main() {
	longID := os.Getenv("LONG_JAR_ID")
	shortID := os.Getenv("JAR_CLIENT_ID")
	if longID == "" && shortID == "" {
		log.Fatal("set LONG_JAR_ID and/or JAR_CLIENT_ID env var")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cli := jar.New()

	if shortID != "" {
		fmt.Println("## ByShortID (send.monobank.ua/api/handler)")
		short, err := cli.ByShortID(ctx, shortID)
		switch {
		case errors.Is(err, jar.ErrNotFound):
			log.Fatalf("jar with clientId=%q not found", shortID)
		case err != nil:
			log.Fatalf("ByShortID: %v", err)
		}
		fmt.Printf("  name:        %s\n", short.Name)
		fmt.Printf("  description: %s\n", short.Description)
		fmt.Printf("  amount/goal: %d / %d\n", short.JarAmount, short.JarGoal)
		fmt.Printf("  status:      %s (trusted=%v)\n", short.JarStatus, short.IsTrusted)
		fmt.Printf("  longJarId:   %s\n", short.LongJarID)
		// Резолвимо у longID, щоб далі показати ByLongID.
		if longID == "" && short.LongJarID != "" {
			longID = short.LongJarID
		}
		fmt.Println()
	}

	if longID == "" {
		return
	}

	fmt.Println("## ByLongID (/bank/jar/{longJarId})")
	info, err := cli.ByLongID(ctx, longID)
	switch {
	case errors.Is(err, jar.ErrNotFound):
		log.Fatalf("jar %q not found", longID)
	case err != nil:
		log.Fatalf("ByLongID: %v", err)
	}
	fmt.Printf("  jarId:     %s\n", info.JarID)
	fmt.Printf("  title:     %s\n", info.Title)
	fmt.Printf("  owner:     %s\n", info.OwnerName)
	fmt.Printf("  amount:    %d (minor units of ccy %d)\n", info.Amount, info.Currency)
	fmt.Printf("  goal:      %d\n", info.Goal)
	if info.Goal > 0 {
		fmt.Printf("  progress:  %.1f%%\n", float64(info.Amount)/float64(info.Goal)*100)
	}
}
