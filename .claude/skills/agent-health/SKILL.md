---
name: agent-health
description: Quick operational status check — active strategy, last heartbeat, current positions, recent errors, and whether the agent is behaving consistently with its rules. Run this any time you want a fast situational read without a full performance review.
allowed-tools: Read Glob
---

You are doing a fast operational health check on the Prophet trading agent. This should take under 60 seconds to produce — be concise.

## Step 1 — Config state

Read `data/agent-config.json`. Extract and report:
- Active agent name and ID
- Active strategy name and ID (confirm it is `Aggressive Options v2`)
- Active model
- Heartbeat intervals (pre_market / market_open / midday / market_close / after_hours)
- Key permissions: allowLiveTrading, maxPositionPct, maxDailyLoss, allow0DTE, maxToolRoundsPerBeat

Flag anything unusual (e.g. 0DTE enabled when it should be off, maxDailyLoss set above 5%).

## Step 2 — Last session

Read the most recent activity log from `data/sandboxes/8f201546/activity_logs/`. Report:
- Date and session start time
- Starting capital, ending capital, P&L for the day ($  and %)
- Positions opened / closed / currently active
- Any errors or anomalies in the `activities` array (look for type: "ERROR" or reasoning containing "failed", "error", "exception")

## Step 3 — Recent decisions (last 24 hours)

Glob `data/sandboxes/8f201546/decisive_actions/*.json`. Read the 15 most recent. Report a compact table:

| Time | Action | Symbol | One-line reasoning summary |
|---|---|---|---|
| HH:MM | BUY/SELL/HOLD | XYZ | ... |

Flag any decision that looks inconsistent with the active strategy (e.g. a hold past -15%, a scalp held overnight, an oversized position).

## Step 4 — Loss-review protocol status

Check whether any of today's decisions should have triggered the loss-review thresholds:
- Was the portfolio down >3.5% at any point? If so, did the agent pause entries?
- Was the portfolio down >5%? If so, did the agent stop trading for the day?
- Is today Monday pre-market? If so, was a weekly review logged?

## Step 5 — Health summary

Produce a clean 5-line status block:

```
Agent:        [name] on [model]
Strategy:     [strategy name] (ID: [id])
Last session: [date] | P&L: [+/-$X / +/-X%]
Open positions: [N] | Cash: $[X] ([X]% of portfolio)
Status:       HEALTHY / WATCH / ALERT — [one-sentence reason]
```

Use HEALTHY if everything looks normal.
Use WATCH if there's a minor anomaly (a rule bent, a loss approaching thresholds).
Use ALERT if there's a rule violation, a strategy mismatch, a session error, or a loss-review trigger that was ignored.
