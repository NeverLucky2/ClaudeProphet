package services

import (
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestSocialSignalService_PruneWindow(t *testing.T) {
	svc := &SocialSignalService{logger: logrus.New()}
	now := time.Now()
	svc.mentionWindow = []mentionRecord{
		{Ticker: "OLD", Timestamp: now.Add(-40 * time.Minute)}, // older than 30m window
		{Ticker: "NEW", Timestamp: now.Add(-10 * time.Minute)},
	}
	svc.pruneWindow(now)
	if len(svc.mentionWindow) != 1 || svc.mentionWindow[0].Ticker != "NEW" {
		t.Errorf("expected [NEW] after pruning, got %v", svc.mentionWindow)
	}
}

func TestSocialSignalService_RecomputeRedditScores(t *testing.T) {
	svc := &SocialSignalService{
		entries: make(map[string]socialEntry),
		logger:  logrus.New(),
	}
	now := time.Now()
	// 3 mentions of TICK, 1 of OTHER → velocity of TICK is higher
	svc.mentionWindow = []mentionRecord{
		{Ticker: "TICK", Timestamp: now},
		{Ticker: "TICK", Timestamp: now},
		{Ticker: "TICK", Timestamp: now},
		{Ticker: "OTHER", Timestamp: now},
	}
	svc.recomputeRedditScores(now)
	tickScore := svc.entries["TICK"].BaseScore
	otherScore := svc.entries["OTHER"].BaseScore
	if tickScore <= otherScore {
		t.Errorf("expected TICK score (%f) > OTHER score (%f)", tickScore, otherScore)
	}
}

func TestSocialSignalService_GetSocialScore_Decay(t *testing.T) {
	svc := &SocialSignalService{
		entries: make(map[string]socialEntry),
		logger:  logrus.New(),
	}
	// 4-hour-old entry with 4h half-life → score should be ~half
	svc.entries["STALE"] = socialEntry{BaseScore: 20.0, DetectedAt: time.Now().Add(-4 * time.Hour)}
	got, _ := svc.GetSocialScore("STALE")
	if got < 8 || got > 12 {
		t.Errorf("expected ~10 at half-life, got %f", got)
	}
}

func TestMin64(t *testing.T) {
	if min64(3, 5) != 3 {
		t.Error("min64(3,5) should be 3")
	}
	if min64(5, 3) != 3 {
		t.Error("min64(5,3) should be 3")
	}
}

func TestSocialSignalService_SentimentPtsUseMentionPtsBase(t *testing.T) {
	// Setup: 8 TICK mentions, 2 OTHER mentions → avg=5, TICK velocity=1.6 → mentionPts=8.0
	// Pre-seed TICK with prior StockTwits sentiment (BaseScore=18 = 8 mention + 10 sentiment).
	// After recompute, BaseScore should remain 18 (not compound to 28).
	svc := &SocialSignalService{
		entries: make(map[string]socialEntry),
		logger:  logrus.New(),
	}
	now := time.Now()
	svc.entries["TICK"] = socialEntry{
		BaseScore:  18.0,
		MentionPts: 8.0,
		DetectedAt: now,
		Context:    "mentions=8 velocity=1.6x st_bullish=70%",
	}
	// 8 TICK + 2 OTHER: total=10, avg=5, TICK velocity=1.6, mentionPts=min64(0.8,1)*10=8
	for i := 0; i < 8; i++ {
		svc.mentionWindow = append(svc.mentionWindow, mentionRecord{Ticker: "TICK", Timestamp: now})
	}
	for i := 0; i < 2; i++ {
		svc.mentionWindow = append(svc.mentionWindow, mentionRecord{Ticker: "OTHER", Timestamp: now})
	}
	svc.recomputeRedditScores(now)
	got := svc.entries["TICK"].BaseScore
	if got > 20.0 {
		t.Errorf("BaseScore should not exceed 20, got %f", got)
	}
	if got != 18.0 {
		t.Errorf("expected 18.0 (8 mention + 10 prior sentiment, no compounding), got %f", got)
	}
	if svc.entries["TICK"].MentionPts != 8.0 {
		t.Errorf("expected MentionPts=8.0, got %f", svc.entries["TICK"].MentionPts)
	}
}
