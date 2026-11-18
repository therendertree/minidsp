package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/therendertree/minidsp/bidder/internal/campaign"
)

// Contract tests replay the golden fixtures in <repo>/testdata through the
// real handler. Comparison rules are documented in testdata/README.md.

const testdataDir = "../../../testdata"

type manifest struct {
	Cases []struct {
		Name             string `json:"name"`
		Request          string `json:"request"`
		ExpectStatus     int    `json:"expect_status"`
		ExpectResponse   string `json:"expect_response"`
		ExpectNoBidCause string `json:"expect_nobid_reason"`
	} `json:"cases"`
}

func readFile(t *testing.T, rel string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(testdataDir, rel))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func decodeAny(t *testing.T, data []byte) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("invalid JSON %q: %v", data, err)
	}
	return v
}

func TestGoldenFixtures(t *testing.T) {
	campaigns, err := campaign.Load(filepath.Join(testdataDir, "campaigns.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m manifest
	if err := json.Unmarshal(readFile(t, "cases.json"), &m); err != nil {
		t.Fatal(err)
	}
	if len(m.Cases) == 0 {
		t.Fatal("manifest has no cases")
	}

	for _, tc := range m.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			// Fresh bidder per case so the counter check is exact.
			mux := NewMux(&Bidder{Campaigns: campaigns})
			req := httptest.NewRequest(http.MethodPost, "/openrtb2/bid",
				strings.NewReader(string(readFile(t, tc.Request))))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != tc.ExpectStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.ExpectStatus, rec.Body.String())
			}

			switch tc.ExpectStatus {
			case http.StatusOK:
				if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
					t.Errorf("Content-Type = %q", ct)
				}
				got := decodeAny(t, rec.Body.Bytes())
				want := decodeAny(t, readFile(t, tc.ExpectResponse))
				if !reflect.DeepEqual(got, want) {
					t.Errorf("response mismatch\n got: %s\nwant: %s", rec.Body.String(), readFile(t, tc.ExpectResponse))
				}
			case http.StatusNoContent:
				if rec.Body.Len() != 0 {
					t.Errorf("204 body must be empty, got %q", rec.Body.String())
				}
				if got := rec.Header().Get(NoBidReasonHeader); got != tc.ExpectNoBidCause {
					t.Errorf("%s = %q, want %q", NoBidReasonHeader, got, tc.ExpectNoBidCause)
				}
			case http.StatusBadRequest:
				// Only code and field are contract; message is free text.
				pick := func(v any) [2]any {
					e := v.(map[string]any)["error"].(map[string]any)
					return [2]any{e["code"], e["field"]}
				}
				got := pick(decodeAny(t, rec.Body.Bytes()))
				want := pick(decodeAny(t, readFile(t, tc.ExpectResponse)))
				if got != want {
					t.Errorf("error (code, field) = %v, want %v", got, want)
				}
			}
		})
	}
}
