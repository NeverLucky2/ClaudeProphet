package services

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

const aggregatorRefreshInterval = 10 * time.Second
const evictionThreshold = 10.0

// PennySignalAggregator combines three sub-scores into composite CandidateScore entries.
type PennySignalAggregator struct {
	universe   *PennyUniverseService
	screener   *PennyScreenerService
	edgar      *SECEdgarService
	social     *SocialSignalService
	mu         sync.RWMutex
	candidates map[string]CandidateScore
	logger     *logrus.Logger
}

// NewPennySignalAggregator creates the aggregator.
func NewPennySignalAggregator(
	universe *PennyUniverseService,
	screener *PennyScreenerService,
	edgar *SECEdgarService,
	social *SocialSignalService,
) *PennySignalAggregator {
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	return &PennySignalAggregator{
		universe:   universe,
		screener:   screener,
		edgar:      edgar,
		social:     social,
		candidates: make(map[string]CandidateScore),
		logger:     logger,
	}
}

// Start runs the aggregation loop until ctx is cancelled.
func (a *PennySignalAggregator) Start(ctx context.Context) {
	a.aggregate()
	ticker := time.NewTicker(aggregatorRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.aggregate()
		}
	}
}

// GetCandidates returns all scored candidates above minScore, sorted by composite score descending.
func (a *PennySignalAggregator) GetCandidates(minScore float64) []CandidateScore {
	a.mu.RLock()
	defer a.mu.RUnlock()
	var out []CandidateScore
	for _, c := range a.candidates {
		if c.CompositeScore >= minScore {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CompositeScore > out[j].CompositeScore
	})
	return out
}

// GetSignalDetail returns the full CandidateScore for one ticker, or nil if not tracked.
func (a *PennySignalAggregator) GetSignalDetail(ticker string) *CandidateScore {
	a.mu.RLock()
	defer a.mu.RUnlock()
	c, ok := a.candidates[ticker]
	if !ok {
		return nil
	}
	cp := c
	return &cp
}

// GetUniverse returns the current universe from the universe service.
func (a *PennySignalAggregator) GetUniverse() []UniverseSymbol {
	return a.universe.GetUniverse()
}

// RefreshUniverse logs an informational message. The universe self-refreshes on its 15-min cycle.
// This method exists for MCP tool compatibility (scan_penny_universe_now).
func (a *PennySignalAggregator) RefreshUniverse() {
	a.logger.Info("PennySignalAggregator: RefreshUniverse called (next auto-refresh in ≤15min)")
}

func (a *PennySignalAggregator) aggregate() {
	universe := a.universe.GetUniverse()
	now := time.Now()

	a.mu.Lock()
	defer a.mu.Unlock()

	for _, u := range universe {
		techScore, techCtx := a.screener.GetTechnicalScore(u.Ticker)
		regScore, regEvent := a.edgar.GetRegulatoryScore(u.Ticker)
		socScore, socCtx := a.social.GetSocialScore(u.Ticker)

		composite := math.Min(techScore+regScore+socScore, 100.0)

		if composite < evictionThreshold {
			delete(a.candidates, u.Ticker)
			continue
		}

		a.candidates[u.Ticker] = CandidateScore{
			Ticker:           u.Ticker,
			Price:            u.Price,
			CompositeScore:   composite,
			TechnicalScore:   techScore,
			RegulatoryScore:  regScore,
			SocialScore:      socScore,
			DominantSignal:   dominantSignal(techScore, regScore, socScore),
			TechnicalContext: techCtx,
			RegulatoryEvent:  regEvent,
			SocialContext:    socCtx,
			LastUpdated:      now,
		}
	}
}
