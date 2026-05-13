package monobanktest

import (
	"net/http"

	"github.com/OlexiyOdarchuk/go-monobank-sdk/bank"
)

// Білдери — типові пресет-сценарії, щоб не писати .Handle вручну.
// Кожен повертає сам *Server для чейнінгу.

// WithClientInfo прив'язує відповідь GET /personal/client-info до
// вказаної ClientInfo. Stub-ить ОБИДВА Personal API і Corporate API
// (вони мають однаковий шлях).
func (s *Server) WithClientInfo(info *bank.ClientInfo) *Server {
	s.Handle(http.MethodGet, "/personal/client-info", JSON(info))
	return s
}

// WithRates прив'язує відповідь GET /bank/currency до вказаного списку
// курсів. Використовуй із bank.Client.Rates або як джерело для
// [bank.Rates.Convert] у тестах.
func (s *Server) WithRates(rates bank.Rates) *Server {
	s.Handle(http.MethodGet, "/bank/currency", JSON(rates))
	return s
}

// WithServerKey прив'язує відповідь GET /bank/sync до сирого
// raw-payload-у (на wire — поля serverKeyId, serverPubKey,
// serverTimeMsec). Конструктор [bank.ServerKey] — приватний; тому
// білдер приймає raw map, який ти точно знаєш форматом.
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

// WithStatement прив'язує всі виклики GET /personal/statement/{account}/*
// до одного й того ж списку транзакцій. Інші акаунти повернуть 404 —
// тест явно не очікує таких викликів.
//
// Працює для Personal і Corporate Open API однаково (шлях ідентичний).
func (s *Server) WithStatement(account string, txs bank.Transactions) *Server {
	s.HandlePrefix(http.MethodGet, "/personal/statement/"+account+"/", JSON(txs))
	return s
}

// WithWebHookSubscription прив'язує POST /personal/webhook (Personal API)
// до 200 OK — достатньо для тестів, які перевіряють тільки факт виклику.
// Для перевірки тіла запиту використовуй [Server.Handle] напряму.
func (s *Server) WithWebHookSubscription() *Server {
	s.Handle(http.MethodPost, "/personal/webhook", Status(http.StatusOK))
	return s
}
