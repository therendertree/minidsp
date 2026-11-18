package bidding

import (
	"sync"
	"testing"
	"time"

	"github.com/therendertree/minidsp/bidder/internal/campaign"
	"github.com/therendertree/minidsp/bidder/internal/money"
	"github.com/therendertree/minidsp/bidder/internal/openrtb"
)

var now = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

func ptr(t time.Time) *time.Time { return &t }

// base returns a campaign that passes every filter for baseReq.
func base(id string, cpm money.Micros) campaign.Campaign {
	return campaign.Campaign{
		ID: id, Status: campaign.StatusActive, CPMMicros: cpm,
		Creatives: []campaign.Creative{{CRID: "cr-" + id, W: 300, H: 250}},
	}
}

func baseReq() *openrtb.Request {
	return &openrtb.Request{
		ID: "r", ImpID: "1", BannerW: 300, BannerH: 250, BidFloor: 1_000_000,
		Device: openrtb.Device{OS: "Android", Country: "USA"},
	}
}

func TestFilterRankMatchesDocumentedOrder(t *testing.T) {
	want := []Reason{ReasonInactive, ReasonOutsideSchedule, ReasonGeoMismatch,
		ReasonOSMismatch, ReasonSizeMismatch, ReasonFloorAboveBid}
	if len(filters) != len(want) {
		t.Fatalf("len(filters) = %d, want %d", len(filters), len(want))
	}
	for i, r := range want {
		if filters[i].reason != r {
			t.Errorf("filters[%d].reason = %s, want %s", i, filters[i].reason, r)
		}
	}
	for _, f := range filters {
		if f.reason == ReasonNoCampaigns {
			t.Error("no_campaigns must not be a filter reason")
		}
	}
}

// Each case breaks exactly one filter, so the expected reason does not depend
// on filter order.
func TestEachFilterInIsolation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(c *campaign.Campaign, r *openrtb.Request)
		reason Reason // "" = eligible
	}{
		{"eligible baseline", func(c *campaign.Campaign, r *openrtb.Request) {}, ""},
		{"paused", func(c *campaign.Campaign, r *openrtb.Request) { c.Status = campaign.StatusPaused }, ReasonInactive},
		{"ended", func(c *campaign.Campaign, r *openrtb.Request) { c.Status = campaign.StatusEnded }, ReasonInactive},
		{"before start", func(c *campaign.Campaign, r *openrtb.Request) { c.Start = ptr(now.Add(time.Hour)) }, ReasonOutsideSchedule},
		{"after end", func(c *campaign.Campaign, r *openrtb.Request) { c.End = ptr(now.Add(-time.Hour)) }, ReasonOutsideSchedule},
		{"start inclusive", func(c *campaign.Campaign, r *openrtb.Request) { c.Start = ptr(now) }, ""},
		{"end exclusive", func(c *campaign.Campaign, r *openrtb.Request) { c.End = ptr(now) }, ReasonOutsideSchedule},
		{"geo match", func(c *campaign.Campaign, r *openrtb.Request) { c.Geo = []string{"CAN", "USA"} }, ""},
		{"geo case-insensitive", func(c *campaign.Campaign, r *openrtb.Request) { r.Device.Country = "usa"; c.Geo = []string{"USA"} }, ""},
		{"geo mismatch", func(c *campaign.Campaign, r *openrtb.Request) { c.Geo = []string{"CAN"} }, ReasonGeoMismatch},
		{"geo targeted, request has none", func(c *campaign.Campaign, r *openrtb.Request) { c.Geo = []string{"USA"}; r.Device.Country = "" }, ReasonGeoMismatch},
		{"os match", func(c *campaign.Campaign, r *openrtb.Request) { c.OS = []string{"android"} }, ""},
		{"os mismatch", func(c *campaign.Campaign, r *openrtb.Request) { c.OS = []string{"ios"} }, ReasonOSMismatch},
		{"os targeted, request has none", func(c *campaign.Campaign, r *openrtb.Request) { c.OS = []string{"ios"}; r.Device.OS = "" }, ReasonOSMismatch},
		{"size mismatch", func(c *campaign.Campaign, r *openrtb.Request) { r.BannerW, r.BannerH = 728, 90 }, ReasonSizeMismatch},
		{"floor above bid", func(c *campaign.Campaign, r *openrtb.Request) { r.BidFloor = c.CPMMicros + 1 }, ReasonFloorAboveBid},
		{"floor equals bid", func(c *campaign.Campaign, r *openrtb.Request) { r.BidFloor = c.CPMMicros }, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, r := base("c1", 2_500_000), baseReq()
			tc.mutate(&c, r)
			d := Decide([]campaign.Campaign{c}, r, now)
			if tc.reason == "" {
				if d.Bid == nil {
					t.Fatalf("want bid, got no-bid %s", d.Reason)
				}
				return
			}
			if d.Bid != nil || d.Reason != tc.reason {
				t.Fatalf("got bid=%v reason=%s, want reason %s", d.Bid, d.Reason, tc.reason)
			}
			if len(d.Rejections) != 1 || d.Rejections[0].Reason != tc.reason {
				t.Errorf("rejections = %+v", d.Rejections)
			}
		})
	}
}

func TestNoCampaigns(t *testing.T) {
	d := Decide(nil, baseReq(), now)
	if d.Bid != nil || d.Reason != ReasonNoCampaigns {
		t.Fatalf("got %+v", d)
	}
}

func TestBestCPMWins(t *testing.T) {
	cs := []campaign.Campaign{base("a", 2_000_000), base("b", 3_000_000), base("c", 2_500_000)}
	d := Decide(cs, baseReq(), now)
	if d.Bid == nil || d.Bid.CampaignID != "b" || d.Bid.CPMMicros != 3_000_000 || d.Bid.CRID != "cr-b" {
		t.Fatalf("got %+v", d.Bid)
	}
}

func TestIneligibleHigherCPMDoesNotWin(t *testing.T) {
	hi := base("hi", 9_000_000)
	hi.Status = campaign.StatusPaused
	d := Decide([]campaign.Campaign{hi, base("lo", 2_000_000)}, baseReq(), now)
	if d.Bid == nil || d.Bid.CampaignID != "lo" {
		t.Fatalf("got %+v", d)
	}
	if len(d.Rejections) != 1 || d.Rejections[0].CampaignID != "hi" {
		t.Errorf("rejections = %+v", d.Rejections)
	}
}

func TestTieBreaksOnLowestID(t *testing.T) {
	for _, order := range [][]string{{"b", "a", "c"}, {"c", "b", "a"}, {"a", "b", "c"}} {
		var cs []campaign.Campaign
		for _, id := range order {
			cs = append(cs, base(id, 2_000_000))
		}
		if d := Decide(cs, baseReq(), now); d.Bid.CampaignID != "a" {
			t.Errorf("order %v: winner %s, want a", order, d.Bid.CampaignID)
		}
	}
}

func TestFurthestProgressedReasonWins(t *testing.T) {
	paused := base("paused", 2_000_000)
	paused.Status = campaign.StatusPaused // fails filter 0
	wrongSize := base("size", 2_000_000)
	wrongSize.Creatives = []campaign.Creative{{CRID: "x", W: 728, H: 90}} // fails size
	lowBid := base("low", 500_000)                                        // fails floor (floor is 1_000_000)

	r := baseReq()
	d := Decide([]campaign.Campaign{paused, wrongSize}, r, now)
	if d.Reason != ReasonSizeMismatch {
		t.Errorf("paused+size: reason = %s, want size_mismatch", d.Reason)
	}
	d = Decide([]campaign.Campaign{paused, wrongSize, lowBid}, r, now)
	if d.Reason != ReasonFloorAboveBid {
		t.Errorf("paused+size+low: reason = %s, want floor_above_bid", d.Reason)
	}
	if len(d.Rejections) != 3 {
		t.Errorf("rejections = %+v", d.Rejections)
	}
}

// A campaign failing several filters is attributed to the FIRST one.
func TestMultipleFailuresReportFirstFilter(t *testing.T) {
	c := base("c", 500_000)
	c.Geo = []string{"CAN"} // geo fails, floor also fails
	d := Decide([]campaign.Campaign{c}, baseReq(), now)
	if d.Reason != ReasonGeoMismatch {
		t.Errorf("reason = %s, want geo_mismatch", d.Reason)
	}
}

func TestCounter(t *testing.T) {
	c := NewCounter()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); c.Inc(ReasonGeoMismatch) }()
	}
	wg.Wait()
	c.Inc(ReasonInactive)
	s := c.Snapshot()
	if s[ReasonGeoMismatch] != 50 || s[ReasonInactive] != 1 {
		t.Errorf("snapshot = %v", s)
	}
	s[ReasonInactive] = 99 // mutating the copy must not affect the counter
	if c.Snapshot()[ReasonInactive] != 1 {
		t.Error("Snapshot is not a copy")
	}
}
