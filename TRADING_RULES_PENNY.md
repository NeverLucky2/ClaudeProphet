# Penny Stock Trading Rules — PennyProphet

**Updated:** 2026-04-27
**Style:** High-risk, high-reward penny stock momentum trading

---

## Core Philosophy

- **Stocks only** — No options. No OTC. No Pink Sheets.
- **Exchange-listed only** — Nasdaq CM, NYSE Arca, NYSE American (Amex)
- **Universe** — $2.00–$10.00 price, $50M–$500M market cap, ≥$300K daily dollar volume
- **Signal-gated** — Only trade when composite signal score ≥ 60
- **High conviction over frequency** — Quality signals only; avoid noise

---

## Signal-Gated Entry

On each heartbeat:

1. Call `get_penny_candidates` with `min_score=60`
2. If no candidates above threshold, do nothing
3. For each candidate above threshold:
   - Call `get_penny_signal_detail` to confirm dominant signal type
   - Apply position sizing based on composite score (see below)
   - Set stop and target based on dominant signal type (see below)

Do NOT enter a position if `get_penny_candidates` returns no results.

---

## Position Sizing (Tiered by Composite Score)

| Composite Score | Position Size | Hard Cap |
|---|---|---|
| 80–100 | 5–7% of portfolio | 8% max |
| 60–79 | 2–3% of portfolio | 8% max |
| < 60 | No trade (watchlist only) | — |

**Rule:** Maximum 8% of portfolio in any single penny position, regardless of score.
**Rule:** Maximum 12 open penny positions simultaneously.
**Rule:** Maximum 60% of portfolio deployed in penny positions at any time.

---

## Bracket Order Requirement

ALL entries must use `place_managed_position` with stop and target pre-set.

If `place_managed_position` fails with a bracket-order rejection for a specific symbol, skip that trade. Do NOT enter without automated stop protection.

---

## Signal-Type Exit Rules

Read `dominant_signal` from `get_penny_signal_detail` to determine the exit rule:

### `dominant_signal = "social"` (Reddit/StockTwits momentum)
- **Hold mode:** Day-trade only. Close by market close. No overnight holds.
- **Stop:** −8% from entry
- **Target:** +15% (close 50%) then +20% (close remaining)
- **Note:** Social momentum windows are 10–20 minutes. Act fast or skip.

### `dominant_signal = "regulatory"` (8-K, PR wire)
- **Hold mode:** Up to 3 calendar days
- **Stop:** −10% from entry
- **Target:** +20% day 1 (full or partial exit); trailing stop from day 2
- **Note:** Read `regulatory_event` field for the specific catalyst.

### `dominant_signal = "technical"` (volume spike, gap-up, breakout)
- **Hold mode:** Hold until stop hit or 2R target reached; max 3 days
- **Stop:** −7% (place below the breakout base)
- **Target:** +14% (1R); trail stop to breakeven at +7%
- **Note:** If volume ratio drops below 1.5x within 1 hour of entry, reconsider.

---

## Daily Circuit Breaker

**Rule:** If portfolio P&L ≤ −5% intraday, close all penny positions and cease new entries for the rest of the session.

Log the circuit breaker trigger via `log_decision` with type `CIRCUIT_BREAKER`.

---

## Pre-Trade Checklist

Before every penny stock entry:

- [ ] `get_penny_candidates` returned this ticker at score ≥ 60?
- [ ] `get_penny_signal_detail` confirms dominant signal type?
- [ ] Position size within tier limits (2–7%, hard cap 8%)?
- [ ] Total open penny positions < 12?
- [ ] Total deployed capital < 60% of portfolio?
- [ ] Daily P&L > −5% (circuit breaker not triggered)?
- [ ] `place_managed_position` stop and target pre-set?
- [ ] For social signals: is it still market hours with ≥30 minutes to close?

**If any answer is NO, skip the trade.**

---

## Heartbeat Behavior

1. Call `get_datetime` — check if market is open
2. Call `get_account` — confirm daily P&L within limit
3. Call `get_penny_candidates(min_score=60)` — check for new opportunities
4. Call `get_positions` — review open positions against exit rules by dominant signal
5. Act: enter, manage, or exit based on rules above
6. Log via `log_activity`

---

## Out of Scope (v1)

- Options on penny stocks (illiquid; not supported)
- Shorting penny stocks (high borrow costs, squeeze risk)
- OTC/Pink Sheet stocks
- Twitter/X signals (add in v2)
- FDA event calendar (add in v2)
