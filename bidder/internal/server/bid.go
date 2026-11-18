package server

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/therendertree/minidsp/bidder/internal/bidding"
	"github.com/therendertree/minidsp/bidder/internal/money"
	"github.com/therendertree/minidsp/bidder/internal/openrtb"
)

const (
	maxBodyBytes = 64 << 10 // 64 KiB: bid requests are small
	seatName     = "minidsp"

	// NoBidReasonHeader carries the internal no-bid reason on 204 responses.
	// Not part of OpenRTB; it lets the simulator and tests see why we passed.
	NoBidReasonHeader = "X-Nobid-Reason"
)

type errorBody struct {
	Error openrtb.ValidationError `json:"error"`
}

func (b *Bidder) handleBid(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, &openrtb.ValidationError{
				Code: "body_too_large", Message: "request body exceeds 64 KiB"})
			return
		}
		writeError(w, http.StatusBadRequest, &openrtb.ValidationError{
			Code: openrtb.CodeMalformedJSON, Message: "could not read body"})
		return
	}

	req, ve := openrtb.Parse(body)
	if ve != nil {
		writeError(w, http.StatusBadRequest, ve)
		return
	}

	d := bidding.Decide(b.Campaigns, req, b.Now())
	if d.Bid == nil {
		b.NoBids.Inc(d.Reason)
		w.Header().Set(NoBidReasonHeader, string(d.Reason))
		w.WriteHeader(http.StatusNoContent)
		return
	}

	resp := openrtb.BidResponse{
		ID:  req.ID,
		Cur: "USD",
		SeatBid: []openrtb.SeatBid{{
			Seat: seatName,
			Bid: []openrtb.Bid{{
				ID:    req.ID + "-" + req.ImpID, // deterministic: a retry yields the same bid ID
				ImpID: req.ImpID,
				Price: json.Number(money.FormatCPM(d.Bid.CPMMicros)),
				CRID:  d.Bid.CRID,
			}},
		}},
	}
	writeJSON(w, http.StatusOK, resp)
}

func (b *Bidder) handleNoBidStats(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, b.NoBids.Snapshot())
}

func writeError(w http.ResponseWriter, status int, ve *openrtb.ValidationError) {
	writeJSON(w, status, errorBody{Error: *ve})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("marshal response: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}
