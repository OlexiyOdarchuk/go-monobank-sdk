# Security Policy

## Supported Versions

Підтримуються виправлення безпеки для останньої мінорної версії за SemVer.
Для `0.x` релізів — лише найсвіжіша `0.x.y`.

| Версія | Підтримка |
|--------|-----------|
| 0.1.x  | ✅        |
| < 0.1  | ❌        |

## Reporting a Vulnerability

Не відкривайте публічних issue для вразливостей. Замість цього:

1. Скористайтеся [GitHub Security Advisories](https://github.com/OlexiyOdarchuk/go-monobank-sdk/security/advisories/new)
   (приватний звіт, видно лише мейнтейнеру).
2. У звіті вкажіть:
   - версію SDK і Go;
   - короткий опис вразливості та потенційний вплив;
   - PoC або кроки відтворення;
   - запропоноване виправлення (якщо є).

Очікуваний час відповіді — до 72 годин. Координуємо публічне розкриття
після релізу патчу.

## Scope

Цей SDK взаємодіє з продакшен-API monobank і обробляє токени, ECDSA-ключі,
HMAC-секрети та webhook-підписи. Особлива увага — до:

- верифікації webhook-підписів (`webhook.Verify`,
  `acquiring.VerifyWebhook`, `installment.VerifyCallback`);
- роботи з приватними ключами в `auth.NewCorpAuthMaker`;
- обробки токенів у `auth.NewPersonal`;
- дедуплікації webhook-подій (`webhook.Deduper`).

Вразливості в офіційному API monobank — не у скоупі цього репозиторію;
звертайтеся напряму до банку.
