package monobanktest

import (
	"net/http"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/bank"
)

// Builders are preset scenarios so you do not have to write .Handle
// by hand. Each one returns the *Server for chaining.

// WithClientInfo binds the response of GET /personal/client-info to
// the given ClientInfo. It stubs BOTH the Personal API and the
// Corporate API (they share the same path).
func (s *Server) WithClientInfo(info *bank.ClientInfo) *Server {
	s.Handle(http.MethodGet, "/personal/client-info", JSON(info))
	return s
}

// WithRates binds the response of GET /bank/currency to the given
// list of rates. Use with bank.Client.Rates or as a source for
// [bank.Rates.Convert] in tests.
func (s *Server) WithRates(rates bank.Rates) *Server {
	s.Handle(http.MethodGet, "/bank/currency", JSON(rates))
	return s
}

// WithServerKey binds the response of GET /bank/sync to a raw
// payload (on the wire: the fields serverKeyId, serverPubKey,
// serverTimeMsec). The [bank.ServerKey] constructor is private, so
// the builder takes a raw map you control the format of.
//
//	srv.WithServerKey(map[string]any{
//	    "serverKeyId": "k1",
//	    "serverPubKey": "BNDZP+AGo...",  // base64 uncompressed point
//	    "serverTimeMsec": 1700000000000,
//	})
func (s *Server) WithServerKey(payload any) *Server {
	s.Handle(http.MethodGet, "/bank/sync", JSON(payload))
	return s
}

// WithStatement binds every GET /personal/statement/{account}/*
// call to the same list of transactions. Other accounts return 404 —
// the test explicitly does not expect those calls.
//
// Works identically for the Personal and Corporate Open API (the
// path is the same).
func (s *Server) WithStatement(account string, txs bank.Transactions) *Server {
	s.HandlePrefix(http.MethodGet, "/personal/statement/"+account+"/", JSON(txs))
	return s
}

// WithWebHookSubscription binds POST /personal/webhook (Personal API)
// to 200 OK — enough for tests that only check that the call was
// made. To verify the request body, use [Server.Handle] directly.
func (s *Server) WithWebHookSubscription() *Server {
	s.Handle(http.MethodPost, "/personal/webhook", Status(http.StatusOK))
	return s
}
