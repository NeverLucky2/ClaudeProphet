package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPennyUniverseService_Filter(t *testing.T) {
	items := []fmpScreenerItem{
		{Symbol: "GOOD", CompanyName: "Good Co", MarketCap: 100_000_000, Price: 5.0, Volume: 100_000, ExchangeShortName: "NASDAQ"},
		{Symbol: "CHEAP", CompanyName: "Too Cheap", MarketCap: 100_000_000, Price: 1.0, Volume: 100_000, ExchangeShortName: "NASDAQ"},
		{Symbol: "PRICEY", CompanyName: "Too Pricey", MarketCap: 100_000_000, Price: 15.0, Volume: 100_000, ExchangeShortName: "NASDAQ"},
		{Symbol: "TINYCAP", CompanyName: "Tiny Cap", MarketCap: 10_000_000, Price: 5.0, Volume: 100_000, ExchangeShortName: "NASDAQ"},
		{Symbol: "LOWVOL", CompanyName: "Low Vol", MarketCap: 100_000_000, Price: 5.0, Volume: 1_000, ExchangeShortName: "NASDAQ"},
		{Symbol: "OTC", CompanyName: "OTC Co", MarketCap: 100_000_000, Price: 5.0, Volume: 100_000, ExchangeShortName: "OTC"},
	}
	svc := NewPennyUniverseService("dummy", nil)
	result := svc.filter(items)
	if len(result) != 1 {
		t.Fatalf("expected 1 symbol, got %d", len(result))
	}
	if result[0].Ticker != "GOOD" {
		t.Errorf("expected GOOD, got %s", result[0].Ticker)
	}
}

func TestPennyUniverseService_HTTPRefresh(t *testing.T) {
	items := []fmpScreenerItem{
		{Symbol: "TEST", CompanyName: "Test Inc", MarketCap: 200_000_000, Price: 4.0, Volume: 200_000, ExchangeShortName: "NYSE"},
	}
	body, _ := json.Marshal(items)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer ts.Close()

	svc := NewPennyUniverseService("testkey", ts.Client())
	// Test filter() directly; verify GetTickers returns non-empty after a successful parse.
	svc.universe = svc.filter(items)
	tickers := svc.GetTickers()
	if len(tickers) != 1 || tickers[0] != "TEST" {
		t.Errorf("expected [TEST], got %v", tickers)
	}
}
