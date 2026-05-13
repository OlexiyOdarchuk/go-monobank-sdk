# Як долучитися

Дякую за бажання покращити `go-monobank-sdk`. Цей документ — швидкий
гайд для тих, хто збирається відкрити issue або PR.

## Перед PR

1. **Відкрийте issue** з описом, що зламано / що пропонуєте додати,
   крім тривіальних правок (опечатка, форматування, оновлення лінка).
   Так уникнемо ситуації «зробив роботу — а її не приймуть».
2. **Узгодьте підхід у коментарях** до issue, якщо зміна нетривіальна
   (нові endpoint-и, зміна публічного API, нові залежності).

## Локальне середовище

```bash
git clone git@github.com:OlexiyOdarchuk/go-monobank-sdk.git
cd go-monobank-sdk
go mod download
make ci          # fmt-check + vet + test-race
```

Потрібен Go 1.23+. Для `otelmonobank` (окремий submodule) — Go 1.25+.

`make help` (або просто `make`) — список усіх dev-таргетів: `test`,
`test-race`, `cover`, `cover-html`, `lint`, `fmt`, `vet`, `bench`,
`fuzz`, `fuzz-all`, `tidy`, `integration`.

Опційно: `flake.nix` дає готове середовище через `nix develop` (Go,
golangci-lint, gh, jq).

## Стиль коду

- **Форматування:** `gofmt -w .` — CI падає на неформатованих файлах.
- **Лінт:** `golangci-lint run` — конфіг у `.golangci.yml` (29+ правил).
- **Race:** `go test -race ./...` — обов'язково перед PR.
- **Коментарі:** англійською в коді, українською — у godoc для
  публічних типів (як уже зроблено в існуючих пакетах).
- **Wire-ідентифікатори** (HTTP-заголовки, JSON-теги) — лишаються
  англійською.
- **Без `interface{}`/`any` без потреби.** Якщо тип відомий — типізуйте.

## Тести

- Кожен новий публічний метод — мінімум один happy-path тест і один
  error-path. Для HTTP-клієнтів використовуйте `monobanktest.NewServer`
  замість сирого `httptest.NewServer` — економить біла плита.
- Цільове покриття — **75%+ на patch** (codecov блокує PR нижче).
- Інтеграційні тести проти sandbox — позначайте build tag-ом
  `//go:build integration` і не запускайте у CI за замовчуванням.

## Коміти й PR

- Один логічний коміт = одна зміна. Refactor + feature + fix в одному
  коміті — на рев'ю розгортається в три.
- Заголовок коміту: imperative mood, без крапки в кінці, ≤ 70 символів.
  («Add rate limiter», не «Added rate limiter» чи «Adds rate limiter.»)
- В описі — *чому*, а не *що* (що видно з diff).
- PR-title — теж imperative, ≤ 70 символів. PR-body — за шаблоном.

## Нові endpoint-и

Якщо додаєте підтримку endpoint-у Mono:

1. Знайдіть його в офіційній документації — посиланням на `api.monobank.ua/docs`,
   `corp-api.monobank.ua` або `acquiring.html`.
2. Додайте посилання в godoc функції (як уже зроблено в існуючих
   методах — `// https://api.monobank.ua/docs/#tag/...`).
3. Покрийте mock-сервером через `monobanktest`.
4. Якщо endpoint змінює стан — додайте приклад у `examples/`.

## Безпека

Знайшли вразливість? **НЕ** відкривайте публічний issue.
Дивіться [`SECURITY.md`](SECURITY.md).

## Ліцензія

Вкладаючи код, ви погоджуєтеся, що він буде випущений під MIT-ліцензією
проєкту (див. [`LICENSE`](LICENSE)).
