package campaign

import (
	"strings"
	"testing"
)

func TestParseValid(t *testing.T) {
	cs, err := Parse([]byte(`{"campaigns":[{
	  "id":"c1","status":"active","cpm_micros":2500000,
	  "start":"2026-01-01T00:00:00Z","end":"2027-01-01T00:00:00Z",
	  "geo":["usa","CAN"],"os":["Android","iOS"],
	  "creatives":[{"crid":"cr1","w":300,"h":250}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	c := cs[0]
	if c.Geo[0] != "USA" || c.Geo[1] != "CAN" || c.OS[0] != "android" || c.OS[1] != "ios" {
		t.Errorf("normalization wrong: geo=%v os=%v", c.Geo, c.OS)
	}
	if _, ok := c.CreativeFor(300, 250); !ok {
		t.Error("creative 300x250 not found")
	}
	if _, ok := c.CreativeFor(728, 90); ok {
		t.Error("creative 728x90 should not match")
	}
}

func TestParseEmptyListAllowed(t *testing.T) {
	cs, err := Parse([]byte(`{"campaigns":[]}`))
	if err != nil || len(cs) != 0 {
		t.Fatalf("got %v, %v", cs, err)
	}
}

func TestParseInvalid(t *testing.T) {
	const cr = `"creatives":[{"crid":"x","w":1,"h":1}]`
	tests := []struct{ name, body, wantSub string }{
		{"bad json", `{`, "decode"},
		{"unknown field", `{"campaigns":[],"oops":1}`, "unknown field"},
		{"unknown campaign field", `{"campaigns":[{"id":"a","status":"active","cpm_micros":1,` + cr + `,"gro":["USA"]}]}`, "unknown field"},
		{"missing id", `{"campaigns":[{"status":"active","cpm_micros":1,` + cr + `}]}`, "id is required"},
		{"bad status", `{"campaigns":[{"id":"a","status":"on","cpm_micros":1,` + cr + `}]}`, "status"},
		{"zero cpm", `{"campaigns":[{"id":"a","status":"active","cpm_micros":0,` + cr + `}]}`, "cpm_micros"},
		{"float cpm", `{"campaigns":[{"id":"a","status":"active","cpm_micros":2.5,` + cr + `}]}`, "decode"},
		{"no creatives", `{"campaigns":[{"id":"a","status":"active","cpm_micros":1,"creatives":[]}]}`, "creative"},
		{"bad creative", `{"campaigns":[{"id":"a","status":"active","cpm_micros":1,"creatives":[{"crid":"","w":1,"h":1}]}]}`, "creative"},
		{"start after end", `{"campaigns":[{"id":"a","status":"active","cpm_micros":1,"start":"2027-01-01T00:00:00Z","end":"2026-01-01T00:00:00Z",` + cr + `}]}`, "before end"},
		{"duplicate id", `{"campaigns":[{"id":"a","status":"active","cpm_micros":1,` + cr + `},{"id":"a","status":"active","cpm_micros":1,` + cr + `}]}`, "duplicate"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.body))
			if err == nil || !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("err = %v, want containing %q", err, tc.wantSub)
			}
		})
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load("does-not-exist.json"); err == nil {
		t.Error("expected error")
	}
}
