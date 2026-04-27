# Sandbox–Account Link Design

**Date:** 2026-04-27
**Status:** Approved

## Summary

Connect the Sandboxes and Accounts tabs so users can create a new sandbox directly from an existing trading account without re-entering credentials. Simultaneously, remove all runtime controls (Start/Pause/Stop) from the Accounts tab, making it a pure credential management surface. All sandbox lifecycle controls live exclusively in the Sandboxes tab.

## Goals

- Reduce friction when creating a sandbox for an already-configured account
- Eliminate the confusing duplication of Start/Pause/Stop controls across two tabs
- Keep the Accounts tab focused solely on credential CRUD

## Non-Goals

- Changes to the backend API (no new endpoints)
- Changes to the Sandboxes tab's existing "New Sandbox" manual-entry flow
- Removing status badges from the Accounts tab (kept as read-only display)

## Design

### 1. Sandboxes Tab Header

The header changes from one button to two, side by side:

```
Active Sandboxes          [New Sandbox]  [From Account]
```

- **New Sandbox** — existing button, unchanged, primary style
- **From Account** — new button, secondary/outlined style, calls `showModal('sandbox-from-account')`
- If `config.accounts` is empty, "From Account" renders as `disabled` with `title="Add an account first in the Accounts tab"`

### 2. "From Account" Modal

**Title:** New Sandbox from Account

**Fields:**

| Field | Type | Required | Notes |
|---|---|---|---|
| Sandbox Name | text input | Yes | Free-form label for this sandbox |
| Account | dropdown | Yes | Lists all accounts as `"Name (Paper)"` or `"Name (Live)"` |
| Assign Agent | dropdown | No | Same options as existing New Sandbox modal |
| Heartbeat Profile | dropdown | No | Same options as existing New Sandbox modal |

**Behaviour on submit (`submitSandboxFromAccount()`):**
1. Validate name and account selection client-side; show toast on failure
2. POST to `/api/accounts/:id/clone` with `{ name }` — the backend copies credentials from the source account server-side
3. If an agent was selected, PUT to `/api/sandboxes/:newId/agent`
4. If a heartbeat profile was selected, POST to `/api/heartbeat/apply-profile`
5. Close modal and show success toast

**Why a clone endpoint:** `config.accounts` only holds masked secret keys (last 4 chars visible). Re-POSTing the masked value would corrupt the new account's credentials. A thin `POST /api/accounts/:id/clone` endpoint on the server reads the full stored credentials and writes a new account record, keeping secret keys server-side at all times.

### 3. Accounts Tab Cleanup

**Remove from each account card:**
- Start button
- Pause button  
- Stop button

**Keep on each account card:**
- Status badges: Running / Ready / Stopped (read-only)
- Runtime info: Phase, Beats count (read-only, shown only when running)
- View button
- Configure button
- Delete button

The `controls` variable in `renderAccounts()` (currently lines 3184–3186 in `index.html`) is deleted entirely. The card's action row retains View, Configure, Delete.

## Files Changed

| File | Change |
|---|---|
| `agent/public/index.html` | Add "From Account" button to Sandboxes header; add `sandbox-from-account` modal branch in `showModal()`; add `submitSandboxFromAccount()` function; remove `controls` from `renderAccounts()` |
| `agent/server.js` | Add `POST /api/accounts/:id/clone` route that reads source account credentials and creates a new account+sandbox |

## Open Questions

None — all decisions made during brainstorming.
