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

1. **Підготувати `CHANGELOG.md`:**
   - перенести зміни з `[Unreleased]` у нову секцію
     `## [X.Y.Z] — YYYY-MM-DD`;
   - оновити посилання-діффи внизу файла:
     - додати `[X.Y.Z]: …compare/v(prev)...vX.Y.Z`;
     - оновити `[Unreleased]` на `…compare/vX.Y.Z...HEAD`.

2. **Локальна перевірка:**

   ```bash
   make ci             # fmt-check + vet + test-race
   make bench          # переконатися, що нічого не повільнішає
   make fuzz-all FUZZTIME=10s   # швидкий fuzz-прогін усіх цілей
   ```

3. **Закомітити CHANGELOG:**

   ```bash
   git add CHANGELOG.md
   git commit -m "Release vX.Y.Z"
   git push origin main
   ```

   Дочекатися зеленого CI на main.

4. **Створити signed tag:**

   ```bash
   git tag -a vX.Y.Z -m "vX.Y.Z"
   git push origin vX.Y.Z
   ```

5. **Workflow `release.yaml` автоматично:**
   - витягне секцію `## [X.Y.Z]` з `CHANGELOG.md`;
   - створить GitHub Release із цим body.

6. **Перевірити pkg.go.dev:**
   - відкрити <https://pkg.go.dev/github.com/OlexiyOdarchuk/go-monobank-sdk@vX.Y.Z>;
   - якщо нова версія не з'являється протягом 5-10 хв — `GOPROXY=https://proxy.golang.org go list -m github.com/OlexiyOdarchuk/go-monobank-sdk@vX.Y.Z` форсить індексацію.

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
