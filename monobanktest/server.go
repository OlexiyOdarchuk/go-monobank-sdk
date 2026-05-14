// Package monobanktest надає helper-и для тестування коду, що
// використовує monobank-sdk: фейковий HTTP-сервер із роутингом і
// готовими білдерами під типові сценарії, плюс контракти-інтерфейси
// для кожного клієнта (див. документацію відповідних підпакетів).
//
// Інтенція — щоб користувач SDK не писав власний `httptest.NewServer`
// з ручним розгалуженням по `r.URL.Path` у кожному тесті.
//
//	srv := monobanktest.NewServer(t)
//	srv.WithClientInfo(&bank.ClientInfo{ID: "c1", Name: "Test"})
//	cli := personal.New("token", srv.Option())
//	info, _ := cli.ClientInfo(ctx)  // повертає те, що зашили вище
//
// Покриття білдерами: наразі готові [Server.WithClientInfo],
// [Server.WithRates], [Server.WithServerKey], [Server.WithStatement],
// [Server.WithWebHookSubscription] — це переважно personal/corporate
// + bank. Для acquiring/business/installment/jar використовуй
// [Server.Handle] напряму, щоб додати матчинг на потрібний шлях.
package monobanktest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	monobank "github.com/OlexiyOdarchuk/go-monobank-sdk"
)

// Server — фейковий monobank HTTP-сервер. Обгортка над
// [httptest.Server] з матчингом по (method, path). Безпечний для
// конкурентного використання — gorout-friendly.
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

// prefixRoute — маршрут, що матчиться по префіксу шляху замість точного
// збігу. Потрібен для endpoint-ів типу /personal/statement/{...}/{...},
// у яких частина шляху — параметри.
type prefixRoute struct {
	method string
	prefix string
	resp   Responder
}

// NewServer стартує фейковий сервер. t.Cleanup закриє його по
// завершенню тесту. Маршрути додаються через [Server.Handle] або
// білдери (WithClientInfo, WithRates, WithStatement тощо).
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

// URL повертає base URL цього сервера.
func (s *Server) URL() string { return s.srv.URL }

// Close зупиняє сервер. Викликається автоматично через t.Cleanup —
// потрібен тільки якщо хочеш закрити раніше.
func (s *Server) Close() { s.srv.Close() }

// Option повертає [monobank.Option], який направляє клієнта на цей
// сервер (WithBaseURL + WithHTTPClient). Передавай у фабрику будь-якого
// підпакета:
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

// optionGroup — composit [monobank.Option] із кількох.
type optionGroup []monobank.Option

func (g optionGroup) apply(c *monobank.Client) {
	for _, o := range g {
		o(c)
	}
}

// Handle реєструє відповідник [Responder] на пару (method, path).
// Якщо для цієї пари вже зареєстрований відповідник — він буде замінений
// (зручно, коли тест перевизначає білдер).
//
//	srv.Handle(http.MethodGet, "/bank/currency", monobanktest.JSON(rates))
func (s *Server) Handle(method, path string, r Responder) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routes[routeKey{method, path}] = r
}

// HandlePrefix реєструє Responder, який спрацьовує для будь-якого
// шляху, що починається з prefix і відповідає method. Корисно для
// endpoint-ів із параметрами у шляху (наприклад
// /personal/statement/{account}/{from}/{to}). Точні матчі через
// [Server.Handle] мають вищий пріоритет.
func (s *Server) HandlePrefix(method, prefix string, r Responder) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prefixRoutes = append(s.prefixRoutes, prefixRoute{
		method: method,
		prefix: prefix,
		resp:   r,
	})
}

// handle — root-handler сервера; шукає Responder за (method, path),
// спочатку точний матч, потім префіксний.
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
