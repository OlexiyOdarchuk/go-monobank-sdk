// Command installment demonstrates monobank's «Pay-in-Parts»
// installment API against the sandbox: client validation → order
// creation → state polling → confirm-on-handover → final state.
//
// Credentials are REQUIRED via env (no hardcoded defaults — leaking
// example credentials into a real repo is a recurring source of
// abuse reports against the bank's sandbox):
//
//	CHAST_STORE_ID  — sandbox storeID (request from monobank)
//	CHAST_SECRET    — sandbox secret
//	CHAST_BASE_URL  — optional, defaults to installment.BaseURLSandbox
//	CHAST_PHONE     — optional, defaults to +380501234564
//
// The phone-suffix dictates the sandbox scenario:
//   - ...1 → success in ~5s
//   - ...2 → permanent client-wait (no callback)
//   - ...3 → fail (insufficient limit)
//   - ...4 → success, awaits store confirmation (the default here)
//
// Usage:
//
//	CHAST_STORE_ID=xxx CHAST_SECRET=yyy go run ./examples/installment
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/installment"
)

func main() {
	storeID := os.Getenv("CHAST_STORE_ID")
	secret := os.Getenv("CHAST_SECRET")
	if storeID == "" || secret == "" {
		log.Fatal("CHAST_STORE_ID and CHAST_SECRET must be set — request " +
			"sandbox credentials from monobank support (api@monobank.ua) " +
			"and export them in your shell before running this example.")
	}
	baseURL := envOr("CHAST_BASE_URL", installment.BaseURLSandbox)
	phone := envOr("CHAST_PHONE", "+380501234564") // ...4 = WAITING_FOR_STORE_CONFIRM
	storeOrderID := fmt.Sprintf("demo-%d", time.Now().Unix())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cli, err := installment.New(storeID, secret, installment.WithBaseURL(baseURL))
	if err != nil {
		log.Fatalf("installment.New: %v", err)
	}

	// 1. Швидка перевірка, що клієнт є у monobank.
	found, err := cli.ValidateClient(ctx, phone)
	if err != nil {
		log.Fatalf("ValidateClient: %v", err)
	}
	fmt.Printf("Validate %s → found=%v\n", phone, found)
	if !found {
		log.Fatal("client not found — спробуй інший номер")
	}

	// 2. Створюємо заявку.
	order, err := cli.CreateOrder(ctx, &installment.CreateOrderRequest{
		StoreOrderID: storeOrderID,
		ClientPhone:  phone,
		TotalSum:     2499.99,
		Invoice: installment.CreateOrderInvoice{
			Number: "INV-" + storeOrderID,
			Date:   time.Now().Format("2006-01-02"),
			Source: installment.SourceInternet,
		},
		AvailablePrograms: []installment.Program{{AvailablePartsCount: []int{3, 6, 10}}},
		Products: []installment.Product{
			{Name: "Кит-набір для розробника", Count: 1, Sum: 2499.99},
		},
	})
	if err != nil {
		log.Fatalf("CreateOrder: %v", err)
	}
	fmt.Printf("CreateOrder → order_id=%s\n", order.OrderID)

	// 3. Поллимо стан, поки не SUCCESS, FAIL або до WAITING_FOR_STORE_CONFIRM.
	state, err := waitForTerminalOrConfirm(ctx, cli, order.OrderID)
	if err != nil {
		log.Fatalf("waitForTerminalOrConfirm: %v", err)
	}
	fmt.Printf("State → %s/%s — %s\n", state.State, state.OrderSubState, state.Message)

	// 4. Якщо банк просить підтвердити — підтверджуємо (товар видано).
	if state.State == installment.StateInProcess && state.OrderSubState == installment.SubWaitingForStoreConfirm {
		confirmed, err := cli.ConfirmOrder(ctx, order.OrderID)
		if err != nil {
			log.Fatalf("ConfirmOrder: %v", err)
		}
		fmt.Printf("Confirmed → %s/%s\n", confirmed.State, confirmed.OrderSubState)
	}
}

func waitForTerminalOrConfirm(ctx context.Context, cli *installment.Client, orderID string) (*installment.OrderStateInfo, error) {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-tick.C:
		}
		st, err := cli.OrderState(ctx, orderID)
		if err != nil {
			var apiErr *installment.APIError
			if errors.As(err, &apiErr) {
				return nil, fmt.Errorf("api error: %w", apiErr)
			}
			return nil, err
		}
		fmt.Printf("  %s/%s\n", st.State, st.OrderSubState)
		switch st.State {
		case installment.StateSuccess, installment.StateFail:
			return st, nil
		case installment.StateInProcess:
			if st.OrderSubState == installment.SubWaitingForStoreConfirm {
				return st, nil
			}
		}
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
