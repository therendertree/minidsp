// Command bidder starts the MiniDSP bidder HTTP server.
// Skeleton only: it serves /healthz. The OpenRTB endpoint comes in Milestone 1.
package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/therendertree/minidsp/bidder/internal/server"
)

func main() {
	addr := os.Getenv("BIDDER_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           server.NewMux(),
		ReadHeaderTimeout: 2 * time.Second,
	}
	log.Printf("bidder listening on %s", addr)
	log.Fatal(srv.ListenAndServe())
}
