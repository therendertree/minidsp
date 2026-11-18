package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/therendertree/minidsp/bidder/internal/bidding"
	"github.com/therendertree/minidsp/bidder/internal/campaign"
)

var fixedNow = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

const okRequest = `{"id":"r1","imp":[{"id":"1","banner":{"w":300,"h":250},"bidfloor":1}],
"site":{"id":"s"},"device":{"os":"Android","geo":{"country":"USA"}}}`

func post(mux http.Handler, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	return rec
}

func camp(id, status string, cpm int64, extra string) string {
	return `{"id":"` + id + `","status":"` + status + `","cpm_micros":` + itoa(cpm) +
		`,"creatives":[{"crid":"cr-` + id + `","w":300,"h":250}]` + extra + `}`
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

func newBidder(t *testing.T, campaignsJSON ...string) (*Bidder, http.Handler) {
	t.Helper()
	cs, err := campaign.Parse([]byte(`{"campaigns":[` + strings.Join(campaignsJSON, ",") + `]}`))
	if err != nil {
		t.Fatal(err)
	}
	b := &Bidder{Campaigns: cs, Now: func() time.Time { return fixedNow }}
	return b, NewMux(b)
}

// One request per no-bid reason, through the real handler, checking the
// header and the counter.
func TestNoBidReasonsEndToEnd(t *testing.T) {
	tests := []struct {
		name      string
		campaigns []string
		reason    bidding.Reason
	}{
		{"empty list", nil, bidding.ReasonNoCampaigns},
		{"paused", []string{camp("a", "paused", 2_000_000, "")}, bidding.ReasonInactive},
		{"not started", []string{camp("a", "active", 2_000_000, `,"start":"2026-07-01T00:00:00Z"`)}, bidding.ReasonOutsideSchedule},
		{"ended window", []string{camp("a", "active", 2_000_000, `,"end":"2026-05-01T00:00:00Z"`)}, bidding.ReasonOutsideSchedule},
		{"geo", []string{camp("a", "active", 2_000_000, `,"geo":["CAN"]`)}, bidding.ReasonGeoMismatch},
		{"os", []string{camp("a", "active", 2_000_000, `,"os":["ios"]`)}, bidding.ReasonOSMismatch},
		{"floor", []string{camp("a", "active", 500_000, "")}, bidding.ReasonFloorAboveBid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, mux := newBidder(t, tc.campaigns...)
			rec := post(mux, "/openrtb2/bid", okRequest)
			if rec.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want 204; body=%s", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get(NoBidReasonHeader); got != string(tc.reason) {
				t.Errorf("header = %q, want %q", got, tc.reason)
			}
			if n := b.NoBids.Snapshot()[tc.reason]; n != 1 {
				t.Errorf("counter[%s] = %d, want 1", tc.reason, n)
			}
		})
	}
}

func TestTargetedCampaignBids(t *testing.T) {
	_, mux := newBidder(t,
		camp("a", "active", 2_000_000, `,"geo":["USA"],"os":["android"]`),
		camp("b", "active", 3_000_000, `,"geo":["CAN"]`), // higher CPM but wrong geo
	)
	rec := post(mux, "/openrtb2/bid", okRequest)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"crid":"cr-a"`) ||
		!strings.Contains(rec.Body.String(), `"price":2`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBadRequestsDoNotTouchNoBidCounter(t *testing.T) {
	b, mux := newBidder(t, camp("a", "active", 2_000_000, ""))
	rec := post(mux, "/openrtb2/bid", `{"id":"x"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
	if len(b.NoBids.Snapshot()) != 0 {
		t.Errorf("counter = %v, want empty", b.NoBids.Snapshot())
	}
}

func TestMethodNotAllowed(t *testing.T) {
	_, mux := newBidder(t)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/openrtb2/bid", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestBodyTooLarge(t *testing.T) {
	_, mux := newBidder(t)
	rec := post(mux, "/openrtb2/bid", `{"id":"`+strings.Repeat("x", maxBodyBytes)+`"}`)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", rec.Code)
	}
}

func TestDebugNoBidEndpoint(t *testing.T) {
	_, mux := newBidder(t, camp("a", "paused", 2_000_000, ""))
	post(mux, "/openrtb2/bid", okRequest)
	post(mux, "/openrtb2/bid", okRequest)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/debug/nobid", nil))
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"inactive":2}` {
		t.Errorf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
