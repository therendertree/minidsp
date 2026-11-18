package openrtb

import (
	"testing"

	"github.com/therendertree/minidsp/bidder/internal/money"
)

const validSite = `{
  "id": "r1",
  "imp": [{"id": "1", "banner": {"w": 300, "h": 250}, "bidfloor": 0.5}],
  "tmax": 120,
  "site": {"id": "s1", "domain": "a.example", "page": "https://a.example/x"},
  "device": {"ua": "UA", "ip": "203.0.113.1", "devicetype": 2, "os": "Linux", "geo": {"country": "USA"}}
}`

func TestParseValid(t *testing.T) {
	req, ve := Parse([]byte(validSite))
	if ve != nil {
		t.Fatalf("unexpected error: %v", ve)
	}
	if req.ID != "r1" || req.ImpID != "1" || req.BannerW != 300 || req.BannerH != 250 {
		t.Errorf("basic fields wrong: %+v", req)
	}
	if req.BidFloor != 500_000 {
		t.Errorf("BidFloor = %d, want 500000", req.BidFloor)
	}
	if req.TMaxMs != 120 || req.InventoryKind != "site" || req.InventoryID != "s1" {
		t.Errorf("tmax/inventory wrong: %+v", req)
	}
	if req.Device.OS != "Linux" || req.Device.Country != "USA" || req.Device.DeviceType != 2 {
		t.Errorf("device wrong: %+v", req.Device)
	}
}

func TestParseFloorRoundsUp(t *testing.T) {
	body := `{"id":"r","imp":[{"id":"1","banner":{"w":1,"h":1},"bidfloor":1.0000001}],"site":{}}`
	req, ve := Parse([]byte(body))
	if ve != nil {
		t.Fatal(ve)
	}
	if req.BidFloor != money.Micros(1_000_001) {
		t.Errorf("BidFloor = %d, want 1000001", req.BidFloor)
	}
}

func TestParseDefaultsAndNull(t *testing.T) {
	// null optional fields behave as absent; bidfloorcur may be lowercase usd.
	body := `{"id":"r","imp":[{"id":"1","banner":{"w":1,"h":1},"bidfloor":null,"bidfloorcur":"usd"}],"tmax":null,"app":{"id":"a"},"device":null}`
	req, ve := Parse([]byte(body))
	if ve != nil {
		t.Fatal(ve)
	}
	if req.BidFloor != 0 || req.TMaxMs != 0 || req.InventoryKind != "app" {
		t.Errorf("defaults wrong: %+v", req)
	}
}

// Rules not already covered by the golden fixtures.
func TestParseErrors(t *testing.T) {
	tests := []struct {
		name, body, code, field string
	}{
		{"empty body", ``, CodeMalformedJSON, ""},
		{"trailing garbage", `{"id":"r"} x`, CodeMalformedJSON, ""},
		{"top level array", `[]`, CodeInvalidType, ""},
		{"empty id", `{"id":""}`, CodeMissingField, "id"},
		{"imp missing", `{"id":"r"}`, CodeInvalidImpCount, "imp"},
		{"imp not array", `{"id":"r","imp":{}}`, CodeInvalidType, "imp"},
		{"imp[0] not object", `{"id":"r","imp":[1]}`, CodeInvalidType, "imp[0]"},
		{"banner not object", `{"id":"r","imp":[{"id":"1","banner":3}]}`, CodeInvalidType, "imp[0].banner"},
		{"w string", `{"id":"r","imp":[{"id":"1","banner":{"w":"300","h":1}}]}`, CodeInvalidType, "imp[0].banner.w"},
		{"w fractional", `{"id":"r","imp":[{"id":"1","banner":{"w":300.5,"h":1}}]}`, CodeInvalidValue, "imp[0].banner.w"},
		{"w zero", `{"id":"r","imp":[{"id":"1","banner":{"w":0,"h":1}}]}`, CodeInvalidValue, "imp[0].banner.w"},
		{"bidfloor string", `{"id":"r","imp":[{"id":"1","banner":{"w":1,"h":1},"bidfloor":"1"}]}`, CodeInvalidType, "imp[0].bidfloor"},
		{"bidfloorcur not string", `{"id":"r","imp":[{"id":"1","banner":{"w":1,"h":1},"bidfloorcur":1}]}`, CodeInvalidType, "imp[0].bidfloorcur"},
		{"tmax string", `{"id":"r","imp":[{"id":"1","banner":{"w":1,"h":1}}],"tmax":"5"}`, CodeInvalidType, "tmax"},
		{"tmax zero", `{"id":"r","imp":[{"id":"1","banner":{"w":1,"h":1}}],"tmax":0}`, CodeInvalidValue, "tmax"},
		{"site not object", `{"id":"r","imp":[{"id":"1","banner":{"w":1,"h":1}}],"site":"x"}`, CodeInvalidType, "site"},
		{"site.id number", `{"id":"r","imp":[{"id":"1","banner":{"w":1,"h":1}}],"site":{"id":5}}`, CodeInvalidType, "site.id"},
		{"app.bundle number", `{"id":"r","imp":[{"id":"1","banner":{"w":1,"h":1}}],"app":{"bundle":5}}`, CodeInvalidType, "app.bundle"},
		{"device not object", `{"id":"r","imp":[{"id":"1","banner":{"w":1,"h":1}}],"site":{},"device":[]}`, CodeInvalidType, "device"},
		{"devicetype string", `{"id":"r","imp":[{"id":"1","banner":{"w":1,"h":1}}],"site":{},"device":{"devicetype":"x"}}`, CodeInvalidType, "device.devicetype"},
		{"geo not object", `{"id":"r","imp":[{"id":"1","banner":{"w":1,"h":1}}],"site":{},"device":{"geo":1}}`, CodeInvalidType, "device.geo"},
		{"country number", `{"id":"r","imp":[{"id":"1","banner":{"w":1,"h":1}}],"site":{},"device":{"geo":{"country":1}}}`, CodeInvalidType, "device.geo.country"},
		{"order: id before imp", `{"imp":[]}`, CodeMissingField, "id"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, ve := Parse([]byte(tc.body))
			if ve == nil {
				t.Fatalf("expected error, got request %+v", req)
			}
			if ve.Code != tc.code || ve.Field != tc.field {
				t.Errorf("got (%s, %q), want (%s, %q): %s", ve.Code, ve.Field, tc.code, tc.field, ve.Message)
			}
		})
	}
}

func TestParseHugeNumberDoesNotPanic(t *testing.T) {
	body := `{"id":"r","imp":[{"id":"1","banner":{"w":1,"h":1},"bidfloor":1e999999}],"site":{}}`
	_, ve := Parse([]byte(body))
	if ve == nil || ve.Code != CodeInvalidValue {
		t.Errorf("want invalid_value, got %v", ve)
	}
}
