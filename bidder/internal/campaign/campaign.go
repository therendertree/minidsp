// Package campaign defines campaigns and loads them from a static JSON file.
// Milestone 1 only: later milestones move this to PostgreSQL.
package campaign

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/therendertree/minidsp/bidder/internal/money"
)

type Status string

const (
	StatusActive Status = "active"
	StatusPaused Status = "paused"
	StatusEnded  Status = "ended"
)

// Creative is one banner the campaign can serve.
type Creative struct {
	CRID string `json:"crid"`
	W    int    `json:"w"`
	H    int    `json:"h"`
}

// Campaign is one advertiser campaign with a fixed CPM.
//
//   - Start is inclusive, End is exclusive; either may be omitted (open-ended).
//   - Empty Geo / OS means "no targeting restriction on that dimension".
//   - CPMMicros is micro-USD per 1000 impressions (an integer: never a float).
type Campaign struct {
	ID        string       `json:"id"`
	Status    Status       `json:"status"`
	Start     *time.Time   `json:"start,omitempty"`
	End       *time.Time   `json:"end,omitempty"`
	Geo       []string     `json:"geo,omitempty"` // ISO-3166-1 alpha-3, stored upper-case
	OS        []string     `json:"os,omitempty"`  // stored lower-case
	CPMMicros money.Micros `json:"cpm_micros"`
	Creatives []Creative   `json:"creatives"`
}

// CreativeFor returns the first creative whose size is exactly w x h.
func (c *Campaign) CreativeFor(w, h int) (Creative, bool) {
	for _, cr := range c.Creatives {
		if cr.W == w && cr.H == h {
			return cr, true
		}
	}
	return Creative{}, false
}

type file struct {
	Campaigns []Campaign `json:"campaigns"`
}

// Load reads and validates the campaign file at path.
func Load(path string) ([]Campaign, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read campaigns: %w", err)
	}
	cs, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return cs, nil
}

// Parse validates campaign JSON of the form {"campaigns": [...]}.
// Unknown fields are rejected: a typo in config must not silently disable
// targeting. An empty list is allowed (the bidder then no-bids with
// no_campaigns).
func Parse(data []byte) ([]Campaign, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var f file
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	seen := make(map[string]bool, len(f.Campaigns))
	for i := range f.Campaigns {
		c := &f.Campaigns[i]
		if err := c.normalizeAndValidate(); err != nil {
			return nil, fmt.Errorf("campaign[%d] (%q): %w", i, c.ID, err)
		}
		if seen[c.ID] {
			return nil, fmt.Errorf("campaign[%d]: duplicate id %q", i, c.ID)
		}
		seen[c.ID] = true
	}
	return f.Campaigns, nil
}

func (c *Campaign) normalizeAndValidate() error {
	if c.ID == "" {
		return fmt.Errorf("id is required")
	}
	switch c.Status {
	case StatusActive, StatusPaused, StatusEnded:
	default:
		return fmt.Errorf("status must be active, paused or ended, got %q", c.Status)
	}
	if c.CPMMicros <= 0 {
		return fmt.Errorf("cpm_micros must be > 0")
	}
	if c.Start != nil && c.End != nil && !c.Start.Before(*c.End) {
		return fmt.Errorf("start must be before end")
	}
	if len(c.Creatives) == 0 {
		return fmt.Errorf("at least one creative is required")
	}
	for _, cr := range c.Creatives {
		if cr.CRID == "" || cr.W <= 0 || cr.H <= 0 {
			return fmt.Errorf("creative needs crid and w,h > 0: %+v", cr)
		}
	}
	for i, g := range c.Geo {
		if g == "" {
			return fmt.Errorf("geo entries must be non-empty")
		}
		c.Geo[i] = strings.ToUpper(g)
	}
	for i, o := range c.OS {
		if o == "" {
			return fmt.Errorf("os entries must be non-empty")
		}
		c.OS[i] = strings.ToLower(o)
	}
	return nil
}
