package services

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestExtractTickerFromTitle(t *testing.T) {
	tickers := map[string]bool{"ACME": true, "FOO": true}
	tests := []struct {
		title string
		want  string
	}{
		{"8-K - ACME CORP (Issuer)", "ACME"},
		{"8-K - BORING INC (Issuer)", ""},
		{"8-K - (FOO) Corp", "FOO"},
	}
	for _, tc := range tests {
		got := extractTickerFromTitle(tc.title, tickers)
		if got != tc.want {
			t.Errorf("extractTickerFromTitle(%q) = %q, want %q", tc.title, got, tc.want)
		}
	}
}

func TestSECEdgarService_GetRegulatoryScore_Decay(t *testing.T) {
	svc := &SECEdgarService{
		entries: make(map[string]regulatoryEntry),
		logger:  logrus.New(),
	}
	svc.entries["TICK"] = regulatoryEntry{BaseScore: 40.0, DetectedAt: time.Now(), EventDesc: "test"}
	score, desc := svc.GetRegulatoryScore("TICK")
	if score < 39 || score > 40 {
		t.Errorf("fresh entry: expected ~40, got %f", score)
	}
	if desc != "test" {
		t.Errorf("expected desc 'test', got %q", desc)
	}
}

func TestSECEdgarService_UpsertEntry_KeepsHigher(t *testing.T) {
	svc := &SECEdgarService{entries: make(map[string]regulatoryEntry), logger: logrus.New()}
	now := time.Now()
	svc.upsertEntry("T", 25.0, now, "pr wire")
	svc.upsertEntry("T", 40.0, now, "8-K")
	svc.upsertEntry("T", 10.0, now, "lower")
	if svc.entries["T"].BaseScore != 40.0 {
		t.Errorf("expected 40.0, got %f", svc.entries["T"].BaseScore)
	}
	if svc.entries["T"].EventDesc != "8-K" {
		t.Errorf("expected '8-K', got %q", svc.entries["T"].EventDesc)
	}
}

func TestSECEdgarService_FetchAtom_NonOK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer ts.Close()

	svc := NewSECEdgarService(nil, ts.Client())
	entries, err := svc.fetchAtom(ts.URL)
	if err == nil {
		t.Error("expected error for non-200 response, got nil")
	}
	if entries != nil {
		t.Errorf("expected nil entries on error, got %v", entries)
	}
}
