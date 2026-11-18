# OpenRTB 2.6 Subset Supported by MiniDSP

MiniDSP is a **simulation**. It speaks a deliberately small subset of OpenRTB 2.6
(reference: <https://github.com/InteractiveAdvertisingBureau/openrtb2.x/blob/main/2.6.md>).
Anything not listed here is **not supported**. Where this document says "verify", the
behavior was written from memory of the spec and must be checked against 2.6 before
being relied on or quoted in an interview.

Documented simplifications (not universal RTB behavior):

- One banner impression per request. USD only. First-price auction.
- The bidder never connects to a real exchange.

## Endpoint contract

`POST /openrtb2/bid`, `Content-Type: application/json`.

| Outcome | HTTP status | Body |
|---|---|---|
| Bid | `200` | `BidResponse` JSON (below) |
| No bid (valid request, nothing eligible / floor not met) | `204` | empty |
| Malformed or unsupported request | `400` | MiniDSP error JSON (below, not part of OpenRTB) |

Also: non-POST requests get `405`; bodies over 64 KiB get `413` (error code `body_too_large`).
On every `204` the bidder sets the non-OpenRTB header `X-Nobid-Reason` to one of:
`no_campaigns`, `inactive`, `outside_schedule`, `geo_mismatch`, `os_mismatch`, `size_mismatch`,
`floor_above_bid`. Counts are exposed at `GET /debug/nobid` (in-memory, local simulation only).

`204` for no-bid is permitted by OpenRTB 2.6 (verify exact wording). The `400` body is **our own
format**; OpenRTB does not define one.

## General parsing rules

- **Unknown fields are ignored**, never rejected (forward compatibility; e.g. `at`, `regs`, `ext`).
- **Type mismatches on supported fields are rejected** (`id: 12345` is a 400, not coerced).
- **Money is never parsed as a float.** `bidfloor` and `price` are read as decimal text and
  converted to integer micro-USD (`1 USD = 1_000_000 micro-USD`). OpenRTB prices are **CPM**
  (per 1000 impressions); per-impression cost = CPM / 1000, converted explicitly elsewhere.
- Decimal precision: more than 6 fractional digits in `bidfloor` is rounded **up** to the next
  micro-USD (conservative: never bid below the true floor). Emitted `price` has at most 6 fractional digits.
- Validation order is the order of the tables below; the **first** failure is reported.

## BidRequest — supported fields

| Field | Type | Req? | Validation rule | Failure |
|---|---|---|---|---|
| `id` | string | yes | non-empty | `missing_field` / `invalid_type` |
| `imp` | array of Imp | yes | **exactly 1** element (simplification; OpenRTB allows ≥1) | `invalid_imp_count` |
| `imp[0].id` | string | yes | non-empty | `missing_field` |
| `imp[0].banner` | object | yes | present (we support banner only; video/native/audio unsupported) | `missing_field` |
| `imp[0].banner.w` | integer | yes | > 0 | `invalid_value` |
| `imp[0].banner.h` | integer | yes | > 0 | `invalid_value` |
| `imp[0].bidfloor` | number (CPM) | no, default `0` | ≥ 0 | `invalid_value` |
| `imp[0].bidfloorcur` | string | no, default `"USD"` | must equal `"USD"` (case-insensitive; verify) | `unsupported_currency` |
| `tmax` | integer (ms) | no | if present, > 0 | `invalid_value` |
| `site` | object | one of site/app | exactly one of `site` / `app` must be present | `invalid_inventory` |
| `site.id`, `site.domain`, `site.page` | string | no | strings if present; no further validation | `invalid_type` |
| `app` | object | one of site/app | see above | `invalid_inventory` |
| `app.id`, `app.bundle` | string | no | strings if present | `invalid_type` |
| `device` | object | no | object if present | `invalid_type` |
| `device.ua`, `device.ip`, `device.os` | string | no | strings if present; `ip` format not validated | `invalid_type` |
| `device.devicetype` | integer | no | integer if present; value not range-checked (verify the 2.6 device-type list before targeting on it) | `invalid_type` |
| `device.geo.country` | string | no | string if present; OpenRTB uses ISO-3166-1 alpha-3, e.g. `"USA"` (verify) | `invalid_type` |

Extra fields beyond your original list, and why: `bidfloorcur` (needed to enforce USD-only),
`banner.w/h` (needed for creative-size matching), `device.geo.country` (candidate targeting
attribute), `site.*`/`app.*` sub-fields and `device.*` sub-fields (the concrete shape of "site/app" and "device").

### `tmax` semantics

`tmax` is the exchange's timeout in ms. In this project **the exchange simulator enforces it**:
it discards a response that arrives late (see `docs/plan.md` test matrix). The bidder only
validates the value and may use it for logging/metrics. A request is **not** rejected for having a small `tmax`.

### Why 400 vs 204

- **400**: the request is not valid under this subset (malformed JSON, missing/ill-typed field,
  unsupported feature such as multiple impressions or non-USD floor).
- **204**: the request is valid but we choose not to bid (floor above our bid, no creative of the requested size, no eligible campaign).

## BidResponse — fields emitted

| Field | Type | OpenRTB required? | Value |
|---|---|---|---|
| `id` | string | yes | echoes `BidRequest.id` |
| `cur` | string | no | always `"USD"` |
| `seatbid` | array | yes (for a bid) | exactly 1 `SeatBid` |
| `seatbid[0].seat` | string | no | `"minidsp"` |
| `seatbid[0].bid` | array | yes | exactly 1 `Bid` |
| `bid[0].id` | string | yes | `"<request.id>-<imp.id>"` — deterministic, so an exchange retry of the same request yields the same bid ID (this becomes the budget-reservation key; design choice to defend) |
| `bid[0].impid` | string | yes | echoes `imp[0].id` |
| `bid[0].price` | number (CPM, USD) | yes | ≥ `bidfloor` (equality allowed); emitted from integer micro-USD |
| `bid[0].crid` | string | recommended | creative ID of the chosen creative |

**Not emitted** (yet): `adm`, `nurl`, `burl`, `lurl`, `adomain`, `w`, `h`, `nbr`. A real bid
would carry `adm` or a notice URL; the simulator works from the response without rendering a creative.
Win/loss/billing notices are simulated out-of-band in later milestones, not via these URLs.

## Error body (400 only, MiniDSP-specific)

```json
{"error": {"code": "missing_field", "field": "imp[0].id", "message": "human readable text"}}
```

`code` and `field` are the contract and are asserted by golden tests; `message` is free text
and **ignored** by tests. `field` uses a JSON-path-like notation and is `""` when not applicable (e.g. `malformed_json`).

Codes: `malformed_json`, `missing_field`, `invalid_type`, `invalid_value`, `invalid_imp_count`,
`invalid_inventory`, `unsupported_currency`.

## Golden fixtures

See `testdata/README.md`. Every row above that can fail has at least one fixture in `testdata/cases.json`.
