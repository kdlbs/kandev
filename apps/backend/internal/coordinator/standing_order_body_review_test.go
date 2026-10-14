package coordinator

import (
	"net/http"
	"strings"
	"testing"
)

func TestStandingOrderRejectsTrailingAndOversizedBodyWithoutWriting(t *testing.T) {
	for name, body := range map[string]string{
		"trailing document":       `{"text":"rule"}{"text":"second"}`,
		"trailing delimiter":      `{"text":"rule"}}`,
		"oversized unknown field": `{"text":"rule","padding":"` + strings.Repeat("x", 64<<10) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			store, c, svc := ordersFixture(t)
			h := &Handlers{service: svc, logger: newTestLogger(t)}
			valid := runHandler(h.httpAddStandingOrder, http.MethodPost, "/x", `{"text":"valid rule"}`, workspaceParams(c.ID))
			if valid.Code != http.StatusCreated {
				t.Fatalf("positive control status %d: %s", valid.Code, valid.Body.String())
			}
			bad := runHandler(h.httpAddStandingOrder, http.MethodPost, "/x", body, workspaceParams(c.ID))
			if bad.Code != http.StatusBadRequest {
				t.Fatalf("malformed body accepted: %d %s", bad.Code, bad.Body.String())
			}
			orders, err := store.ActiveStandingOrders(t.Context(), c.ID)
			if err != nil || len(orders) != 1 || orders[0].Text != "valid rule" {
				t.Fatalf("invalid request persisted an order: %+v, %v", orders, err)
			}
		})
	}
}
