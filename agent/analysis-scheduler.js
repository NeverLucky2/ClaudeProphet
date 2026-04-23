/**
 * Analysis Scheduler - Runs pre-market, weekly, and event-driven analysis jobs.
 *
 * Time-based (while server is running):
 *   6:00 AM ET weekdays    → daily_briefing
 *   6:05 AM ET Mondays     → review_performance (if not done this week) → adapt_strategy
 *   4:30 PM ET weekdays    → loss check → postmortem + adapt_strategy if triggered
 *   6:00 PM ET Sundays     → weekly_screeners
 *
 * Startup-based (on server start, if criteria met):
 *   daily_briefing         → data/reports/daily_brief_YYYYMMDD.json missing
 *   scenario_analysis      → no data/reports/scenario_*_YYYYMMDD.md today
 *   review_performance     → not run this ISO week  → then adapt_strategy
 *   postmortem             → last session had ≥-3% loss and not yet run → then adapt_strategy
 *   adapt_strategy         → after review or postmortem, or 3 consecutive losing days
 *
 * Persisted state: data/scheduler-state.json
 */
import { spawn } from 'child_process';
import fs from 'fs/promises';
import os from 'os';
import path from 'path';
import { fileURLToPath } from 'url';
import { EventEmitter } from 'events';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const PROJECT_ROOT = path.join(__dirname, '..');
const OPENCODE_BIN = process.platform === 'win32' ? 'cmd.exe' : 'opencode';
const OPENCODE_WIN_PREFIX = process.platform === 'win32' ? ['/c', 'opencode.cmd'] : [];
const STATE_FILE = path.join(PROJECT_ROOT, 'data', 'scheduler-state.json');
const SANDBOXES_DIR = path.join(PROJECT_ROOT, 'data', 'sandboxes');
const REPORTS_DIR = path.join(PROJECT_ROOT, 'data', 'reports');

export class AnalysisScheduler extends EventEmitter {
  constructor(options = {}) {
    super();
    this.model = options.model || 'anthropic/claude-sonnet-4-6';
    this._timer = null;
    this._running = false;
    this._activeJob = null;
    // File-detectable (reset on restart is fine — file presence is the guard)
    this._lastDailyBriefDate = null;
    this._lastWeeklyScreenDate = null;
    this._lastScenarioDate = null;
    // State-persisted (no output file — must survive restarts)
    this._lastReviewWeek = null;
    this._lastPostmortemDate = null;
    this._lastAdaptDate = null;
    this._lastLossCheckDate = null;
  }

  async start() {
    if (this._running) return;
    await this._loadState();
    this._running = true;
    this._timer = setInterval(() => this._checkSchedule(), 60 * 1000);
    this._checkSchedule();
    this._log('Analysis scheduler started.', 'info');
  }

  stop() {
    this._running = false;
    if (this._timer) { clearInterval(this._timer); this._timer = null; }
    this._log('Analysis scheduler stopped.', 'warning');
  }

  getStatus() {
    return {
      running: this._running,
      activeJob: this._activeJob,
      lastDailyBriefDate: this._lastDailyBriefDate,
      lastWeeklyScreenDate: this._lastWeeklyScreenDate,
      lastScenarioDate: this._lastScenarioDate,
      lastReviewWeek: this._lastReviewWeek,
      lastPostmortemDate: this._lastPostmortemDate,
      lastAdaptDate: this._lastAdaptDate,
    };
  }

  async triggerJob(jobName, date, target) {
    if (this._activeJob) return { error: `Job already running: ${this._activeJob}` };
    const isoDate = date || new Date().toLocaleDateString('en-CA', { timeZone: 'America/New_York' });
    this._activeJob = jobName;
    try {
      if (jobName === 'daily_briefing') {
        this._lastDailyBriefDate = isoDate;
        await this._runDailyBriefing(isoDate);
      } else if (jobName === 'weekly_screeners') {
        this._lastWeeklyScreenDate = isoDate;
        await this._runWeeklyScreeners(isoDate);
      } else if (jobName === 'scenario_analysis') {
        this._lastScenarioDate = isoDate;
        await this._runScenarioAnalysis(isoDate);
      } else if (jobName === 'review_performance') {
        this._lastReviewWeek = this._getISOWeek(isoDate);
        await this._runSkill('review-performance', isoDate, null, 10 * 60 * 1000);
        await this._saveState();
      } else if (jobName === 'postmortem') {
        this._lastPostmortemDate = isoDate;
        await this._runSkill('postmortem', isoDate, target || isoDate, 10 * 60 * 1000);
        await this._saveState();
      } else if (jobName === 'adapt_strategy') {
        this._lastAdaptDate = isoDate;
        await this._runAdaptStrategy(isoDate);
        await this._saveState();
      } else {
        this._activeJob = null;
        return { error: `Unknown job: ${jobName}. Valid: daily_briefing, weekly_screeners, scenario_analysis, review_performance, postmortem, adapt_strategy` };
      }
      return { success: true, job: jobName, date: isoDate };
    } finally {
      this._activeJob = null;
    }
  }

  // Run all startup checks in order. Call once after start() — runs in background.
  async runStartupChecks() {
    const isoDate = new Date().toLocaleDateString('en-CA', { timeZone: 'America/New_York' });
    const todaySlug = isoDate.replace(/-/g, '');
    let adaptNeeded = false;

    // 1. Daily briefing (file-based)
    try { await fs.access(path.join(REPORTS_DIR, `daily_brief_${todaySlug}.json`)); }
    catch {
      this._log('No daily briefing for today — triggering now...', 'info');
      await this.triggerJob('daily_briefing').catch(() => {});
    }

    // 2. Scenario analysis (file-based)
    const reportFiles = await fs.readdir(REPORTS_DIR).catch(() => []);
    if (!reportFiles.some(f => f.startsWith('scenario_') && f.includes(todaySlug))) {
      this._log('No scenario analysis for today — triggering now...', 'info');
      await this.triggerJob('scenario_analysis').catch(() => {});
    }

    // 3. Weekly performance review (state-based)
    if (this._lastReviewWeek !== this._getISOWeek(isoDate)) {
      this._log('No performance review this week — triggering now...', 'info');
      await this.triggerJob('review_performance').catch(() => {});
      adaptNeeded = true;
    }

    // 4. Postmortem for significant loss (activity log detection)
    const lossInfo = await this._detectLossConditions();
    if (lossInfo?.significantLoss && this._lastPostmortemDate !== lossInfo.lossDate) {
      this._log(`Significant loss on ${lossInfo.lossDate} (${lossInfo.lossPercent.toFixed(1)}%) — triggering postmortem...`, 'warning');
      await this.triggerJob('postmortem', lossInfo.lossDate).catch(() => {});
      adaptNeeded = true;
    }
    if (lossInfo?.consecutiveLossDays >= 3) {
      this._log('3 consecutive losing days detected.', 'warning');
      adaptNeeded = true;
    }

    // 5. Adapt strategy if anything above triggered it
    if (adaptNeeded && this._lastAdaptDate !== isoDate) {
      this._log('Triggering adapt-strategy...', 'info');
      await this.triggerJob('adapt_strategy').catch(() => {});
    }
  }

  // ── Private helpers ──────────────────────────────────────────────

  _log(message, level = 'info') {
    this.emit('agent_log', { message: `[Scheduler] ${message}`, level, timestamp: new Date().toISOString() });
  }

  _getETInfo() {
    const now = new Date();
    const et = now.toLocaleTimeString('en-US', {
      timeZone: 'America/New_York', hour: '2-digit', minute: '2-digit', hour12: false,
    });
    const [hour, minute] = et.split(':').map(Number);
    const isoDate = now.toLocaleDateString('en-CA', { timeZone: 'America/New_York' });
    const dayName = now.toLocaleDateString('en-US', { timeZone: 'America/New_York', weekday: 'long' });
    const dayOfWeek = ['Sunday','Monday','Tuesday','Wednesday','Thursday','Friday','Saturday'].indexOf(dayName);
    return { hour, minute, isoDate, dayOfWeek };
  }

  _getISOWeek(dateStr) {
    const d = new Date(dateStr + 'T12:00:00Z');
    const day = d.getUTCDay() || 7;
    d.setUTCDate(d.getUTCDate() + 4 - day);
    const yearStart = new Date(Date.UTC(d.getUTCFullYear(), 0, 1));
    const weekNo = Math.ceil((((d - yearStart) / 86400000) + 1) / 7);
    return `${d.getUTCFullYear()}-W${String(weekNo).padStart(2, '0')}`;
  }

  async _loadState() {
    try {
      const raw = await fs.readFile(STATE_FILE, 'utf-8');
      const s = JSON.parse(raw);
      this._lastReviewWeek = s.lastReviewWeek || null;
      this._lastPostmortemDate = s.lastPostmortemDate || null;
      this._lastAdaptDate = s.lastAdaptDate || null;
      this._lastLossCheckDate = s.lastLossCheckDate || null;
    } catch {}
  }

  async _saveState() {
    try {
      await fs.writeFile(STATE_FILE, JSON.stringify({
        lastReviewWeek: this._lastReviewWeek,
        lastPostmortemDate: this._lastPostmortemDate,
        lastAdaptDate: this._lastAdaptDate,
        lastLossCheckDate: this._lastLossCheckDate,
      }, null, 2), 'utf-8');
    } catch {}
  }

  // Returns { significantLoss, lossDate, lossPercent, consecutiveLossDays } or null.
  async _detectLossConditions() {
    try {
      // Find sandboxes that have activity logs
      const sandboxes = await fs.readdir(SANDBOXES_DIR).catch(() => []);
      let allLogs = [];
      for (const sb of sandboxes) {
        const logsDir = path.join(SANDBOXES_DIR, sb, 'activity_logs');
        let files;
        try { files = (await fs.readdir(logsDir)).filter(f => f.startsWith('activity_') && f.endsWith('.json')).sort(); }
        catch { continue; }
        for (const f of files.slice(-5)) {
          try {
            const log = JSON.parse(await fs.readFile(path.join(logsDir, f), 'utf-8'));
            const s = log.summary || {};
            const hasTrades = (s.winning_trades || 0) + (s.losing_trades || 0) > 0;
            if (hasTrades) allLogs.push({ date: log.date, pnlPct: s.total_pnl_percent || 0 });
          } catch {}
        }
      }
      if (allLogs.length === 0) return null;
      allLogs.sort((a, b) => a.date.localeCompare(b.date));

      const latest = allLogs[allLogs.length - 1];
      const significantLoss = latest.pnlPct <= -5.0;

      // Count consecutive losing days from the end
      let consecutiveLossDays = 0;
      for (let i = allLogs.length - 1; i >= 0; i--) {
        if (allLogs[i].pnlPct < 0) consecutiveLossDays++;
        else break;
      }

      return {
        significantLoss,
        lossDate: significantLoss ? latest.date : null,
        lossPercent: latest.pnlPct,
        consecutiveLossDays,
      };
    } catch { return null; }
  }

  async _checkAndRunLossJobs(isoDate) {
    const lossInfo = await this._detectLossConditions();
    if (!lossInfo) return;
    let adaptNeeded = false;
    if (lossInfo.significantLoss && this._lastPostmortemDate !== lossInfo.lossDate) {
      this._log(`Significant loss on ${lossInfo.lossDate} (${lossInfo.lossPercent.toFixed(1)}%) — triggering postmortem...`, 'warning');
      await this.triggerJob('postmortem', lossInfo.lossDate).catch(() => {});
      adaptNeeded = true;
    }
    if (lossInfo.consecutiveLossDays >= 3) adaptNeeded = true;
    if (adaptNeeded && this._lastAdaptDate !== isoDate) {
      await this.triggerJob('adapt_strategy').catch(() => {});
    }
  }

  async _checkSchedule() {
    if (!this._running || this._activeJob) return;
    const { hour, minute, isoDate, dayOfWeek } = this._getETInfo();
    const isWeekday = dayOfWeek >= 1 && dayOfWeek <= 5;
    const isMonday = dayOfWeek === 1;
    const isSunday = dayOfWeek === 0;

    if (isWeekday && hour === 6 && minute === 0 && this._lastDailyBriefDate !== isoDate) {
      await this.triggerJob('daily_briefing').catch(() => {});
    }

    if (isMonday && hour === 6 && minute === 5 && this._lastReviewWeek !== this._getISOWeek(isoDate)) {
      await this.triggerJob('review_performance').catch(() => {});
      if (this._lastAdaptDate !== isoDate) {
        await this.triggerJob('adapt_strategy').catch(() => {});
      }
    }

    if (isWeekday && hour === 16 && minute === 30 && this._lastLossCheckDate !== isoDate) {
      this._lastLossCheckDate = isoDate;
      await this._saveState();
      await this._checkAndRunLossJobs(isoDate);
    }

    if (isSunday && hour === 18 && minute === 0 && this._lastWeeklyScreenDate !== isoDate) {
      await this.triggerJob('weekly_screeners').catch(() => {});
    }
  }

  // ── Job runners ──────────────────────────────────────────────────

  async _runDailyBriefing(date) {
    const dateSlug = date.replace(/-/g, '');
    this._log(`Starting daily briefing for ${date}...`, 'info');
    this.emit('scheduler_job_start', { job: 'daily_briefing', date });

    const hasFmp = !!process.env.FMP_API_KEY;
    const fmpNote = hasFmp ? '' : '\nNote: FMP_API_KEY not set — FTD check, economic calendar, and earnings calendar will be skipped.';

    const prompt = `You are the Prophet Pre-Market Analysis Agent. Today is ${date}. Your job is to run the daily pre-market briefing pipeline and save the results.${fmpNote}

Call these MCP tools in this exact order:
1. run_market_briefing — fetches breadth and uptrend ratio data from public CSV sources (no API key needed). Wait for it to complete.
${hasFmp ? `2. run_ftd_check — detects Follow-Through Day signals (requires FMP API).
3. run_economic_calendar — fetches this week's tier-1 macro events (FOMC, CPI, NFP, GDP).
4. run_earnings_calendar — fetches key earnings announcements for this week.` : `2. (Skipping FTD, economic calendar, and earnings calendar — FMP_API_KEY not set)`}

After all tools have returned, use the Write tool to save the briefing to exactly this path:
data/reports/daily_brief_${dateSlug}.json

The JSON must be exactly this structure (fill all values from tool results):
{
  "date": "${date}",
  "generated_at": "<current UTC ISO timestamp>",
  "market_posture": "<BULLISH|NEUTRAL|BEARISH — based on breadth score: BULLISH >70, NEUTRAL 40-70, BEARISH <40>",
  "breadth_score": <integer 0-100 from run_market_briefing composite score>,
  "uptrend_ratio": <float 0-100 from run_market_briefing uptrend ratio field>,
  "ftd_status": "<active_ftd|rally_attempt|no_signal|correction — from run_ftd_check, or null if skipped>",
  "tier1_macro_events": [<objects from run_economic_calendar with date, event, impact fields — empty array if skipped or none>],
  "key_earnings_this_week": [<objects from run_earnings_calendar with date, ticker, timing fields — empty array if skipped or none>],
  "exposure_ceiling_pct": <integer 0-100 — your recommended max exposure: 100 if BULLISH, 60 if NEUTRAL, 20 if BEARISH; reduce further if active_ftd or tier-1 event today>,
  "summary": "<2-3 sentences describing today's market setup and key risks>"
}

Use null for any field where the corresponding tool failed. Write only the JSON — no markdown, no explanation.`;

    await this._runOneshotOpencode(prompt, 'daily_briefing', 10 * 60 * 1000);
    this._log(`Daily briefing complete → data/reports/daily_brief_${dateSlug}.json`, 'success');
    this.emit('scheduler_job_end', { job: 'daily_briefing', date, output: `data/reports/daily_brief_${dateSlug}.json` });
  }

  async _runWeeklyScreeners(date) {
    const dateSlug = date.replace(/-/g, '');
    this._log(`Starting weekly screeners for week of ${date}...`, 'info');
    this.emit('scheduler_job_start', { job: 'weekly_screeners', date });

    const hasFmp = !!process.env.FMP_API_KEY;
    const fmpNote = hasFmp ? '' : '\nNote: FMP_API_KEY not set — market top check, VCP, and PEAD screeners will be skipped.';

    const prompt = `You are the Prophet Weekly Research Agent. Today is ${date} (Sunday). Run the weekly screening pipeline.${fmpNote}

Call these MCP tools in this exact order:
1. run_market_briefing — fetch current breadth and uptrend data.
${hasFmp ? `2. run_market_top_check — get distribution day count and market top probability (runs synchronously, ~90 seconds).
3. run_vcp_screener — start background VCP screener (returns immediately with job status).
4. run_pead_screener — start background PEAD screener (returns immediately with job status).
5. wait(210) — wait 3.5 minutes for background screeners to finish.
6. read_latest_report("vcp") — retrieve VCP screener results.
7. read_latest_report("pead") — retrieve PEAD screener results.` : `2. (Skipping FMP screeners — FMP_API_KEY not set)`}

After all tools complete, use the Write tool to save to data/reports/weekly_regime_${dateSlug}.json:
{
  "date": "${date}",
  "generated_at": "<current UTC ISO timestamp>",
  "breadth_score": <0-100 integer from run_market_briefing>,
  "uptrend_ratio": <float 0-100>,
  "market_top_score": <0-100 integer from run_market_top_check, or null if skipped>,
  "distribution_days": <count from run_market_top_check, or null if skipped>,
  "market_posture": "<BULLISH|NEUTRAL|BEARISH|CORRECTION|TOP_RISK>",
  "vcp_candidates": [<top 5 VCP candidates: {ticker, score, execution_state, pivot_price, stop_loss} — empty array if skipped>],
  "pead_candidates": [<top 5 PEAD candidates: {ticker, score, entry_zone, stop_loss} — empty array if skipped>],
  "weekly_thesis": "<2-3 sentences: current market conditions, key risks for the week, general posture>"
}

Limit vcp_candidates and pead_candidates to top 5 each by score. Write only the JSON.`;

    await this._runOneshotOpencode(prompt, 'weekly_screeners', 25 * 60 * 1000);
    this._log(`Weekly screeners complete → data/reports/weekly_regime_${dateSlug}.json`, 'success');
    this.emit('scheduler_job_end', { job: 'weekly_screeners', date, output: `data/reports/weekly_regime_${dateSlug}.json` });
  }

  async _runScenarioAnalysis(date) {
    const dateSlug = date.replace(/-/g, '');
    this._log(`Starting scenario analysis for ${date}...`, 'info');
    this.emit('scheduler_job_start', { job: 'scenario_analysis', date });

    const prompt = await this._readSkillPrompt('scenario-analysis');
    if (!prompt) {
      this.emit('scheduler_job_end', { job: 'scenario_analysis', date, output: null });
      return;
    }

    await this._runOneshotOpencode(prompt, 'scenario_analysis', 15 * 60 * 1000);
    this._log(`Scenario analysis complete → data/reports/scenario_*_${dateSlug}.md`, 'success');
    this.emit('scheduler_job_end', { job: 'scenario_analysis', date, output: `data/reports/scenario_*_${dateSlug}.md` });
  }

  // Generic skill runner. Replaces $ARGUMENTS in the prompt with `target` if provided.
  async _runSkill(skillName, date, target, timeoutMs) {
    this._log(`Starting ${skillName} for ${date}${target ? ` (target: ${target})` : ''}...`, 'info');
    this.emit('scheduler_job_start', { job: skillName.replace(/-/g, '_'), date });

    let prompt = await this._readSkillPrompt(skillName);
    if (!prompt) {
      this.emit('scheduler_job_end', { job: skillName.replace(/-/g, '_'), date, output: null });
      return;
    }
    if (target !== null && target !== undefined) {
      prompt = prompt.replace(/\$ARGUMENTS/g, target);
    }

    await this._runOneshotOpencode(prompt, skillName, timeoutMs);
    this._log(`${skillName} complete.`, 'success');
    this.emit('scheduler_job_end', { job: skillName.replace(/-/g, '_'), date, output: null });
  }

  async _runAdaptStrategy(date) {
    this._log(`Starting adapt-strategy for ${date}...`, 'info');
    this.emit('scheduler_job_start', { job: 'adapt_strategy', date });

    let prompt = await this._readSkillPrompt('adapt-strategy');
    if (!prompt) {
      this.emit('scheduler_job_end', { job: 'adapt_strategy', date, output: null });
      return;
    }

    // Automated run: skip the confirmation step and apply all proposed edits autonomously.
    prompt += '\n\n---\n**AUTOMATED RUN**: This analysis was triggered automatically by the scheduler. After completing the gap analysis and proposing edits, skip Step 6 (user confirmation) and automatically apply all proposed changes to data/agent-config.json. List every rule that was changed in your final response.';

    await this._runOneshotOpencode(prompt, 'adapt_strategy', 15 * 60 * 1000);
    this._log('adapt-strategy complete.', 'success');
    this.emit('scheduler_job_end', { job: 'adapt_strategy', date, output: 'data/agent-config.json' });
  }

  // Read a skill's SKILL.md and strip the YAML frontmatter.
  async _readSkillPrompt(skillName) {
    const skillPath = path.join(PROJECT_ROOT, '.claude', 'skills', skillName, 'SKILL.md');
    try {
      const raw = await fs.readFile(skillPath, 'utf-8');
      const match = raw.match(/^---[\s\S]*?---\n([\s\S]*)$/);
      return match ? match[1].trim() : raw.trim();
    } catch (err) {
      this._log(`Cannot read ${skillName} skill: ${err.message}`, 'error');
      return null;
    }
  }

  async _runOneshotOpencode(prompt, jobName, timeoutMs) {
    return new Promise(async (resolve) => {
      const ocModel = this.model?.includes('/') ? this.model : `anthropic/${this.model}`;
      const args = ['run', '--format', 'json', '--model', ocModel];

      let tempFile = null;
      if (process.platform === 'win32') {
        tempFile = path.join(os.tmpdir(), `prophet_sched_${Date.now()}.txt`);
        await fs.writeFile(tempFile, prompt, 'utf-8');
        args.push('Process the prompt from the attached file.', '--file', tempFile);
      } else {
        args.push(prompt);
      }

      const proc = spawn(OPENCODE_BIN, [...OPENCODE_WIN_PREFIX, ...args], {
        cwd: PROJECT_ROOT,
        env: {
          ...process.env,
          ANTHROPIC_API_KEY: process.env.CLAUDE_API_KEY || process.env.ANTHROPIC_API_KEY || '',
          OPENPROPHET_SANDBOX_ID: 'analysis',
          OPENPROPHET_ACCOUNT_ID: 'analysis',
        },
        stdio: ['pipe', 'pipe', 'pipe'],
      });

      proc.stdin.end();

      let buffer = '';
      proc.stdout.on('data', (chunk) => {
        buffer += chunk.toString();
        const lines = buffer.split('\n');
        buffer = lines.pop();
        for (const line of lines) {
          if (!line.trim()) continue;
          try {
            const event = JSON.parse(line);
            if (event.type === 'text' && event.part?.text?.trim()) {
              this._log(`${event.part.text.trim().slice(0, 200)}`, 'info');
            }
          } catch {}
        }
      });

      proc.stderr.on('data', (chunk) => {
        const msg = chunk.toString().trim();
        if (msg && !msg.toLowerCase().startsWith('warn')) {
          this._log(`[stderr] ${msg.slice(0, 200)}`, 'info');
        }
      });

      const timeout = setTimeout(() => {
        if (!proc.killed) {
          this._log(`Job timed out after ${Math.round(timeoutMs / 60000)} min`, 'warning');
          proc.kill('SIGTERM');
        }
      }, timeoutMs);

      proc.on('exit', (code) => {
        clearTimeout(timeout);
        if (tempFile) fs.unlink(tempFile).catch(() => {});
        this._log(`[${jobName}] finished (exit: ${code})`, code === 0 ? 'success' : 'warning');
        resolve();
      });

      proc.on('error', (err) => {
        clearTimeout(timeout);
        if (tempFile) fs.unlink(tempFile).catch(() => {});
        this._log(`[${jobName}] spawn failed: ${err.message}`, 'error');
        resolve();
      });
    });
  }
}
