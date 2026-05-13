package installment

import (
	"context"
	"net/http"
)

// GuaranteeLetterPDF повертає байти PDF-файлу гарантійного листа для
// заявки. Передавати invoice (число + дата) опційно — деякі магазини
// вшивають його в назву документа.
//
// POST /api/order/guarantee/letter  (200 → application/pdf)
func (c *Client) GuaranteeLetterPDF(ctx context.Context, in *OrderDataRequest) ([]byte, error) {
	if in == nil {
		return nil, ErrNilRequest
	}
	return c.doPDF(ctx, "/api/order/guarantee/letter", in)
}

// GuaranteeLetterData повертає структуровані дані для самостійної
// генерації гарантійного листа (XML/PDF на стороні магазину).
//
// POST /api/order/data/for/guarantee/letter  (200 → OrderData)
func (c *Client) GuaranteeLetterData(ctx context.Context, in *OrderDataRequest) (*OrderData, error) {
	if in == nil {
		return nil, ErrNilRequest
	}
	var out OrderData
	if err := c.doJSON(ctx, "/api/order/data/for/guarantee/letter", in, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}

// GuaranteeLetterDataV2 — версія 2 GuaranteeLetterData: повертає ту саму
// структуру [OrderData], але з додатково заповненими contract_number та
// contract_date у Header.
//
// POST /api/v2/order/data/for/guarantee/letter  (200 → OrderData)
func (c *Client) GuaranteeLetterDataV2(ctx context.Context, in *OrderDataRequest) (*OrderData, error) {
	if in == nil {
		return nil, ErrNilRequest
	}
	var out OrderData
	if err := c.doJSON(ctx, "/api/v2/order/data/for/guarantee/letter", in, &out, http.StatusOK); err != nil {
		return nil, err
	}
	return &out, nil
}
