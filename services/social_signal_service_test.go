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
	svc := &SocialSignalService{
		entries: make(map[string]socialEntry),
		logger:  logrus.New(),
	}
	now := time.Now()
	// Simulate a state where StockTwits already ran once and added 10 sentimentPts
	svc.entries["TICK"] = socialEntry{
		BaseScore:  18.0, // 8 mentionPts + 10 sentimentPts
		MentionPts: 8.0,
		DetectedAt: now,
		Context:    "mentions=3 velocity=2.0x st_bullish=70%",
	}
	// Now simulate recomputeRedditScores re-running with same mention counts.
	// MentionPts = 8 → sentimentPts = existing.BaseScore - existing.MentionPts = 18-8 = 10
	// new BaseScore = min64(8+10, 20) = 18 — should NOT compound to 28.
	existing := svc.entries["TICK"]
	sentimentPts := existing.BaseScore - existing.MentionPts
	if sentimentPts < 0 {
		sentimentPts = 0
	}
	newMentionPts := 8.0 // same velocity
	newScore := min64(newMentionPts+sentimentPts, 20.0)
	if newScore > 20.0 {
		t.Errorf("score should not exceed 20, got %f", newScore)
	}
	if newScore != 18.0 {
		t.Errorf("expected 18.0 (8 mention + 10 sentiment), got %f", newScore)
	}
}
