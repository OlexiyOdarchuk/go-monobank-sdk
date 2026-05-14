package installment

import (
	"context"
	"net/http"
)

// GuaranteeLetterPDF returns the PDF bytes of the guarantee letter
// for an order. Passing invoice (number + date) is optional — some
// merchants embed it in the document name.
//
// POST /api/order/guarantee/letter  (200 → application/pdf)
func (c *Client) GuaranteeLetterPDF(ctx context.Context, in *OrderDataRequest) ([]byte, error) {
	if in == nil {
		return nil, ErrNilRequest
	}
	return c.doPDF(ctx, "/api/order/guarantee/letter", in)
}

// GuaranteeLetterData returns structured data so the merchant can
// generate the guarantee letter themselves (XML/PDF on the merchant
// side).
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

// GuaranteeLetterDataV2 is the version 2 of GuaranteeLetterData: it
// returns the same [OrderData] structure but with contract_number
// and contract_date additionally populated in Header.
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
