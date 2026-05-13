// Package auth — авторизатори для запитів до monobank API.
//
// Інтерфейс [Authorizer] дозволяє базовому HTTP-клієнту лишатися
// агностиком щодо того, як конкретний запит авторизується. Три
// реалізації покривають усі публічні поверхні Mono:
//
//   - [Public] — для endpoint-ів без авторизації (курси, серверний ключ).
//   - [Personal] — токен Personal API (заголовок X-Token).
//   - [Corp] — ECDSA-підписаний доступ Corporate API (X-Key-Id, X-Time,
//     X-Sign, плюс X-Request-Id або X-Permissions залежно від endpoint-а).
//
// Підпакети business (corp-api юр. осіб) та acquiring використовують
// простішу схему з одного X-Token і мають власні authorizer-и в
// своїх пакетах.
package auth

import "net/http"

// Authorizer модифікує вихідний запит, додаючи credentials, на які
// чекає певна поверхня monobank.
type Authorizer interface {
	SetAuth(*http.Request) error
}

// Public — no-op Authorizer для endpoint-ів, яким не потрібна авторизація
// (наприклад /bank/currency, /bank/sync).
type Public struct{}

// SetAuth реалізує [Authorizer]; нічого не робить.
func (Public) SetAuth(_ *http.Request) error { return nil }

// NewPublic повертає no-op Authorizer для неавторизованих викликів.
func NewPublic() Public { return Public{} }
