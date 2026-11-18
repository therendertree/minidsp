// Package bidding decides whether to bid on a request: it filters campaigns
// for eligibility, picks the best fixed-CPM campaign, and explains no-bids.
package bidding

import (
	"strings"
	"time"

	"github.com/therendertree/minidsp/bidder/internal/campaign"
	"github.com/therendertree/minidsp/bidder/internal/money"
	"github.com/therendertree/minidsp/bidder/internal/openrtb"
)

// Reason says why a campaign (or the whole request) did not result in a bid.
type Reason string

// ReasonNoCampaigns is request-level: the campaign list is empty, so no filter
// ever runs. It is deliberately NOT in the filters list below and never takes
// part in the "furthest progressed" ranking.
const ReasonNoCampaigns Reason = "no_campaigns"

// Per-campaign reasons, one per filter.
const (
	ReasonInactive        Reason = "inactive"         // status is not active
	ReasonOutsideSchedule Reason = "outside_schedule" // active, but now is outside [start, end)
	ReasonGeoMismatch     Reason = "geo_mismatch"
	ReasonOSMismatch      Reason = "os_mismatch"
	ReasonSizeMismatch    Reason = "size_mismatch"
	ReasonFloorAboveBid   Reason = "floor_above_bid"
)

// filter is one eligibility check. pass reports whether the campaign survives it.
type filter struct {
	reason Reason
	pass   func(c *campaign.Campaign, r *openrtb.Request, now time.Time) bool
}

// filters is run in this order, stopping at the first failure per campaign.
// The index of a filter is its "rank": a campaign failing a later filter got
// further than one failing an earlier filter. Reordering this slice changes
// which reason is recorded for a campaign failing several filters; it never
// changes which campaigns are eligible (see docs / tests).
var filters = []filter{
	{ReasonInactive, func(c *campaign.Campaign, _ *openrtb.Request, _ time.Time) bool {
		return c.Status == campaign.StatusActive
	}},
	{ReasonOutsideSchedule, func(c *campaign.Campaign, _ *openrtb.Request, now time.Time) bool {
		if c.Start != nil && now.Before(*c.Start) {
			return false
		}
		if c.End != nil && !now.Before(*c.End) {
			return false
		}
		return true
	}},
	{ReasonGeoMismatch, func(c *campaign.Campaign, r *openrtb.Request, _ time.Time) bool {
		return targetMatches(c.Geo, r.Device.Country)
	}},
	{ReasonOSMismatch, func(c *campaign.Campaign, r *openrtb.Request, _ time.Time) bool {
		return targetMatches(c.OS, r.Device.OS)
	}},
	{ReasonSizeMismatch, func(c *campaign.Campaign, r *openrtb.Request, _ time.Time) bool {
		_, ok := c.CreativeFor(r.BannerW, r.BannerH)
		return ok
	}},
	{ReasonFloorAboveBid, func(c *campaign.Campaign, r *openrtb.Request, _ time.Time) bool {
		return c.CPMMicros >= r.BidFloor // equality is allowed
	}},
}

// targetMatches: an empty target list means "no restriction"; otherwise the
// request value must be present and equal (case-insensitively) to one entry.
func targetMatches(targets []string, value string) bool {
	if len(targets) == 0 {
		return true
	}
	if value == "" {
		return false
	}
	for _, t := range targets {
		if strings.EqualFold(t, value) {
			return true
		}
	}
	return false
}

// Bid is the chosen campaign and creative.
type Bid struct {
	CampaignID string
	CRID       string
	CPMMicros  money.Micros
}

// Rejection records why one campaign was filtered out.
type Rejection struct {
	CampaignID string
	Reason     Reason
}

// Decision is the full outcome for one request. Exactly one of Bid != nil or
// Reason != "" is set. Rejections lists every filtered-out campaign.
type Decision struct {
	Bid        *Bid
	Reason     Reason
	Rejections []Rejection
}

// Decide evaluates every campaign serially against req at time now.
//
// A campaign is eligible if it passes all filters. Among eligible campaigns the
// highest CPM wins (first-price: we bid our own fixed CPM); ties go to the
// lexicographically smallest campaign ID so results are deterministic.
// With no eligible campaign, the request's reason is the reason of the
// campaign that got furthest through the filters.
func Decide(cs []campaign.Campaign, req *openrtb.Request, now time.Time) Decision {
	if len(cs) == 0 {
		return Decision{Reason: ReasonNoCampaigns}
	}

	var d Decision
	var best *campaign.Campaign
	furthest := -1

	for i := range cs {
		c := &cs[i]
		failed := firstFailure(c, req, now)
		if failed >= 0 {
			d.Rejections = append(d.Rejections, Rejection{CampaignID: c.ID, Reason: filters[failed].reason})
			if failed > furthest {
				furthest = failed
			}
			continue
		}
		if best == nil || c.CPMMicros > best.CPMMicros ||
			(c.CPMMicros == best.CPMMicros && c.ID < best.ID) {
			best = c
		}
	}

	if best == nil {
		d.Reason = filters[furthest].reason
		return d
	}
	cr, _ := best.CreativeFor(req.BannerW, req.BannerH) // size filter guarantees a match
	d.Bid = &Bid{CampaignID: best.ID, CRID: cr.CRID, CPMMicros: best.CPMMicros}
	return d
}

// firstFailure returns the index of the first filter c fails, or -1.
func firstFailure(c *campaign.Campaign, r *openrtb.Request, now time.Time) int {
	for i, f := range filters {
		if !f.pass(c, r, now) {
			return i
		}
	}
	return -1
}
