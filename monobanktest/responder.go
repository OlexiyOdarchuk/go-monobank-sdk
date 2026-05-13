package monobanktest

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
)

// Responder — щось, що знає, як відповісти на HTTP-запит. Інтерфейс
// той самий, що в [http.Handler], — будь-який існуючий handler можна
// використати як Responder.
type Responder interface {
	RespondHTTP(w http.ResponseWriter, r *http.Request)
}

// ResponderFunc дозволяє звичайній функції бути [Responder].
type ResponderFunc func(http.ResponseWriter, *http.Request)

// RespondHTTP — задовольняє [Responder].
func (f ResponderFunc) RespondHTTP(w http.ResponseWriter, r *http.Request) { f(w, r) }

// JSON повертає Responder, що серіалізує body у JSON і відповідає
// HTTP 200 (або статусом, переданим у [WithStatus]). Якщо body — це
// рядок чи []byte, він пишеться як є (для випадків, коли треба точний
// контроль над форматом).
func JSON(body any) Responder {
	return ResponderFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		writeBody(w, body)
	})
}

// Error повертає Responder, що відповідає вказаним статусом і JSON
// `{"errorDescription": msg}` у тілі (формат помилки Mono).
func Error(status int, msg string) Responder {
	return ResponderFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"errorDescription": msg})
	})
}

// Status повертає Responder, що віддає тільки статус без тіла (зручно
// для 204, 401 тощо).
func Status(status int) Responder {
	return ResponderFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	})
}

// Sequence повертає Responder, що при i-му виклику віддає i-ий
// Responder зі списку. Корисно для тестів ретраїв: перший виклик
// повертає 503, другий — 200.
//
//	srv.Handle("GET", "/x", monobanktest.Sequence(
//	    monobanktest.Status(503),
//	    monobanktest.JSON(`{"ok":true}`),
//	))
//
// Після вичерпання списку — кожен наступний виклик отримує останній
// елемент (щоб тест не падав на додатковому ретраї).
func Sequence(responders ...Responder) Responder {
	if len(responders) == 0 {
		return Status(http.StatusInternalServerError)
	}
	var i atomic.Int32
	return ResponderFunc(func(w http.ResponseWriter, r *http.Request) {
		idx := int(i.Add(1) - 1)
		if idx >= len(responders) {
			idx = len(responders) - 1
		}
		responders[idx].RespondHTTP(w, r)
	})
}

// writeBody обробляє body одним із форматів: []byte, string, або JSON.
func writeBody(w http.ResponseWriter, body any) {
	switch v := body.(type) {
	case nil:
		return
	case []byte:
		_, _ = w.Write(v)
	case string:
		_, _ = w.Write([]byte(v))
	default:
		_ = json.NewEncoder(w).Encode(v)
	}
}
