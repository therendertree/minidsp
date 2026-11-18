package openrtb

import "encoding/json"

// BidResponse is the subset of the OpenRTB 2.6 response we emit.
type BidResponse struct {
	ID      string    `json:"id"`
	SeatBid []SeatBid `json:"seatbid"`
	Cur     string    `json:"cur"`
}

type SeatBid struct {
	Seat string `json:"seat"`
	Bid  []Bid  `json:"bid"`
}

// Bid.Price is a json.Number so the decimal text we produce from integer
// micro-USD is written verbatim; it never goes through a float.
type Bid struct {
	ID    string      `json:"id"`
	ImpID string      `json:"impid"`
	Price json.Number `json:"price"`
	CRID  string      `json:"crid"`
}
