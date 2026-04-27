# PennyProphet — Design Spec

**Date:** 2026-04-27
**Status:** Approved
**Branch:** claude-schedule-mt → merge target: main (ClaudePennyProphet) + ClaudeProphet

---

## 1. Goal

Transform ClaudePennyProphet into a high-risk, high-reward autonomous penny stock trading robot. PennyProphet operates alongside the existing options-trading Prophet agent — same Go backend process, same MCP server, same dashboard. No existing code is deleted.

---

## 2. Tradeable Universe

| Constraint | Value |
|---|---|
| Price range | $2.00–$10.00/share |
| Market cap | $50M–$500M |
| Average daily volume | ≥ $300,000 dollar volume (avg_volume_30d × price ≥ $300K) |
| Exchange | Nasdaq CM, NYSE Arca, NYSE American (Amex) — exchange-listed only |
| Excluded | OTC, Pink Sheets, Grey Market |

Universe size: approximately 200–500 symbols depending on market conditions. Refreshed every 15 minutes during market hours via Alpaca screener endpoint.

---

## 3. Signal Pipeline Architecture

### 3.1 Overview

Three signal adapters run as independent background goroutines inside the Go binary. Each adapter can fail without affecting the others. Scores from all adapters feed into a central aggregator that maintains an in-memory candidate cache.

```
Go Binary (prophet_bot)
├── PennyUniverseService     → goroutine, 15-min refresh
├── PennyScreenerService     → goroutine, 60-second polling (technical)
├── SECEdgarService          → goroutine, 30-second polling (regulatory)
├── SocialSignalService      → goroutine, 30-second polling (social)
└── PennySignalAggregator    → in-memory cache, serves MCP tool reads
```

### 3.2 PennyUniverseService

- Calls Alpaca screener API with filters: price 2–10, market_cap 50M–500M, avg_volume_30d producing ≥ $300K/day
- Resolves exchange metadata; drops any symbol routing to OTC/Pink Sheet venues
- Publishes updated symbol list to a shared slice (mutex-protected) every 15 minutes
- Other services read from this list to scope their polling

### 3.3 PennyScreenerService (Technical Signals, 0–40 pts)

Polls every 60 seconds. For each universe symbol, computes:

```
volumeRatio     = currentVolume / avg20DayVolume
gapPct          = (currentPrice - prevClose) / prevClose * 100
breakoutBonus   = 1 if price within 2% of 52-week high, else 0

TechnicalScore  = min(volumeRatio/5, 1)*20 + min(gapPct/5, 1)*10 + breakoutBonus*10
```

Score range: 0–40. Uses Alpaca `GetLatestBar` and `GetHistoricalBars` (already in `AlpacaDataService`).

Decay: half-life 2 hours (`λ = ln(2)/2`).

### 3.4 SECEdgarService (Regulatory Signals, 0–40 pts)

Polls every 30 seconds. Three RSS sources:

| Source | URL | Event types scored |
|---|---|---|
| SEC EDGAR full-text search | `https://efts.sec.gov/LATEST/search-index?q=%22{ticker}%22&dateRange=custom&startdt={today}&forms=8-K` | 8-K filings |
| Globe Newswire RSS | `https://www.globenewswire.com/RssFeed/company/{ticker}` | Press releases |
| PR Newswire (public feed) | `https://www.prnewswire.com/rss/news-releases-list.rss` (filtered by ticker mention) | Press releases |

Base point values:

| Event | Base pts |
|---|---|
| 8-K filed today | 40 |
| PR wire (Globe/BusinessWire) | 25 |
| S-1 or secondary offering filed | −10 (dilution penalty) |

Decay: half-life 24 hours (`λ = ln(2)/24`). Score resets to base on each new filing.

### 3.5 SocialSignalService (Social Signals, 0–20 pts)

Polls every 30 seconds. Two sources:

| Source | Method | Notes |
|---|---|---|
| Reddit r/pennystocks + r/RobinHoodPennyStocks | Public JSON API (`/r/pennystocks/new.json`) | No auth required for read-only |
| StockTwits | Public API `https://api.stocktwits.com/api/2/streams/symbol/{ticker}.json` | Rate limit: 200 req/hour unauthenticated |

Scoring:

```
mentionVelocityPts = min(mentionsLast30min / avgMentionsPer30min, 2.0) * 5   → max 10 pts
sentimentPts       = if bullishRatio > 0.65 → 10; if > 0.55 → 5; else 0      → max 10 pts

SocialScore = mentionVelocityPts + sentimentPts
```

Decay: half-life 4 hours (`λ = ln(2)/4`).

Twitter/X: excluded from v1 (API costs ~$100/month Basic tier). Adapter interface is defined so it can be added later without touching the aggregator.

### 3.6 PennySignalAggregator

Combines the three sub-scores:

```
CompositeScore = TechnicalScore + RegulatoryScore + SocialScore   (max 100)
```

Maintains `map[string]CandidateScore` in memory. `CandidateScore` struct:

```go
type CandidateScore struct {
    Ticker           string
    CompositeScore   float64
    TechnicalScore   float64
    RegulatoryScore  float64
    SocialScore      float64
    DominantSignal   string    // "technical" | "regulatory" | "social"
    LastUpdated      time.Time
    RegulatoryEvent  string    // e.g. "8-K filed 09:32 ET"
    SocialContext    string    // e.g. "3.2x mention velocity, 71% bullish"
}
```

`DominantSignal` is set to whichever sub-score is highest, normalized by its maximum. This drives the exit-rule selection in the agent's strategy rules.

---

## 4. MCP Tools (4 new)

Added to `mcp-server.js`. All read from the in-memory aggregator cache — no blocking I/O.

| Tool | Input | Output |
|---|---|---|
| `get_penny_candidates` | `min_score` (optional, default 60) | Array of `CandidateScore` above threshold, sorted by composite score desc |
| `get_penny_signal_detail` | `ticker` (required) | Full `CandidateScore` for one symbol including raw inputs, decay timestamps |
| `get_penny_universe` | none | Current universe symbol list with price/mcap/adv metadata |
| `scan_penny_universe_now` | none | Triggers immediate out-of-cycle universe refresh; returns `{status: "refreshing"}` |

HTTP endpoints added to Go backend:

```
GET  /api/penny/candidates?min_score=60
GET  /api/penny/signal/:ticker
GET  /api/penny/universe
POST /api/penny/scan
```

---

## 5. Go Backend File Changes

### New files

| File | Purpose |
|---|---|
| `services/penny_universe_service.go` | Universe cache + 15-min refresh goroutine |
| `services/penny_screener_service.go` | Technical signal computation (Alpaca data) |
| `services/sec_edgar_service.go` | EDGAR + PR wire RSS polling |
| `services/social_signal_service.go` | Reddit + StockTwits polling |
| `services/penny_signal_aggregator.go` | Composite score cache + decay engine |
| `controllers/penny_controller.go` | HTTP handlers for 4 endpoints |

### Modified files

| File | Change |
|---|---|
| `cmd/bot/main.go` | Register 4 new routes; start 5 new goroutines on startup |
| `interfaces/interfaces.go` | Add `CandidateScore`, `PennySignal`, `UniverseSymbol` types |

---

## 6. MCP Server Changes

**File:** `mcp-server.js`

Additions only:
- 4 new tool definitions in the tools array
- 4 new cases in the tool-call switch block
- Each calls the corresponding Go endpoint on `localhost:4534`

No existing tools modified.

---

## 7. Agent Configuration

### New agent persona: PennyProphet

Added to `data/agent-config.json` `agents` array:

```json
{
  "id": "penny-prophet",
  "name": "PennyProphet",
  "description": "High-risk penny stock momentum trader, exchange-listed $2–$10",
  "systemPromptTemplate": "custom",
  "strategyId": "penny-momentum",
  "model": "anthropic/claude-sonnet-4-6",
  "heartbeatOverrides": {
    "pre_market": 900,
    "market_open": 60,
    "midday": 180,
    "market_close": 60,
    "after_hours": 3600,
    "closed": 10800
  }
}
```

Tighter heartbeat during market hours (60s open/close, 180s midday) reflects the shorter momentum windows in penny stocks vs. options.

### New strategy: Penny Stock Momentum

```json
{
  "id": "penny-momentum",
  "name": "Penny Stock Momentum",
  "description": "Multi-signal penny stock strategy: social, regulatory, technical",
  "rulesFile": "TRADING_RULES_PENNY.md",
  "customRules": null
}
```

### Permission overrides for PennyProphet sandbox

```json
{
  "allowLiveTrading": true,
  "allowStocks": true,
  "allowOptions": false,
  "allow0DTE": false,
  "maxPositionPct": 8,
  "maxDeployedPct": 70,
  "maxDailyLoss": 5,
  "maxOpenPositions": 12,
  "maxOrderValue": 0,
  "maxToolRoundsPerBeat": 30
}
```

---

## 8. TRADING_RULES_PENNY.md

New file at project root. Key rules:

### Universe & Asset Class
- Exchange-listed stocks only: Nasdaq CM, NYSE Arca, NYSE American
- Price: $2.00–$10.00; Market cap: $50M–$500M; ADV: ≥ $300K
- No options. No OTC. No Pink Sheets.

### Signal-Gated Entry
- Call `get_penny_candidates(min_score=60)` on each heartbeat
- Only enter positions on candidates with composite score ≥ 60
- Read `get_penny_signal_detail(ticker)` before sizing to confirm `dominantSignal`

### Position Sizing (Tiered by Composite Score)

| Composite Score | Position Size | Note |
|---|---|---|
| 80–100 | 5–7% of portfolio | Strong multi-signal confluence |
| 60–79 | 2–3% of portfolio | Partial or single strong signal |
| < 60 | No trade | Watchlist only |
| Any | Hard cap 8% max | Absolute ceiling |

### Signal-Type Exit Rules

| Dominant Signal | Hold Mode | Stop | Target |
|---|---|---|---|
| `social` | Day-trade only, no overnight | −8% | +15–20%, scale out in halves |
| `regulatory` | Up to 3 days | −10% | +20% day 1, trail from day 2 |
| `technical` | Hold until stop or 2R, adaptive | −7% (below breakout base) | +14–20% or trailing stop |

### Bracket Order Requirement
- All entries use `place_managed_position` with stop and target pre-set
- Verify Alpaca supports bracket orders on the specific symbol before entry
- If bracket orders are rejected for a symbol, skip that trade — do not enter without automated stop

### Daily Circuit Breaker
- If portfolio P&L ≤ −5% intraday: close all penny positions, cease new entries for the session
- Log circuit breaker trigger via `log_decision`

### Heartbeat Behavior
1. Call `get_penny_candidates(min_score=60)` — check for new opportunities
2. Call `get_positions` — review open penny positions against exit rules
3. Call `get_account` — confirm within daily loss limit before any new entry
4. Act: enter, manage, or exit based on rules above
5. Log via `log_activity`

---

## 9. Broker Verification Checklist (Day-1 Gate)

Before enabling live trading, verify in paper trading:

- [ ] `place_managed_position` with bracket (stop + target) succeeds on a $2–$10 exchange-listed symbol
- [ ] Alpaca does not restrict bracket orders on low-priced exchange-listed names
- [ ] `place_sell_order` (manual close) works as fallback if bracket is rejected
- [ ] StockTwits unauthenticated rate limit (200 req/hr) is sufficient for universe size
- [ ] Reddit public JSON API is accessible without auth in production environment

---

## 10. ClaudeProphet Integration Guide

All changes are net-additive. To apply to the ClaudeProphet repository:

1. **Copy new Go service files** — `services/penny_*.go`, `services/sec_edgar_service.go`, `services/social_signal_service.go`, `controllers/penny_controller.go`
2. **Copy `TRADING_RULES_PENNY.md`** to project root
3. **Merge `interfaces/interfaces.go` additions** — add the three new type definitions
4. **Patch `cmd/bot/main.go`** — add 4 route registrations and 5 goroutine starts (diff can be cherry-picked)
5. **Patch `mcp-server.js`** — add 4 tool definitions and 4 switch cases
6. **Add agent/strategy entries** via the dashboard UI or direct JSON edit to `data/agent-config.json`
7. **Switch active agent** to PennyProphet from the dashboard Agents tab when ready to run

No existing files in ClaudeProphet need to be deleted or restructured.

---

## 11. Out of Scope (v1)

- Twitter/X social adapter (add later — same interface, no core changes)
- FDA event calendar integration (add to SECEdgarService as a second adapter in v2)
- Backtesting harness
- Dashboard UI changes (PennyProphet visible through existing Agents/Strategies tabs)
- Portfolio-level correlation tracking across Prophet + PennyProphet simultaneously
