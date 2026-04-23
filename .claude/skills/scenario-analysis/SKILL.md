---
name: scenario-analysis
description: Run a full macro scenario analysis pipeline. Auto-mode (no argument): scans today's top financial headlines and selects the most macro-significant one automatically. Manual mode (with argument): uses the provided headline or topic. Chains Scenario Analyst → Strategy Reviewer and saves both reports to data/reports/. Use before any macro-driven position entry over $20K, or as a daily pre-market briefing.
allowed-tools: Read WebSearch WebFetch Write
---

You are running a two-phase macro scenario analysis pipeline for the Prophet trading system. Phase 1 is the Scenario Analyst (18-month scenario construction). Phase 2 is the Strategy Reviewer (second-opinion critique). Both reports are saved to `data/reports/`.

Work through every step in order. Do not skip steps.

---

## Step 0 — Determine the headline

**Check whether an argument was passed to this skill.**

**If an argument was provided:** Use it exactly as the headline or topic. Record it, then skip to Step 1.

**If no argument was provided (auto mode):**

Run the following WebSearch queries to surface today's most important macro news:
1. `"market news today" financial site:reuters.com OR site:ft.com OR site:wsj.com OR site:bloomberg.com`
2. `"macro catalyst" OR "market moving" OR "Fed" OR "tariffs" OR "inflation" financial news today`
3. `top market headlines today stocks bonds`

From the results, select **one headline** that best fits these criteria (in priority order):
- Federal Reserve / central bank decisions or commentary
- Major tariff, trade war, or geopolitical escalation/de-escalation
- Inflation, CPI, PPI, or jobs data surprises
- Geopolitical shock (conflict, sanctions, elections with market consequences)
- Sector-wide catalyst (not a single stock story — must affect an index or broad sector)
- Major earnings only if they move the broader market narrative (e.g. NVDA for AI, JPM for financials)

Avoid: individual company stories with no sector read-through, entertainment/sports, routine analyst price target changes, regulatory filings.

**State the selected headline clearly before proceeding.** If no clearly macro-significant news is found, say so and use the most relevant available story with a note.

---

## Step 1 — News gathering

You are now acting as an experienced fund manager with 20+ years in mid-to-long-term equity portfolio management.

Extract 3-5 keywords from the headline. Run these WebSearch queries to gather related coverage from the past 2 weeks:
- `[main keywords] market impact`
- `[main keywords] sector analysis`
- `[related policy or regulatory angle] 2025 OR 2026`
- `[main keywords] analyst reaction OR expert view`

Priority sources: Wall Street Journal, Financial Times, Bloomberg, Reuters. For each relevant article collect: headline, source name, publication date, key data points or quotes, and the initial market reaction if reported.

Gather at least 4 articles before proceeding. If fewer than 4 are found, run one additional search with different keywords.

---

## Step 2 — Event classification

Classify the event into exactly one of these categories:

| Category | Examples |
|----------|---------|
| Monetary Policy | Fed rate decisions, central bank guidance, QT/QE changes |
| Geopolitics | Military conflict, sanctions, tariffs, trade negotiations |
| Regulation | Financial regulation, antitrust action, environmental rules |
| Technology | AI breakthrough, EV adoption, semiconductor supply |
| Commodities | Oil supply shock, gold price move, crop failure |
| Corporate / M&A | Major acquisition, bankruptcy, sector consolidation |

State the category and a one-sentence description of why this event fits it.

---

## Step 3 — 18-month scenario construction

Build three scenarios. **Probabilities must sum to exactly 100%.**

**Base Case (target 50-60%):** The most probable outcome given current data and historical precedent. State every assumption explicitly — do not leave premises implicit.

**Bull Case (target 15-25%):** The optimistic outcome. Name the specific upside catalysts that would need to materialize.

**Bear Case (target 20-30%):** The risk scenario. Name the specific downside factors that would need to play out.

For each scenario provide all of the following:
- **Summary:** 1-2 sentences describing the outcome
- **Key assumptions:** Bullet list of the premises this scenario depends on
- **Timeline:**
  - 0–6 months: Near-term developments
  - 6–12 months: Mid-term developments
  - 12–18 months: Long-term outcome
- **Economic impacts:** Directional effect on GDP, inflation, and interest rates (use: rising / falling / neutral / uncertain)

---

## Step 4 — Sector impact analysis

For each of the three tiers below, produce a markdown table.

**1st order — Direct impact:**
Sectors immediately and directly affected by the headline event itself.
`| Sector | Impact | Reasoning |`

**2nd order — Value chain / related industries:**
Downstream and upstream ripple effects: supply chain, customers, competitors.
`| Sector | Impact | Transmission path |`

**3rd order — Structural / macro:**
Regulatory environment shifts, technology acceleration or deceleration, long-term changes to industry structure.
`| Domain | Impact | Long-term implication |`

Impact column values: `Positive`, `Negative`, `Mixed`, or `Neutral`.

Include at least 3 rows per tier. Do not leave a tier empty.

---

## Step 5 — Stock selection

**Positive impact stocks (3–5, US-listed only):**

Selection criteria — all three must apply:
1. A clear, specific reason why this scenario benefits this stock
2. The stock has outperformed in historically similar events (state the analogue)
3. Fundamentally sound: no severe leverage or margin problems

Table: `| Ticker | Company | Rationale | Performance in similar events |`

**Negative impact stocks (3–5, US-listed only):**

Selection criteria — all three must apply:
1. A clear, specific reason why this scenario hurts this stock
2. The stock has underperformed in historically similar events (state the analogue)
3. A specific vulnerability is present: high leverage, thin margins, regulatory exposure, or direct operational impact

Table: `| Ticker | Company | Rationale | Performance in similar events |`

---

## Step 6 — Save Scenario Analyst report

Determine today's date (format: YYYYMMDD). Create a 2–3 word topic slug from the headline (e.g. `fed_rate_hold`, `tariff_china_tech`, `oil_supply_shock`, `cpi_surprise_high`). Use only lowercase letters and underscores.

Write the complete Scenario Analyst report to:
`data/reports/scenario_<topic>_<YYYYMMDD>.md`

The report must contain all of the following sections in order:
1. `# Scenario Analysis: [headline]`
2. `**Date:** [today] | **Event type:** [classification]`
3. `## Related News Articles` — bulleted list with source and date
4. `## 18-Month Scenarios` — all three scenarios with full structure from Step 3
5. `## 1st-Order Sector Impacts` — table from Step 4
6. `## 2nd-Order Sector Impacts` — table from Step 4
7. `## 3rd-Order Sector Impacts` — table from Step 4
8. `## Positive Impact Stocks` — table from Step 5
9. `## Negative Impact Stocks` — table from Step 5

Do not truncate any section. Write the complete report before proceeding to Step 7.

---

## Step 7 — Strategy Reviewer pass

You are now acting as a **different** senior fund manager providing a rigorous second opinion. You did not write the Scenario Analyst report. Your job is to pressure-test it.

Review the report you just wrote across all six dimensions:

### Dimension 1 — Coverage check

Verify the analyst addressed:
- All materially affected sectors (flag any obvious omissions)
- Global spillover: Europe, Asia, emerging markets
- Cross-asset effects: bonds, commodities, FX
- Regulatory and political risk
- Tail risks: low-probability, high-impact events

List any blind spots found. Common ones: upstream/downstream supply chain effects, FX translation impact on earnings, labor market knock-ons, consumer behavior shifts, indirect competitor exposure.

### Dimension 2 — Scenario probability validity

- Confirm Base + Bull + Bear = 100%. If not, flag it as an error.
- Flag if Base Case is outside 40–75% without a stated justification.
- Flag over-optimistic Bull Cases where downside risk appears under-priced.
- Flag under-weighted Bear Cases indicating optimism bias.
- Check whether any Bull/Bear probability asymmetry is justified by the fundamentals — or whether it looks like lazy symmetric allocation (e.g. 60/20/20 with no reason for the split).

### Dimension 3 — Impact logic check

For each order of impact, verify:
- The transmission mechanism is explicit and causal — not just asserted
- Timing is appropriate (immediate, weeks, or months?)
- Feedback loops and second-round effects are considered
- Magnitude is characterized, not just direction ("significantly negative" is better than "negative")

Flag any logical gaps: correlation mistaken for causation, intermediate steps skipped, impacts stated directionally with no scale.

### Dimension 4 — Bias detection

**Optimism bias:** positive factors over-weighted; risks minimized; worst cases excluded or described as unlikely without evidence.

**Pessimism bias:** negative factors over-weighted; recovery and adaptation mechanisms ignored; worst case treated as base.

**Confirmation bias:** analysis shaped entirely to fit the headline narrative; contrary expert views or data absent; alternative explanations dismissed.

State specifically which bias (if any) is detectable and where in the report it shows.

### Dimension 5 — Alternative scenarios

Identify any important scenarios the analyst did not consider. For each, provide:
- Name
- Probability estimate (even if very small, e.g. 2%)
- One-paragraph summary
- Key catalysts that would trigger it
- Expected market and sector impacts

Required candidates to evaluate (include if probability > 1%):
- Policy response scenario: what if the government or central bank intervenes directly?
- Technology disruption: what if an unexpected innovation shift changes the calculus?
- Geopolitical surprise: what if the situation escalates or resolves unexpectedly?
- Black swan: name one extreme low-probability scenario even if < 3%

### Dimension 6 — Timeline realism

- Are the expected changes achievable within 18 months given real-world constraints?
- Are the phase boundaries (0–6, 6–12, 12–18 months) logically motivated or arbitrary?
- Is the pace of change consistent with historical precedents for this type of event?
- Are delay factors accounted for: regulatory approval timelines, capex cycles, contract renegotiation windows, political cycles?

---

## Step 8 — Save Strategy Reviewer report

Write the complete second-opinion report to:
`data/reports/review_<topic>_<YYYYMMDD>.md`

Use the same `<topic>` and `<YYYYMMDD>` as Step 6.

The report must contain these sections in order:
1. `# Strategy Reviewer: Second Opinion on [headline]`
2. `**Date:** [today] | **Reviewing:** scenario_<topic>_<YYYYMMDD>.md`
3. `## Overall Assessment` — 2–3 sentences on quality, reliability, and confidence level
4. `## Coverage Gaps` — missing sectors with reasoning; additional stock candidates table if any
5. `## Scenario Probability Assessment` — current allocation, recommended adjustments with reasoning
6. `## Impact Logic Review` — valid points and specific issues with suggested fixes
7. `## Bias Assessment` — detected biases with evidence; correction suggestions
8. `## Alternative Scenarios` — each with probability, summary, catalysts, and market impacts
9. `## Timeline Assessment` — valid points and suggested revisions
10. `## Final Recommendations` — top 3 strengths; top 3 improvements in priority order; areas needing additional research

Do not truncate any section.

---

## Step 9 — Summary to user

After both reports are saved, present this compact summary (do not repeat the full reports):

```
SCENARIO ANALYSIS COMPLETE
─────────────────────────────────────────────────────
Headline:     [the headline that was analyzed]
Event type:   [classification]
Date:         [today]

SCENARIOS
  Base  [X]%  [one-line summary]
  Bull  [X]%  [one-line summary]
  Bear  [X]%  [one-line summary]

TOP STOCKS — POSITIVE:  [ticker], [ticker], [ticker]
TOP STOCKS — NEGATIVE:  [ticker], [ticker], [ticker]

REVIEWER FLAGS (top 2):
  1. [most important critique from the Strategy Reviewer]
  2. [second most important critique]

REPORTS SAVED:
  data/reports/scenario_<topic>_<YYYYMMDD>.md
  data/reports/review_<topic>_<YYYYMMDD>.md
─────────────────────────────────────────────────────
```

If any step produced an error (e.g. WebSearch returned no results, report could not be saved), state it clearly in the summary with a suggestion for how to retry.
