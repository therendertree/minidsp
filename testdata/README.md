# Golden protocol fixtures

Contract: see `docs/openrtb-subset.md`. `cases.json` is the manifest: each case names a
request file, the expected HTTP status, and (for 200/400) a golden response file.
No test runner exists yet; the Go contract tests and the Python simulator will both read this manifest.

## Assumed campaign (test fixture, not final bidder design)

Cases that expect a bid assume exactly one active campaign:

- fixed CPM **2.50 USD** (2_500_000 micro-USD per 1000 impressions)
- one creative: `crid = "cr-banner-300x250-001"`, banner **300x250**

So: floor ≤ 2.5 and size 300x250 → bid; floor > 2.5 or other size → 204.

## Comparison rules

- Compare JSON **semantically** (key order and whitespace ignored; `2.5` equals `2.50`).
- For 400 cases compare only `error.code` and `error.field`; ignore `error.message`.
- 204 cases have no response file; body must be empty, and the `X-Nobid-Reason` header must equal
  the case's `expect_nobid_reason`.
- The campaign set is `testdata/campaigns.json` (the same file the bidder loads by default).
- `requests/bad_malformed_json.txt` is deliberately invalid JSON (hence `.txt`).

## Cases

| Name | Status | What it covers |
|---|---|---|
| ok_site_banner | 200 | typical site request, floor 0.5 |
| ok_app_banner | 200 | app inventory, explicit `bidfloorcur: USD` |
| ok_floor_equals_bid | 200 | floor == bid is allowed |
| ok_unknown_fields_ignored | 200 | unknown fields tolerated |
| ok_no_optional_fields | 200 | only required fields; floor defaults to 0 |
| nobid_floor_above_bid | 204 | floor 3.0 > bid 2.5 |
| nobid_size_mismatch | 204 | 728x90 has no matching creative |
| bad_* | 400 | one per validation rule (see manifest) |

Not covered here: simulated `tmax` expiry (enforced by the exchange simulator, not the bidder).
