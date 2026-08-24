module github.com/OlexiyOdarchuk/go-monobank-sdk/otelmonobank

go 1.25.0

require (
	github.com/OlexiyOdarchuk/go-monobank-sdk/v2 v2.0.0
	github.com/stretchr/testify v1.12.1
	go.opentelemetry.io/otel v1.45.0
	go.opentelemetry.io/otel/trace v1.45.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.1 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
)

// Локальний replace для розробки в одному репо. Споживачі цього
// sub-модуля його НЕ бачать (Go ігнорує replace у не-main модулях),
// тому require вище визначає реальну версію.
replace github.com/OlexiyOdarchuk/go-monobank-sdk/v2 => ../
