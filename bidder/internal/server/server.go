// Package server wires up the bidder's HTTP routes.
package server

import (
	"net/http"
	"time"

	"github.com/therendertree/minidsp/bidder/internal/bidding"
	"github.com/therendertree/minidsp/bidder/internal/campaign"
)

// Bidder holds what the handlers need. Campaigns are read-only after startup,
// so concurrent requests share them without locking.
type Bidder struct {
	Campaigns []campaign.Campaign
	Now       func() time.Time // injectable clock; nil means time.Now
	NoBids    *bidding.Counter // nil means a fresh counter
}

// NewMux returns the HTTP handler for the bidder.
func NewMux(b *Bidder) *http.ServeMux {
	if b.Now == nil {
		b.Now = time.Now
	}
	if b.NoBids == nil {
		b.NoBids = bidding.NewCounter()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthz)
	mux.HandleFunc("POST /openrtb2/bid", b.handleBid) // other methods get 405 from the mux
	mux.HandleFunc("GET /debug/nobid", b.handleNoBidStats)
	return mux
}

func healthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
