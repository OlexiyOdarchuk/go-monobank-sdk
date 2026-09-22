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

Потрібен Go 1.26+ — для кореневого модуля і для `otelmonobank`
(окремий submodule).

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

### Інтеграційні тести

Ганяються окремо і кожен скіпається, якщо його змінних немає:

```bash
MONO_ACQUIRING_TOKEN=… go test -tags=integration -run Integration ./acquiring/...
```

| Пакет | Змінні |
|---|---|
| `bank` | — (публічні ендпоінти) |
| `acquiring` | `MONO_ACQUIRING_TOKEN`; опційно `MONO_T2P_EXTERNAL_PAYMENT_ID` |
| `installment` | `CHAST_STORE_ID`, `CHAST_SECRET`, `MONO_QR_ID`; `CHAST_BASE_URL` (дефолт — sandbox) |
| `business` | `MONO_BUSINESS_TOKEN`; опційно `MONO_BUSINESS_IBAN` |
| `openbanking` | `OB_CERT`, `OB_KEY`; опційно `OB_CA`, `OB_IBAN`, `OB_CONSENT_ID`, `OB_BASE_URL` |

Три виклики рухають гроші або видаляють дані, тож самих креденшалів
їм мало — потрібен ще явний опт-ін:

| Тест | Опт-ін | Що робить |
|---|---|---|
| `POSTransactionCancel` | `MONO_ALLOW_REFUND=yes-refund-real-money` + `MONO_POS_RRN`, `MONO_POS_AMOUNT` | справжнє повернення коштів |
| `DeleteImport` | `MONO_ALLOW_PAYSLIP_DELETE=yes-delete-payslips` + `MONO_BUSINESS_PAYSLIP_PERIOD` | видаляє імпорт розрахункових листів за період |
| `RequestTestCertificate` | `OB_ALLOW_CERT_REQUEST=yes-request-certificate` | подає заявку, яку обробляє людина |

Ці опт-іни **не** кладіть у секрети CI — запускайте руками, свідомо.

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
