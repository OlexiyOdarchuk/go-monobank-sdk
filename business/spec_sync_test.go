package business

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSalaryRecipient_additionalPurposeWire(t *testing.T) {
	b, err := json.Marshal(SalaryRecipient{IBAN: "UA1", Amount: 1, AdditionalPurpose: " за жовтень 2026"})
	require.NoError(t, err)
	assert.Contains(t, string(b), `"additionalPurpose":" за жовтень 2026"`)

	b, err = json.Marshal(SalaryRecipient{IBAN: "UA1", Amount: 1})
	require.NoError(t, err)
	assert.NotContains(t, string(b), "additionalPurpose", "omitted when empty")
}

func TestSalaryRegistryTypes_additionalPurposeAllowed(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[
			{"alias":"SALARY","description":"Зарплата","additionalPurposeAllowed":true},
			{"alias":"SALARY_ADVANCE","description":"Аванс"}]`))
	})
	types, err := c.SalaryRegistryTypes(context.Background())
	require.NoError(t, err)
	require.Len(t, types, 2)
	assert.True(t, types[0].AdditionalPurposeAllowed)
	assert.False(t, types[1].AdditionalPurposeAllowed, "absent means not allowed")
}

func TestRegistryStatus_terminal(t *testing.T) {
	for _, s := range []RegistryStatus{RegistryFailed, RegistrySucceeded, RegistryFail, RegistryPaymentsDone} {
		assert.True(t, s.Terminal(), s)
	}
	for _, s := range []RegistryStatus{RegistrySaved, RegistryInProgress, RegistryPrepared,
		RegistryChangeRequired, RegistryAPILoading, RegistryAdded, ""} {
		assert.False(t, s.Terminal(), s)
	}
	assert.True(t, RegistryFailed.Failed())
	assert.True(t, RegistryFail.Failed())
	assert.False(t, RegistrySucceeded.Failed())
}

// fakeStatement serves /ext/v1/statement/{account}/{from}[/{to}] the
// way a real server would: it filters by the inclusive window, sorts
// in the given order and cuts the result to limit.
func fakeStatement(t *testing.T, times []int64, newestFirst bool) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/ext/v1/statement/"), "/")
		lo, err := strconv.ParseInt(parts[1], 10, 64)
		require.NoError(t, err)
		hi := time.Now().Unix()
		if len(parts) > 2 {
			hi, err = strconv.ParseInt(parts[2], 10, 64)
			require.NoError(t, err)
		}
		limit := 10
		if l := r.URL.Query().Get("limit"); l != "" {
			limit, _ = strconv.Atoi(l)
		}
		type op struct {
			id string
			ts int64
		}
		var in []op
		for i, ts := range times {
			if ts >= lo && ts <= hi {
				in = append(in, op{fmt.Sprintf("op-%d", i), ts})
			}
		}
		sort.SliceStable(in, func(a, b int) bool {
			if newestFirst {
				return in[a].ts > in[b].ts
			}

			return in[a].ts < in[b].ts
		})
		if len(in) > limit {
			in = in[:limit]
		}
		items := make([]string, len(in))
		for i, o := range in {
			items[i] = fmt.Sprintf(`{"id":%q,"time":%d,"amount":-1,"currencyCode":"UAH"}`, o.id, o.ts)
		}
		_, _ = w.Write([]byte("[" + strings.Join(items, ",") + "]"))
	}
}

// StatementAll must return every operation exactly once whichever
// order the server pages in, including operations that share a second
// with a page edge.
func TestStatementAll_eitherOrder(t *testing.T) {
	base := int64(1_700_000_000)
	var times []int64
	for i := int64(0); i < 23; i++ {
		times = append(times, base+100+i*10)
	}
	// Three operations on one second, straddling a page edge.
	times = append(times, base+150, base+150, base+150)

	for _, newestFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("newestFirst=%t", newestFirst), func(t *testing.T) {
			c, _ := newTestClient(t, fakeStatement(t, times, newestFirst))
			got := map[string]int{}
			for op, err := range c.StatementAll(context.Background(), "acc",
				time.Unix(base, 0), time.Unix(base+1000, 0), 4) {
				require.NoError(t, err)
				got[op.ID]++
			}
			assert.Len(t, got, len(times), "every operation is yielded")
			for id, n := range got {
				assert.Equal(t, 1, n, "%s yielded once", id)
			}
		})
	}
}

// A second holding more operations than a page: only one page of that
// second is reachable without an offset, but the rest of the window
// must still be read, in either order.
func TestStatementAll_sameSecondOverflow_continuesPastIt(t *testing.T) {
	base := int64(1_700_000_000)
	times := []int64{base + 10, base + 20}
	for i := 0; i < 7; i++ {
		times = append(times, base+500)
	}
	times = append(times, base+900, base+950)

	for _, newestFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("newestFirst=%t", newestFirst), func(t *testing.T) {
			var calls int
			h := fakeStatement(t, times, newestFirst)
			c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				h(w, r)
			})
			got := map[string]int{}
			for op, err := range c.StatementAll(context.Background(), "acc",
				time.Unix(base, 0), time.Unix(base+1000, 0), 3) {
				require.NoError(t, err)
				got[op.ID]++
			}
			for _, id := range []string{"op-0", "op-1", "op-9", "op-10"} {
				assert.Equal(t, 1, got[id], "%s on the far side of the busy second", id)
			}
			assert.Less(t, calls, 20, "no refetch loop")
		})
	}
}
