---
name: adapt-strategy
description: Analyze recent trading performance, identify what rules are drifting or broken, and propose + apply targeted edits to the Aggressive Options v2 strategy. This is the primary learning loop — run it weekly or after any bad stretch.
allowed-tools: Read Glob
---

You are closing the learning loop for the Prophet trading agent. Your job is to read what the agent actually did, compare it to what the strategy says it should do, find the gaps, and propose concrete rule changes — then apply the ones the user approves.

## Step 1 — Load current strategy

Read `data/agent-config.json`. Find the strategy with name `Aggressive Options v2` and extract its full `customRules` text. This is the ground truth you will be editing.

Also note: the `id` of this strategy (you will need it if applying changes).

## Step 2 — Load recent decisions (last 30 days)

Glob `data/sandboxes/8f201546/decisive_actions/*.json`. Read the 60 most recent files. For each, extract:
- `timestamp`
- `action` (BUY / SELL / HOLD / etc.)
- `symbol`
- `reasoning` (full text)

## Step 3 — Load recent P&L context

Glob `data/sandboxes/8f201546/activity_logs/activity_*.json`. Read the 8 most recent. From each `summary`:
- winning_trades, losing_trades, total_pnl, largest_win, largest_loss

Compute aggregate profit factor across all loaded days.

## Step 4 — Gap analysis

For each section of the strategy rules, ask: does the agent's actual behavior match the rule?

Work through these categories:

**Entry discipline**
- Are positions being sized within 15%?
- Are scalps truly ≤5 DTE?
- Are swing positions in the 50–120 DTE / delta 0.40–0.70 band?
- Is the agent using limit orders? (Look for "limit" vs. absence of it in reasoning)

**Exit discipline**
- Are losers being cut at -15%? Or are stops being moved?
- Are scalps being closed EOD?
- Are profits being taken at +25–50%?

**Loss-review protocol**
- After a bad stretch, does the agent pause entries and run stats?
- Is it re-entering same symbols within 2 hours (revenge trading)?

**Position concentration**
- Any sector exceeding 40%?
- More than 10 simultaneous positions?

**Behavioral drift**
- Reasoning that sounds emotional ("hoping", "giving it more time", "should bounce")
- Thesis changes mid-hold without acknowledging the shift

For each gap you find, write:
> **Gap [N]**: [category] — [what the rule says] vs. [what the agent actually did, with timestamp and quote]

## Step 5 — Propose specific rule edits

For each significant gap (ignore one-offs; focus on patterns appearing 2+ times), propose a rule change using this format:

---
**Proposed Edit [N]** — [Category]

**Current rule:**
> [exact quote from customRules]

**Proposed replacement:**
> [your revised text]

**Rationale:** [1–2 sentences explaining what behavior this fixes and what evidence from the decision log supports it]

---

If a gap suggests adding a *new* rule rather than changing an existing one, say so explicitly and write the full new rule text.

## Step 6 — Present and confirm

Show the user all proposed edits clearly. Ask which ones to apply. Do not modify any file until the user confirms specific edits.

## Step 7 — Apply approved edits

For each approved edit:
1. Re-read `data/agent-config.json` to get the freshest version.
2. In the `strategies` array, find the entry with name `Aggressive Options v2`.
3. Edit `customRules` — replace the old rule text with the new rule text exactly as proposed. Preserve all surrounding content.
4. Update `updatedAt` on the strategy entry to now (ISO string).
5. Write the file back.

After all edits are applied, show the final diff of what changed in the strategy's `customRules`. Remind the user the changes take effect on the agent's next heartbeat.
