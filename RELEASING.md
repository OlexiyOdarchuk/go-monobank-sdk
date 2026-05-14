# Releasing

Гайд для мейнтейнера. Інструкція для контриб'юторів — у [`CONTRIBUTING.md`](CONTRIBUTING.md).

## SemVer

- **patch** (`v0.1.x`) — багфікси, dependabot-апдейти, internal рефактор
  без зміни публічного API.
- **minor** (`v0.x.0`) — нові endpoint-и, нові опції, нові експортовані
  типи. Не ламають існуючі підписи.
- **major** (`v1.0.0`+) — зміни, що ламають існуючий код користувачів.
  Для Go-модулів v2+ потребує `/v2` в шляху імпорту.

Поки `v0.x.x`: формально дозволено ламати API в мінорних, але SDK
тримає сумісність — фактичні breaking-зміни підуть тільки з `v1.0.0`.

## Чек-лист релізу

Жодного `git tag` без КОЖНОГО пункту нижче. Один пропущений рядок —
один retract.

### Перед комітом

1. **CHANGELOG**:
   - перенести зміни з `[Unreleased]` у нову секцію
     `## [X.Y.Z] — YYYY-MM-DD`;
   - якщо breaking — додати розділ `### Migration з vX.Y` із
     before/after-прикладами;
   - оновити посилання-діффи внизу файла:
     - додати `[X.Y.Z]: …compare/v(prev)...vX.Y.Z`;
     - оновити `[Unreleased]` на `…compare/vX.Y.Z...HEAD`.

2. **README sync**:
   - перевірити, що жоден код-блок у `README.md` / `README.en.md` не
     посилається на видалені/перейменовані символи;
   - якщо додано опцію/функцію — переконатися, що вона згадана хоча б
     в одній з: `README`, `doc.go`, `Example*` тест.

3. **Локальні перевірки** (фейл — стоп):

   ```bash
   make ci                                # fmt + vet + test-race
   env -u GOFLAGS go test -count=20 -race ./...   # race-stress
   make bench                             # нічого не сповільнилося
   make fuzz-all FUZZTIME=10s             # fuzz-прогін усіх цілей
   govulncheck ./...                      # CVE-сканер
   ```

4. **README-приклади компілюються**: всі `examples/*` мають збиратися
   через `make ci` (там є `go build ./examples/...`). Якщо додав
   новий блок коду в README — додай мінімальний `examples/<feature>`
   або `Example*`-тест, який цей блок повторює.

5. **retract** старих версій (якщо актуально): якщо попередня
   `v1.X.Y` має критичний баг або зламану документацію, додай
   `retract v1.X.Y` у `go.mod` із коротким коментарем у тому ж
   commit-і, що готує реліз.

### Коміт і CI

6. **Коміт**:

   ```bash
   git add CHANGELOG.md README.md README.en.md ...
   git commit -m "Release vX.Y.Z"
   git push origin main
   ```

7. **Дочекатися зеленого CI на main**. **НЕ** тегати раніше — навіть
   якщо тести проходять локально. CI має повну матрицю Go-версій,
   яких у тебе локально немає.

   ```bash
   gh run watch --exit-status
   ```

### Тег

8. **Тільки після зеленого CI** на main — signed tag:

   ```bash
   git tag -a vX.Y.Z -m "vX.Y.Z"
   git push origin vX.Y.Z
   ```

9. **Workflow `release.yaml` автоматично**:
   - витягне секцію `## [X.Y.Z]` з `CHANGELOG.md`;
   - створить GitHub Release із цим body.

   Якщо очікувана GitHub Release не зʼявилася за 2 хв — глянь
   `gh run list --workflow=release.yaml`.

10. **pkg.go.dev**:
    - відкрити <https://pkg.go.dev/github.com/OlexiyOdarchuk/go-monobank-sdk@vX.Y.Z>;
    - якщо нова версія не з'являється протягом 5-10 хв —
      `GOPROXY=https://proxy.golang.org go list -m github.com/OlexiyOdarchuk/go-monobank-sdk@vX.Y.Z`
      форсить індексацію.

### Якщо тег виявився поламаним

Див. розділ нижче — НЕ видаляти тег, випустити patch + retract.

## Реліз `otelmonobank` submodule

`otelmonobank/` — окремий Go-модуль з власним `go.mod`. Його теги
формату `otelmonobank/vX.Y.Z`. Випускати окремо лише коли в submodule
є зміни — або разом із основним релізом для зручності.

```bash
git tag -a otelmonobank/vX.Y.Z -m "otelmonobank vX.Y.Z"
git push origin otelmonobank/vX.Y.Z
```

## Якщо тег виявився поламаний

Видалення тегу — погана практика (модулі, що його вже завантажили,
кешовані у `proxy.golang.org` назавжди). Натомість:

1. Випустити новий patch (`vX.Y.Z+1`) із виправленням.
2. Якщо проблема критична — додати запис у `CHANGELOG.md` під
   попередньою версією: `### Yanked`.
3. Опційно — позначити версію як `retracted` у `go.mod`:

   ```go
   retract vX.Y.Z // contains a bug in <…>
   ```

   `go get` показуватиме попередження користувачам.
