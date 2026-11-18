// Command bidder starts the MiniDSP bidder HTTP server.
//
// Environment:
//
//	BIDDER_ADDR       listen address (default 127.0.0.1:8080)
//	BIDDER_CAMPAIGNS  path to the static campaigns JSON (default testdata/campaigns.json,
//	                  relative to the working directory; run from the repo root)
package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/therendertree/minidsp/bidder/internal/campaign"
	"github.com/therendertree/minidsp/bidder/internal/server"
)

func main() {
	addr := getenv("BIDDER_ADDR", "127.0.0.1:8080")
	campaignsPath := getenv("BIDDER_CAMPAIGNS", "testdata/campaigns.json")

	campaigns, err := campaign.Load(campaignsPath)
	if err != nil {
		log.Fatalf("load campaigns: %v", err) // fail fast on bad config
	}
	log.Printf("loaded %d campaign(s) from %s", len(campaigns), campaignsPath)

	srv := &http.Server{
		Addr:              addr,
		Handler:           server.NewMux(&server.Bidder{Campaigns: campaigns}),
		ReadHeaderTimeout: 2 * time.Second,
	}
	log.Printf("bidder listening on %s", addr)
	log.Fatal(srv.ListenAndServe())
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
