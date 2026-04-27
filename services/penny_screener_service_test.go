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
	score, entry := svc.computeEntry("TEST", snap)
	// volumeRatio=5.0 → volScore=20; gapPct=10% → gapScore=10; breakoutBonus=0 (close 5.9 < high 6.0 - 2%)
	if score < 25 {
		t.Errorf("expected score >=25 for high-volume entry, got %f", score)
	}
	if entry.VolumeRatio != 5.0 {
		t.Errorf("expected volumeRatio=5.0, got %f", entry.VolumeRatio)
	}
}

func TestPennyScreenerService_ComputeEntry_NilSnapshot(t *testing.T) {
	svc := &PennyScreenerService{scores: make(map[string]TechnicalEntry), logger: newTestLogger()}
	score, _ := svc.computeEntry("TEST", nil)
	if score != 0 {
		t.Errorf("expected 0 for nil snapshot, got %f", score)
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
