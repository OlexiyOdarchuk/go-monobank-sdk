// Package monobanktest provides helpers for testing code that uses
// monobank-sdk: a fake HTTP server with routing and ready builders
// for common scenarios, plus contract interfaces for each client
// (see the matching sub-package docs).
//
// The intent is that an SDK user does not have to write their own
// `httptest.NewServer` with manual branching on `r.URL.Path` in
// every test.
//
//	srv := monobanktest.NewServer(t)
//	srv.WithClientInfo(&bank.ClientInfo{ID: "c1", Name: "Test"})
//	cli := personal.New("token", srv.Option())
//	info, _ := cli.ClientInfo(ctx)  // returns what was stubbed above
//
// Builder coverage: [Server.WithClientInfo], [Server.WithRates],
// [Server.WithServerKey], [Server.WithStatement],
// [Server.WithWebHookSubscription] are available — primarily
// personal/corporate + bank. For acquiring/business/installment/jar
// use [Server.Handle] directly to add matching for the relevant
// path.
package monobanktest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk"
)

// Server is a fake monobank HTTP server. It wraps [httptest.Server]
// with (method, path) matching. Safe for concurrent use —
// goroutine-friendly.
type Server struct {
	t   testing.TB
	srv *httptest.Server

	mu           sync.Mutex
	routes       map[routeKey]Responder
	prefixRoutes []prefixRoute
}

type routeKey struct {
	method string
	path   string
}

// prefixRoute is a route that matches by path prefix instead of an
// exact equality. Needed for endpoints like
// /personal/statement/{...}/{...} where part of the path is
// parameters.
type prefixRoute struct {
	method string
	prefix string
	resp   Responder
}

// NewServer starts the fake server. t.Cleanup closes it when the
// test finishes. Routes are added via [Server.Handle] or builders
// (WithClientInfo, WithRates, WithStatement etc.).
func NewServer(t testing.TB) *Server {
	t.Helper()
	s := &Server{
		t:      t,
		routes: make(map[routeKey]Responder),
	}
	s.srv = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.srv.Close)
	return s
}

// URL returns this server's base URL.
func (s *Server) URL() string { return s.srv.URL }

// Close stops the server. It is called automatically via t.Cleanup —
// only needed if you want to close it earlier.
func (s *Server) Close() { s.srv.Close() }

// Option returns a [monobank.Option] that points the client at this
// server (WithBaseURL + WithHTTPClient). Pass it into any
// sub-package factory:
//
//	cli := personal.New(token, srv.Option())
//	cli := business.New(token, srv.Option())
//	cli := corporate.New(maker, srv.Option())
func (s *Server) Option() monobank.Option {
	return optionGroup{
		monobank.WithBaseURL(s.srv.URL),
		monobank.WithHTTPClient(s.srv.Client()),
	}.apply
}

// optionGroup is a composite [monobank.Option] built from several
// options.
type optionGroup []monobank.Option

func (g optionGroup) apply(c *monobank.Client) {
	for _, o := range g {
		o(c)
	}
}

// Handle registers a [Responder] for a (method, path) pair. If a
// Responder is already registered for that pair, it is replaced
// (handy when the test overrides a builder).
//
//	srv.Handle(http.MethodGet, "/bank/currency", monobanktest.JSON(rates))
func (s *Server) Handle(method, path string, r Responder) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routes[routeKey{method, path}] = r
}

// HandlePrefix registers a Responder that triggers for any path
// starting with prefix and matching method. Useful for endpoints
// with parameters in the path (for example,
// /personal/statement/{account}/{from}/{to}). Exact matches via
// [Server.Handle] have higher priority.
func (s *Server) HandlePrefix(method, prefix string, r Responder) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prefixRoutes = append(s.prefixRoutes, prefixRoute{
		method: method,
		prefix: prefix,
		resp:   r,
	})
}

// handle is the server's root handler; it looks up a Responder by
// (method, path), trying an exact match first and then a prefix
// match.
func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	resp, ok := s.routes[routeKey{r.Method, r.URL.Path}]
	if !ok {
		for _, pr := range s.prefixRoutes {
			if pr.method == r.Method && strings.HasPrefix(r.URL.Path, pr.prefix) {
				resp = pr.resp
				ok = true
				break
			}
		}
	}
	s.mu.Unlock()

	if !ok {
		s.t.Errorf("monobanktest: unexpected %s %s — no Responder registered", r.Method, r.URL.Path)
		http.Error(w, "no handler registered", http.StatusNotImplemented)
		return
	}
	resp.RespondHTTP(w, r)
}
