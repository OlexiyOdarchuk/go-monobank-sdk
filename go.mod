module github.com/OlexiyOdarchuk/go-monobank-sdk

go 1.23

// v1.1.0 транзитивно тягнув golang.org/x/sync v0.20.0, що вимагає Go
// 1.25.0 — модуль не збирався на CI-матриці 1.23/1.24, заявлених як
// підтримувані. v1.1.1 фіксує депенденсі, але README/документація
// лишалися застарілими (зламані приклади Ccy/NewKeyedLimiter,
// відсутні нові фічі). v1.1.2 — перша версія з повністю узгодженою
// документацією.
retract (
	v1.1.0
	v1.1.1
)

require (
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.1
	github.com/stretchr/testify v1.11.1
	github.com/vtopc/epoch v1.6.0
	golang.org/x/sync v0.10.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
