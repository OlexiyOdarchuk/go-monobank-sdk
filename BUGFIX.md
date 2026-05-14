# BUGFIX — go-monobank-sdk

Бек-лог знахідок з повного code review. Формат: чекбокси `[ ]` / `[x]`.
Кожен пункт — самодостатній: `path:line` → опис → що зробити.

Порядок виправлення (рекомендований):
1. усі **CRITICAL**;
2. безпека (HIGH/SECURITY);
3. дані (HIGH/DATA);
4. решта HIGH;
5. MEDIUM;
6. LOW/NIT — як буде час.

Агент: після виправлення став `[x]` й коротко (одна стрічка) дописуй у `Resolution:` посилання на коміт або суть фікса.

---

## CRITICAL

- [x] **webhook/signature.go:70-72** — `asn1.Unmarshal(sig, &asn1Sig)` ігнорує `rest`, дозволяючи trailing-байти → ECDSA signature malleability (для одного payload існує безліч валідних X-Sign).
  - Виправити: перевіряти `len(rest) == 0` або перевести на `ecdsa.VerifyASN1`.
  - Покрити юніт-тестом: «valid DER + 1 zero byte → ErrBadSignature».
  - Resolution: переписав на єдиний `ecdsa.VerifyASN1` (його ReadASN1 + Empty() відсікає trailing-bytes). Raw r||s ремаршалиться у DER. Додано regression `TestVerify/valid_DER_with_trailing_zero_byte_is_rejected`.

- [x] **webhook/signature.go:63** — deprecated `ecdsa.Verify(pub, digest, r, s)`.
  - Виправити: для raw r||s ремаршалити у DER і викликати `ecdsa.VerifyASN1` (одна гілка коду).
  - Resolution: єдина гілка через `ecdsa.VerifyASN1`; raw r||s конвертується у DER через `marshalRawSigToDER`.

- [ ] **otelmonobank/store.go:17 + otel.go:84-94** — `map[*http.Request]trace.Span` тече при `resp == nil` (transport error: `End()` ніколи не викликається) і при retry (повторний request hook перетирає span попередньої спроби).
  - Виправити: тримати span у `r.Context()` через `context.WithValue` АБО зберігати стек на ключ із обов'язковим `End()` попереднього перед push нового; у response-hook завжди викликати `End()`, навіть якщо `resp == nil` (брати помилку з аргумента).
  - Додати regression-тест із транспортом, що повертає `(nil, error)`, перевірити, що `len(store) == 0`.
  - Resolution:

- [x] **personal/client.go:101-112, corporate/client_data.go:60-72, personal/iter.go:53, corporate/iter.go:47** — пагінатор робить `cursor = end` без зсуву на +1с. Mono `[from,to]` інклюзивний → транзакції на стиках вікон з'являться двічі (а у streaming-ітераторі — без помітної ознаки).
  - Виправити: `cursor = end.Add(time.Second)` АБО дедуп через `seen map[string]struct{}` на ID транзакцій (memory-safe — освіжати на кожному вікні).
  - Тест: вікно з транзакціями рівно на boundary секунді — не повинно бути дублів.
  - Resolution: cursor зсувається на +1s після кожного вікна в personal/corporate (slice + iter). Loop guard `!cursor.After(to)` замість `cursor.Before(to)` коректно завершує. Regression `TestTransactionsRange_noBoundaryDuplicate` фіксує: `from` кожного запиту унікальний (немає overlap).

- [x] **business/iter.go:57-94** — `oldest.Add(-time.Second)` як курсор: якщо в одній секунді операцій більше за `pageSize` — рештa губиться (silent data loss).
  - Виправити: ID-based курсор АБО не зсувати на секунду коли `len(page) == limit` і всі мають однаковий `time`.
  - Тест: фейковий API, що повертає 50 операцій із однією і тією ж секундою — переконатися, що ітератор бачить усі.
  - Resolution: same-second overflow detection: cursorTo не зсувається на -1s коли всі items на одній секунді і page заповнений; seen-map дедуплікує IDs цієї секунди й чиститься коли cursorTo переходить на нову секунду; loop-guard зупиняє ітерацію якщо API більше нічого не дає. Regression `TestStatementAll_sameSecondOverflow_yieldsAll` + `TestStatementAll_progressOnPartialPage`.

- [x] **business/statement.go:23-26** — `from.Unix()` без перевірки `from.IsZero()`: zero `time.Time{}` дає `-6795364578` у URL.
  - Виправити: повертати `ErrInvalidTimeRange` (нова sentinel-помилка), якщо `from.IsZero() || to.IsZero() || !to.After(from)`.
  - Resolution: новий sentinel `business.ErrInvalidTimeRange`; `Statement` відхиляє `from.IsZero() || from.Unix() < 0` і `!to.IsZero() && !to.After(from)`.

- [x] **installment/orders.go:33-66, 90-123** — `OrderState/Confirm/Reject/OrderInfo/OrderData/CheckPaid` не валідують порожній `orderID`.
  - Виправити: на початку кожної функції `if orderID == "" { return nil, ErrEmptyOrderID }`.
  - Resolution: новий sentinel `ErrEmptyOrderID`, валідація у кожній з 6 функцій. Regression `TestOrderID_emptyRejectedLocally` (table-test з усіма ендпоінтами).

- [x] **installment/report.go:12-19** — `DailyReport` без валідації порожнього `date`.
  - Виправити: повернути `ErrInvalidDate` (новий sentinel), якщо date.IsZero() / format empty.
  - Resolution: новий sentinel `ErrEmptyDate`; валідація додана. Regression `TestDailyReport_emptyDateRejected`.

- [x] **installment/validate.go:15-35** — `ValidateClient`/`ValidateClientLegacy` без перевірки phone.
  - Виправити: `if phone == "" { return ... ErrEmptyPhone }`; додати простий шаблон-чек (починається з `+`, лише цифри).
  - Resolution: новий `validatePhone()` хелпер — `ErrEmptyPhone` / `ErrInvalidPhone`; перевіряє `phone[0]=='+'` + лише цифри. Regression `TestValidatePhone_rejectsMalformed`.

- [x] **installment/client.go:97-108** — `New("", "", ...)` мовчки приймає порожні `storeID`/`secret`.
  - Виправити: повернути `(*Client, error)`; помилка при порожніх обов'язкових параметрах.
  - Resolution: BREAKING — `installment.New` тепер `(*Client, error)`; sentinel `ErrEmptyStoreID`, `ErrEmptySecret`; також відхиляє `http://` для non-loopback (`ErrInsecureBaseURL`) з opt-out через `WithInsecureBaseURL`. Regression: `TestNew_rejectsEmptyCredentials`, `TestNew_rejectsInsecureBaseURL`, `TestNew_allowsLoopbackHTTP`, `TestNew_insecureOptOut`.

- [x] **acquiring/webhook.go:89-102** — `VerifyWebhook` не має replay-захисту.
  - Виправити: явно задокументувати, що caller ОБОВ'ЯЗКОВО має робити persistent dedup за `(invoiceId, modifiedDate)`. Додати опціональний helper `VerifyWebhookFresh(pub, body, xSign, maxAge time.Duration)`, який парсить `modifiedDate` із payload і відкидає старіші за `maxAge`.
  - Resolution: docstring `VerifyWebhook` поповнено IMPORTANT-блоком про persistent dedup. Новий хелпер `VerifyWebhookFresh(pub, body, xSign, maxAge)` що (1) перевіряє підпис, (2) парсить `modifiedDate` (RFC3339Nano/RFC3339/millis-Z/тощо), (3) відкидає старіші за `maxAge`. Sentinel-помилки `ErrWebhookStale`, `ErrWebhookNoTimestamp`. Regression-тести `TestVerifyWebhookFresh` (6 sub-cases).

- [x] **examples/installment/main.go:33-34** — hardcoded дефолтні sandbox-credentials (`test_store_with_confirm`, `secret_98765432...`).
  - Виправити: прибрати fallback; якщо env пустий — `log.Fatal` з інструкцією, як отримати ключі.
  - Resolution: видалено fallback; `log.Fatal` з інструкцією звертатися до api@monobank.ua за credentials, якщо `CHAST_STORE_ID`/`CHAST_SECRET` порожні.

---

## HIGH — Security / Network

- [ ] **options.go:160-173** — `isInsecureBaseURL` хост-чек охоплює тільки `localhost/127.0.0.1/::1` (пропускає `127.0.0.2`, link-local).
  - Виправити: `ip := net.ParseIP(host); ip != nil && ip.IsLoopback()` АБО literal `localhost`.
  - Resolution:

- [ ] **options.go:152-156** — `WithInsecureBaseURL` мусить бути ДО `WithBaseURL`, інакше bypass не спрацьовує (порядок-залежна семантика).
  - Виправити: два проходи опцій — спершу `allowInsecureBaseURL`, потім решта; або відкладати валідацію `baseURL` на момент першого Do.
  - Resolution:

- [ ] **jar/jar.go:132-141** — `WithAPIBaseURL`/`WithSendBaseURL` без перевірки схеми → SSRF.
  - Виправити: ту саму insecure-baseURL логіку, що в `monobank.WithBaseURL`.
  - Resolution:

- [x] **installment/client.go:42** — `MaxResponseBytes = 50<<20` для всіх відповідей; скомпрометований proxy може дути 50 MiB у JSON.
  - Виправити: окремий ліміт для JSON (1 MiB) і PDF (50 MiB); вибирати за endpoint-ом.
  - Resolution: нові константи `MaxJSONResponseBytes = 1 MiB`, `MaxPDFResponseBytes = 50 MiB`; `doJSON` обмежений JSON-cap-ом, `doPDF` — PDF-cap-ом. `MaxResponseBytes` залишений як deprecated alias на 50 MiB.

- [ ] **corporate/auth.go:33** — `X-Callback` ставиться сирим, без валідації scheme.
  - Виправити: парсити `url.Parse`, відхиляти не-https (з опт-аутом, як у `WithInsecureBaseURL`).
  - Resolution:

- [ ] **auth/corporate.go:191** — `timestamp = time.Now().Unix()` без захисту від clock-skew.
  - Виправити: задокументувати у godoc; опційно — порівняти з останнім `Date` response-заголовком і warn-логнути drift > N сек.
  - Resolution:

- [ ] **auth/corporate.go:218** — deprecated `ecdsa.Sign` + ручний ASN.1 marshal.
  - Виправити: перейти на `ecdsa.SignASN1` (один виклик повертає DER).
  - Resolution:

- [ ] **auth/personal.go:20** — `Personal.SetAuth` ставить X-Token навіть якщо `token == ""`.
  - Виправити: повернути error при порожньому токені у `NewPersonal` АБО у `SetAuth`.
  - Resolution:

- [ ] **personal/client.go:55-64, corporate/client_data.go:19-29** — `ClientInfo` повертає ПІБ/IBAN/маски карток; SDK логує url-шлях з accountID на Debug.
  - Виправити: реалізувати `slog.LogValuer` на `bank.ClientInfo`, маскуючи IBAN/маски/ПІБ у логах.
  - Resolution:

- [ ] **client.go:397** — `json.NewDecoder(resp.Body).Decode(v)` не вичерпує тіло → connection re-use страждає.
  - Виправити: після Decode додати `io.Copy(io.Discard, resp.Body)`.
  - Resolution:

---

## HIGH — Correctness / Data

- [ ] **retry.go:87-113** — full-jitter дає `delay = 0` навіть без `Retry-After: 0`; миттєвий ретрай після 5xx.
  - Виправити: equal jitter `d/2 + rand.Int64N(d/2)`. Мінімальний floor: 50ms.
  - Resolution:

- [ ] **money/money.go:44-65** — `Add/Sub/Mul(n int64)` без overflow guard.
  - Виправити: `math/bits.Add64` / `math.MulOverflow`-стиль; повертати error або (Money, ok).
  - Тест: `MaxInt64 + 1`, `MinInt64 - 1`, `MaxInt64 * 2`.
  - Resolution:

- [ ] **money/money.go:70-76** — `Scale(float64)` втрачає точність для minor > 2^53.
  - Виправити: задокументувати ліміт у godoc; для критичних шляхів — рекомендувати ручне множення.
  - Resolution:

- [ ] **business/types.go:38,52** — `int64(math.Round(a.Balance*100))` припускає 2 знаки; для JPY (0), BHD/JOD/KWD (3) — невірно.
  - Виправити: підтягувати `decimals` із `currency` пакету (через `currency.Decimals(code int) int`).
  - Resolution:

- [ ] **acquiring/types.go:260** — `TipsInfo.Amount int` (на 32-бітних overflow).
  - Виправити: змінити на `int64`.
  - Resolution:

- [ ] **bank/currency.go:57-95** — `Convert` плутає `RateBuy/RateSell` для напрямку.
  - Виправити: написати explicit table-test із 4 кейсами (UAH→USD buy, UAH→USD sell, USD→UAH buy, USD→UAH sell) із верифікацією на реальних котируваннях Mono; виправити логіку до проходження.
  - Resolution:

- [ ] **mcc/mcc.go:67-78** — діапазони неточні: `3500..3999` (готелі) маркуються як Transport; `5912` (аптеки) → Retail замість Health.
  - Виправити: розбити `3000..3999` на airlines (3000-3299), car-rental (3300-3499), lodging (3500-3999); додати override-мапу для health-MCC (5912, 5122, 5975, 8011, 8021, 8041-8062, 8071, 8099).
  - Resolution:

- [ ] **bank/data.go:131-132** — `MCC int32` зі знаком; немає валідації 1..9999.
  - Виправити: у `MCCCode()` повертати `0`/`Unknown` для значень поза 1..9999.
  - Resolution:

- [ ] **corporate/signature.go:111,128** — query конкатенується через `?`; якщо у `baseURL` уже є query — ламається.
  - Виправити: побудова через `url.URL{Path:..., RawQuery: url.Values{}.Encode()}`.
  - Resolution:

- [ ] **client.go:173-179** — `SetBaseURL` мовчки лишає попереднє значення при `url.Parse` помилці.
  - Виправити: повертати error або записувати у `optErr`.
  - Resolution:

---

## MEDIUM

- [ ] **client.go:274-278** — limiter викликається 1× ДО retry-циклу → 4 ретраї проходять як 1 токен.
  - Виправити: переносити `Wait` у `attempt`-функцію (всередину retry-loop).
  - Resolution:

- [ ] **acquiring/types.go:324-339** — `InvoiceStatusResponse.UnmarshalJSON` перезаписує `Code` на Fee/AgentFee — для крос-граничних транзакцій валюта Fee може відрізнятись.
  - Виправити: перевірити з docs Mono; якщо різні — не перезаписувати, парсити з власної валюти поля.
  - Resolution:

- [ ] **acquiring/subscription.go:260, 296** — `time.RFC3339` форматує локальну TZ.
  - Виправити: `t.UTC().Format(time.RFC3339)`.
  - Resolution:

- [ ] **acquiring/types.go:608-617** — `WalletPaymentRequest.InitiationKind` без `omitempty`.
  - Виправити: додати `omitempty`.
  - Resolution:

- [ ] **acquiring/{invoice,qr,wallet,subscription,monopay}.go** — не валідуються порожні ID у мутаційних запитах.
  - Виправити: `if id == "" { return ErrEmptyID }` у кожній функції; винести `ErrEmptyID` у спільне місце.
  - Resolution:

- [ ] **business/idempotency.go:23-45** — `panic` при `crypto/rand` fail.
  - Виправити: повертати `(string, error)` із `NewIdempotencyKey`; внутрішні викликачі обробляють error.
  - Resolution:

- [ ] **business/payment.go:33** — Idempotency-Key не валідується на порожній рядок.
  - Виправити: повертати `ErrIdempotencyKeyRequired` при `key == ""`.
  - Resolution:

- [ ] **acquiring/client.go:86-92** — `TokenAuth` не ставить `Accept` (на відміну від `business.TokenAuth`).
  - Виправити: додати `Accept: application/json`.
  - Resolution:

- [ ] **webhook/handler.go:259-263** — якщо `Transaction.ID == ""`, dedup no-op; OnEvent виконається на кожен ретрай.
  - Виправити: викликати `OnError` із warning, ack-нути 200.
  - Resolution:

- [ ] **webhook/parse.go** — без перевірки розміру тіла (caller покладається на MaxBytesReader).
  - Виправити: задокументувати у godoc функції `Parse`.
  - Resolution:

- [ ] **jar/jar.go:97-116** — `UnmarshalJSON` робить два `Unmarshal` і ковтає другу помилку.
  - Виправити: один Unmarshal у aux-struct з `*string` для optional-поля.
  - Resolution:

- [ ] **jar/jar.go:243-247** — `if Unmarshal(_, maybeErr)==nil && maybeErr.ErrCode!=""` спрацює на будь-якому JSON із `errCode`.
  - Виправити: жорсткіше — парсити `{errCode, errText}` із DisallowUnknownFields у тимчасовий struct АБО перевіряти status-code раніше.
  - Resolution:

- [ ] **monobanktest/server.go:117-125** — `HandlePrefix` спрацьовує перший доданий, а не найдовший префікс.
  - Виправити: сортувати handlers за довжиною prefix DESC перед матчингом.
  - Resolution:

- [ ] **monobanktest/server.go:129-148** — `s.t.Errorf` із goroutine може race-нути з cleanup при запитах після Close.
  - Виправити: idempotent close через `sync.Once`; ctx-чек на shutdown; ігнорувати запити після close.
  - Resolution:

- [ ] **otelmonobank/otel.go:103** — `http.status_code` як `attribute.String` (semconv очікує Int).
  - Виправити: `attribute.Int("http.status_code", resp.StatusCode)`.
  - Resolution:

- [ ] **otelmonobank/otel.go:72** — `http.url` без query-redaction.
  - Виправити: записувати тільки `req.URL.Path` АБО фільтрувати query через allowlist.
  - Resolution:

- [ ] **otelmonobank/otel.go:48-51** — `WithTracer` перезаписує існуючі hooks.
  - Виправити: chain — викликати попередній hook після власного.
  - Resolution:

- [ ] **installment/types.go** — `float64` для грошей по всьому пакету.
  - Виправити: ввести типовану `Money` обгортку з `MarshalJSON`/`UnmarshalJSON` (рендерить як decimal-string без втрат), застосувати у структурах. Це breaking change → новий мажорний реліз.
  - Resolution:

- [ ] **currency/currency.go:30,80** — глобальні `var map` мутабельні.
  - Виправити: закрити за функціями `FromAlpha3(code) (Currency, bool)` / `Decimals(code) int`; саму мапу зробити пакетно-приватною.
  - Resolution:

- [ ] **personal/client.go:127, corporate/settings.go:50** — `SetWebHook("")` дозволено, але семантика не задокументована.
  - Виправити: явний godoc «передай порожній рядок, щоб скасувати підписку».
  - Resolution:

- [x] **installment/client.go:69-71** — `WithBaseURL` мовчки приймає http://.
  - Виправити: повертати помилку для http://, окрім loopback (та сама логіка, що `monobank.WithBaseURL`).
  - Resolution: `installment.New` тепер відхиляє `http://` для non-loopback з `ErrInsecureBaseURL`; opt-out через `WithInsecureBaseURL(true)`; loopback (`localhost`, `127.0.0.1`, `::1`) дозволений для httptest.

- [ ] **installment/client.go:124-128, 143-153** — `Sign` рахує HMAC лише за body; `VerifyCallback` повертає той самий sentinel для невалідної довжини й невалідного HMAC.
  - Виправити: задокументувати у godoc `Sign`; у `VerifyCallback` різні sentinel-и (`ErrCallbackBadLength`, `ErrCallbackBadSignature`).
  - Resolution:

- [ ] **corporate/registration.go:30-38** — `Logo []byte` без обмеження розміру.
  - Виправити: захардкодити max 1 MiB і повертати error раніше.
  - Resolution:

---

## LOW

- [ ] **client.go:194** — `func (c Client) Close()` value-receiver.
  - Виправити: змінити на `*Client` для консистентності.
  - Resolution:

- [ ] **client.go:219-228** — `optErr` повертається з кожного `Do` назавжди.
  - Виправити: документувати чітко; опційно — `(c *Client) ResetOptErr()` для тестів.
  - Resolution:

- [ ] **retry.go:142-157** — `parseRetryAfter` приймає арбітрарно великий integer.
  - Виправити: hard ceiling на parseRetryAfter (наприклад, 24h) із warning у logger.
  - Resolution:

- [ ] **business/payslips.go:115-132** — підтвердити, що `Client.Do` для `*[]byte` НЕ робить JSON-decode (зараз — окрема гілка, ок; добав regression-тест).
  - Resolution:

- [ ] **installment/types.go:179-184** — `ClientInfo` повертає PII; легко витече у логах.
  - Виправити: реалізувати `LogValue`, маскуючи FirstName/LastName/INN.
  - Resolution:

- [ ] **corporate/signature.go:24-25** — magic number «3 доби» у коментарях.
  - Виправити: винести у `const SignatureRequestTTL = 72 * time.Hour`.
  - Resolution:

- [ ] **examples/business/main.go:40-41** — `%.2f` для float-балансу замість типізованого `BalanceMoney().String()`.
  - Виправити: замінити у прикладі.
  - Resolution:

- [ ] **examples/corporate/main.go:101** — `%d` для `money.Money`-struct.
  - Виправити: `a.Balance.String()` або `%s`.
  - Resolution:

- [ ] **examples/webhook/main.go:33** — `http.ListenAndServe` без `ReadHeaderTimeout`.
  - Виправити: `&http.Server{Addr: ":8080", ReadHeaderTimeout: 10*time.Second, Handler: h}`.
  - Resolution:

- [ ] **examples/jar/main.go:80** — `float64(info.Amount)/float64(info.Goal)*100` без коментаря «not for accounting».
  - Виправити: коментар у прикладі.
  - Resolution:

- [ ] **examples/installment/main.go:55-67** — `TotalSum: 2499.99` як float64.
  - Виправити: коли буде типована Money — використати її.
  - Resolution:

- [ ] **doc.go** — англійська, тоді як решта пакетів — українська.
  - Виправити: узгодити стиль (рекомендую залишити англійською кореневий пакет, оскільки godoc.org).
  - Resolution:

- [ ] **monobanktest/responder.go:39** — `Error` шле тільки `errorDescription`.
  - Виправити: додати поле `errCode` (опт-аут default ""). Не ламати існуючий API.
  - Resolution:

- [ ] **personal/iter.go:35, corporate/iter.go:29** — зайвий `ctx.Err()` (дублюється нижче).
  - Виправити: видалити.
  - Resolution:

- [ ] **acquiring/types.go:120,135** — `Tax []int`.
  - Виправити: типізована обгортка `TaxRate int` із константами.
  - Resolution:

- [ ] **jar/jar.go:200** — magic `"random"`.
  - Виправити: `const jarRandomMode = "random"` із коментарем чому саме «random».
  - Resolution:

---

## NIT

- [ ] **acquiring/webhook.go:42** — `string(keyB64)` зайва копія.
  - Виправити: `base64.StdEncoding.Decode(dst, keyB64)`.
  - Resolution:

- [ ] **business/payment.go:65, payslips.go:64**, тощо — `url.Values{}` коли є рівно один параметр.
  - Виправити: `url.QueryEscape` напряму (мікрооптимізація, мала користь).
  - Resolution:

- [ ] **acquiring/subscription.go:55** — godoc-приклади інтервалу можуть бути неточні.
  - Виправити: звірити з docs.monobank.ua/acquiring; зафіксувати приклад.
  - Resolution:

- [ ] **acquiring/types.go:111-114, 138-148** — `SubscriptionStatusResponse.WalletData` не вказівник; `SubscriptionListItem` без WalletData.
  - Виправити: уніфікувати — `*WalletData` всюди.
  - Resolution:

- [ ] **business/api.go:34** — `Operation(id, externalReference)` — обидва string, компілятор не страхує.
  - Виправити: типізовані алиаси `type OperationID string`, `type ExternalRef string`.
  - Resolution:

- [ ] **client.go ResolveReference** — якщо `req.URL` уже абсолютний — пройде у запит.
  - Виправити: явно вимагати path-only у docstring `Do`; опційно — error при абсолютному.
  - Resolution:

- [ ] **monobanktest/server.go:38** — `sync.Mutex` замість `sync.RWMutex`.
  - Виправити: тільки якщо тести стануть гарячими — наразі overkill.
  - Resolution:

- [ ] **currency/currency.go:48-53** — `init()` для побудови оберненої мапи.
  - Виправити: `sync.OnceValue` (Go 1.21+) — не критично.
  - Resolution:

- [ ] **examples/personal/main.go:65** — не критично, але `len(info.Accounts) > 0` перевіряється вище — OK.
  - Resolution: not actionable.

- [ ] **bank/serverkey.go:69-73** — годний коментар про MITM ✅.
  - Resolution: не потребує дій.

---

## Інструкції для агента

1. Виправляй пункти **знизу вгору серйозності НЕ варто** — починай з CRITICAL.
2. **Один пункт = один коміт** (атомарність + ревью). Виняток: пов'язані pagination-bugs (#5 + #6 — один pr).
3. До кожного CRITICAL/HIGH **обов'язково** додавай regression-тест (golangci-lint + `go test ./...` mусить пройти).
4. Усі breaking-зміни (наприклад, `installment.New` повертає error, `business.NewIdempotencyKey` повертає error, типована `Money` для installment) — у CHANGELOG.md + retract попередньої мінорки.
5. Після кожного фікса:
   - постав `[x]` у відповідному пункті;
   - впиши `Resolution: <commit-sha або 1-рядковий опис>`;
   - якщо знаходиш нові баги під час фікса — додавай їх у відповідну секцію цього файла зі статусом `[ ]`.
6. Якщо пункт виявляється не-багом / задокументованою поведінкою, став `[x]` і в `Resolution:` поясни чому (не видаляй).
