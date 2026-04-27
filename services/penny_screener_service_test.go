package services

import (
	"testing"
	"time"

	alpacaMarket "github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	"github.com/sirupsen/logrus"
)

func newTestLogger() *logrus.Logger {
	return logrus.New()
}

func TestPennyScreenerService_ComputeEntry_HighVolume(t *testing.T) {
	svc := &PennyScreenerService{
		scores: make(map[string]TechnicalEntry),
		logger: newTestLogger(),
	}
	snap := &alpacaMarket.Snapshot{
		DailyBar: &alpacaMarket.Bar{
			Open: 5.5, High: 6.0, Low: 5.0, Close: 5.9,
			Volume: 500_000,
		},
		PrevDailyBar: &alpacaMarket.Bar{
			Open: 5.0, High: 5.2, Low: 4.8, Close: 5.0,
			Volume: 100_000,
		},
	}
	entry := svc.computeEntry("TEST", snap)
	// volumeRatio=5.0 → volScore=20; gapPct=10% → gapScore=10; distFromHigh=(6.0-5.9)/6.0≈0.0167 ≤ 0.02 → breakoutScore=10; total=40
	if entry.Score != 40.0 {
		t.Errorf("expected score=40.0 for high-volume entry, got %f", entry.Score)
	}
	if entry.VolumeRatio != 5.0 {
		t.Errorf("expected volumeRatio=5.0, got %f", entry.VolumeRatio)
	}
}

func TestPennyScreenerService_ComputeEntry_NilSnapshot(t *testing.T) {
	svc := &PennyScreenerService{scores: make(map[string]TechnicalEntry), logger: newTestLogger()}
	entry := svc.computeEntry("TEST", nil)
	if entry.Score != 0 {
		t.Errorf("expected 0 for nil snapshot, got %f", entry.Score)
	}
}

func TestPennyScreenerService_ComputeEntry_PartialNil(t *testing.T) {
	svc := &PennyScreenerService{scores: make(map[string]TechnicalEntry), logger: newTestLogger()}
	snap := &alpacaMarket.Snapshot{
		DailyBar: &alpacaMarket.Bar{Open: 5.5, High: 6.0, Low: 5.0, Close: 5.9, Volume: 100_000},
		// PrevDailyBar intentionally nil
	}
	entry := svc.computeEntry("TEST", snap)
	if entry.Score != 0 {
		t.Errorf("expected 0 score for partial-nil snapshot, got %f", entry.Score)
	}
}

func TestPennyScreenerService_GetTechnicalScore_Decay(t *testing.T) {
	svc := &PennyScreenerService{scores: make(map[string]TechnicalEntry), logger: newTestLogger()}
	// 2-hour-old entry with 2h half-life → score should be ~half
	svc.scores["STALE"] = TechnicalEntry{Score: 40.0, UpdatedAt: time.Now().Add(-2 * time.Hour)}
	got, _ := svc.GetTechnicalScore("STALE")
	if got < 18 || got > 22 {
		t.Errorf("expected ~20 at half-life, got %f", got)
	}
}
