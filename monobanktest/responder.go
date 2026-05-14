package monobanktest

import (
	"encoding/json"
	"net/http"
	"sync/atomic"
)

// Responder is anything that knows how to answer an HTTP request.
// The interface mirrors [http.Handler], so any existing handler can
// be used as a Responder.
type Responder interface {
	RespondHTTP(w http.ResponseWriter, r *http.Request)
}

// ResponderFunc lets a plain function act as a [Responder].
type ResponderFunc func(http.ResponseWriter, *http.Request)

// RespondHTTP satisfies [Responder].
func (f ResponderFunc) RespondHTTP(w http.ResponseWriter, r *http.Request) { f(w, r) }

// JSON returns a Responder that serializes body as JSON and replies
// with HTTP 200 (or the status passed via [WithStatus]). When body
// is a string or []byte, it is written as-is (for cases that need
// precise format control).
func JSON(body any) Responder {
	return ResponderFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		writeBody(w, body)
	})
}

// Error returns a Responder that replies with the given status and
// a JSON body `{"errorDescription": msg}` (Mono's error format).
func Error(status int, msg string) Responder {
	return ResponderFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"errorDescription": msg})
	})
}

// Status returns a Responder that writes only the status with no
// body (handy for 204, 401, etc.).
func Status(status int) Responder {
	return ResponderFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	})
}

// Sequence returns a Responder that, on the i-th call, dispatches
// the i-th Responder from the list. Useful for retry tests: the
// first call returns 503, the second returns 200.
//
//	srv.Handle("GET", "/x", monobanktest.Sequence(
//	    monobanktest.Status(503),
//	    monobanktest.JSON(`{"ok":true}`),
//	))
//
// Once the list is exhausted, every subsequent call gets the last
// element (so the test does not fail on an extra retry).
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

// writeBody handles body in one of three forms: []byte, string, or
// JSON.
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
