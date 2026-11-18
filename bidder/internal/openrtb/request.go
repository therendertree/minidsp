// Package openrtb parses and validates the OpenRTB 2.6 subset described in
// docs/openrtb-subset.md, and defines the response types the bidder emits.
//
// Parsing walks a generic JSON tree instead of decoding into structs, so we
// control exactly which error code and field path each failure produces.
package openrtb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/therendertree/minidsp/bidder/internal/money"
)

// Error codes returned in ValidationError.Code (see docs/openrtb-subset.md).
const (
	CodeMalformedJSON       = "malformed_json"
	CodeMissingField        = "missing_field"
	CodeInvalidType         = "invalid_type"
	CodeInvalidValue        = "invalid_value"
	CodeInvalidImpCount     = "invalid_imp_count"
	CodeInvalidInventory    = "invalid_inventory"
	CodeUnsupportedCurrency = "unsupported_currency"
)

// ValidationError describes the first rule a request broke.
// Code and Field are the contract; Message is free text.
type ValidationError struct {
	Code    string `json:"code"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s at %q: %s", e.Code, e.Field, e.Message)
}

// Request is a validated bid request in the form the rest of the bidder uses.
type Request struct {
	ID               string
	ImpID            string
	BannerW, BannerH int
	BidFloor         money.Micros // per-1000-impressions (CPM), micro-USD
	TMaxMs           int          // 0 = not supplied
	InventoryKind    string       // "site" or "app"
	InventoryID      string
	Device           Device
}

// Device holds the device fields we accept. Empty / zero means "not supplied".
type Device struct {
	UA, IP, OS string
	DeviceType int
	Country    string // ISO-3166-1 alpha-3 per OpenRTB (verify), as sent
}

type obj = map[string]any

// Parse validates data and returns the request, or the first ValidationError.
func Parse(data []byte) (*Request, *ValidationError) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber() // keep numbers as text: prices must never pass through float64
	var rootAny any
	if err := dec.Decode(&rootAny); err != nil {
		return nil, newErr(CodeMalformedJSON, "", "body is not valid JSON")
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, newErr(CodeMalformedJSON, "", "unexpected data after JSON value")
	}
	root, ok := rootAny.(obj)
	if !ok {
		return nil, badType("", "object")
	}

	req := &Request{}
	var ve *ValidationError

	if req.ID, ve = reqString(root, "id", "id"); ve != nil {
		return nil, ve
	}

	// imp: exactly one element.
	impV, ok := get(root, "imp")
	if !ok {
		return nil, newErr(CodeInvalidImpCount, "imp", "imp must contain exactly 1 element")
	}
	impArr, ok := impV.([]any)
	if !ok {
		return nil, badType("imp", "array")
	}
	if len(impArr) != 1 {
		return nil, newErr(CodeInvalidImpCount, "imp", "imp must contain exactly 1 element")
	}
	imp, ok := impArr[0].(obj)
	if !ok {
		return nil, badType("imp[0]", "object")
	}
	if req.ImpID, ve = reqString(imp, "id", "imp[0].id"); ve != nil {
		return nil, ve
	}

	// banner
	bannerV, ok := get(imp, "banner")
	if !ok {
		return nil, newErr(CodeMissingField, "imp[0].banner", "banner is required")
	}
	banner, ok := bannerV.(obj)
	if !ok {
		return nil, badType("imp[0].banner", "object")
	}
	if req.BannerW, ve = reqPositiveInt(banner, "w", "imp[0].banner.w"); ve != nil {
		return nil, ve
	}
	if req.BannerH, ve = reqPositiveInt(banner, "h", "imp[0].banner.h"); ve != nil {
		return nil, ve
	}

	// bidfloor (CPM, default 0) and bidfloorcur (default USD)
	if v, ok := get(imp, "bidfloor"); ok {
		num, isNum := v.(json.Number)
		if !isNum {
			return nil, badType("imp[0].bidfloor", "number")
		}
		m, err := money.ParseCPM(num.String())
		if err != nil {
			return nil, newErr(CodeInvalidValue, "imp[0].bidfloor", "must be a non-negative amount: "+err.Error())
		}
		req.BidFloor = m
	}
	if v, ok := get(imp, "bidfloorcur"); ok {
		cur, isStr := v.(string)
		if !isStr {
			return nil, badType("imp[0].bidfloorcur", "string")
		}
		if !strings.EqualFold(cur, "USD") {
			return nil, newErr(CodeUnsupportedCurrency, "imp[0].bidfloorcur", "only USD is supported")
		}
	}

	// tmax
	if v, ok := get(root, "tmax"); ok {
		n, ve := asInt(v, "tmax")
		if ve != nil {
			return nil, ve
		}
		if n <= 0 {
			return nil, newErr(CodeInvalidValue, "tmax", "must be > 0 when supplied")
		}
		req.TMaxMs = n
	}

	// site / app: exactly one
	siteV, hasSite := get(root, "site")
	appV, hasApp := get(root, "app")
	if hasSite == hasApp {
		return nil, newErr(CodeInvalidInventory, "site", "exactly one of site or app must be present")
	}
	if hasSite {
		site, ok := siteV.(obj)
		if !ok {
			return nil, badType("site", "object")
		}
		req.InventoryKind = "site"
		if req.InventoryID, ve = optString(site, "id", "site.id"); ve != nil {
			return nil, ve
		}
		for _, k := range []string{"domain", "page"} {
			if _, ve = optString(site, k, "site."+k); ve != nil {
				return nil, ve
			}
		}
	} else {
		app, ok := appV.(obj)
		if !ok {
			return nil, badType("app", "object")
		}
		req.InventoryKind = "app"
		if req.InventoryID, ve = optString(app, "id", "app.id"); ve != nil {
			return nil, ve
		}
		if _, ve = optString(app, "bundle", "app.bundle"); ve != nil {
			return nil, ve
		}
	}

	// device (optional)
	if v, ok := get(root, "device"); ok {
		dev, ok := v.(obj)
		if !ok {
			return nil, badType("device", "object")
		}
		if req.Device.UA, ve = optString(dev, "ua", "device.ua"); ve != nil {
			return nil, ve
		}
		if req.Device.IP, ve = optString(dev, "ip", "device.ip"); ve != nil {
			return nil, ve
		}
		if req.Device.OS, ve = optString(dev, "os", "device.os"); ve != nil {
			return nil, ve
		}
		if dt, ok := get(dev, "devicetype"); ok {
			if req.Device.DeviceType, ve = asInt(dt, "device.devicetype"); ve != nil {
				return nil, ve
			}
		}
		if gv, ok := get(dev, "geo"); ok {
			geo, ok := gv.(obj)
			if !ok {
				return nil, badType("device.geo", "object")
			}
			if req.Device.Country, ve = optString(geo, "country", "device.geo.country"); ve != nil {
				return nil, ve
			}
		}
	}

	return req, nil
}

// get returns the value at key; JSON null counts as absent.
func get(o obj, key string) (any, bool) {
	v, ok := o[key]
	if !ok || v == nil {
		return nil, false
	}
	return v, true
}

func reqString(o obj, key, path string) (string, *ValidationError) {
	v, ok := get(o, key)
	if !ok {
		return "", newErr(CodeMissingField, path, "required")
	}
	s, ok := v.(string)
	if !ok {
		return "", badType(path, "string")
	}
	if s == "" {
		return "", newErr(CodeMissingField, path, "must be non-empty")
	}
	return s, nil
}

func optString(o obj, key, path string) (string, *ValidationError) {
	v, ok := get(o, key)
	if !ok {
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", badType(path, "string")
	}
	return s, nil
}

// reqPositiveInt: a missing, non-integer or non-positive value is invalid_value;
// a value of the wrong JSON type is invalid_type.
func reqPositiveInt(o obj, key, path string) (int, *ValidationError) {
	v, ok := get(o, key)
	if !ok {
		return 0, newErr(CodeInvalidValue, path, "required and must be > 0")
	}
	n, ve := asInt(v, path)
	if ve != nil {
		return 0, ve
	}
	if n <= 0 {
		return 0, newErr(CodeInvalidValue, path, "must be > 0")
	}
	return n, nil
}

func asInt(v any, path string) (int, *ValidationError) {
	num, ok := v.(json.Number)
	if !ok {
		return 0, badType(path, "integer")
	}
	n, err := num.Int64()
	if err != nil || n != int64(int(n)) {
		return 0, newErr(CodeInvalidValue, path, "must be an integer")
	}
	return int(n), nil
}

func newErr(code, field, msg string) *ValidationError {
	return &ValidationError{Code: code, Field: field, Message: msg}
}

func badType(field, want string) *ValidationError {
	return newErr(CodeInvalidType, field, "must be a JSON "+want)
}
