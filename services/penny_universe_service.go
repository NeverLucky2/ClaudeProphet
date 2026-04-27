package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

const universeRefreshInterval = 15 * time.Minute

type fmpScreenerItem struct {
	Symbol            string  `json:"symbol"`
	CompanyName       string  `json:"companyName"`
	MarketCap         float64 `json:"marketCap"`
	Price             float64 `json:"price"`
	Volume            float64 `json:"volume"` // 30-day avg share volume from FMP
	ExchangeShortName string  `json:"exchangeShortName"`
}

// PennyUniverseService maintains a filtered universe of penny stocks.
type PennyUniverseService struct {
	httpClient *http.Client
	fmpAPIKey  string
	mu         sync.RWMutex
	universe   []UniverseSymbol
	logger     *logrus.Logger
}

// NewPennyUniverseService creates the service. Pass a custom httpClient for testing.
func NewPennyUniverseService(fmpAPIKey string, httpClient *http.Client) *PennyUniverseService {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	return &PennyUniverseService{
		httpClient: httpClient,
		fmpAPIKey:  fmpAPIKey,
		logger:     logger,
	}
}

// Start runs the refresh loop until ctx is cancelled.
func (s *PennyUniverseService) Start(ctx context.Context) {
	s.refresh()
	ticker := time.NewTicker(universeRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refresh()
		}
	}
}

// GetUniverse returns a copy of the current universe.
func (s *PennyUniverseService) GetUniverse() []UniverseSymbol {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]UniverseSymbol, len(s.universe))
	copy(out, s.universe)
	return out
}

// GetTickers returns just the ticker symbols.
func (s *PennyUniverseService) GetTickers() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tickers := make([]string, len(s.universe))
	for i, u := range s.universe {
		tickers[i] = u.Ticker
	}
	return tickers
}

func (s *PennyUniverseService) refresh() {
	url := fmt.Sprintf(
		"https://financialmodelingprep.com/api/v3/stock-screener?marketCapMoreThan=50000000&marketCapLowerThan=500000000&priceMoreThan=2&priceLowerThan=10&exchange=NASDAQ,NYSE,AMEX&country=US&limit=500&apikey=%s",
		s.fmpAPIKey,
	)
	resp, err := s.httpClient.Get(url)
	if err != nil {
		s.logger.WithError(err).Warn("PennyUniverseService: FMP request failed")
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		s.logger.WithError(err).Warn("PennyUniverseService: failed to read FMP response")
		return
	}
	var items []fmpScreenerItem
	if err := json.Unmarshal(body, &items); err != nil {
		s.logger.WithError(err).Warn("PennyUniverseService: failed to parse FMP response")
		return
	}
	universe := s.filter(items)
	s.mu.Lock()
	s.universe = universe
	s.mu.Unlock()
	s.logger.WithField("count", len(universe)).Info("PennyUniverseService: universe refreshed")
}

var allowedExchanges = map[string]bool{
	"NASDAQ": true,
	"NYSE":   true,
	"AMEX":   true,
}

func (s *PennyUniverseService) filter(items []fmpScreenerItem) []UniverseSymbol {
	var out []UniverseSymbol
	for _, item := range items {
		if !allowedExchanges[item.ExchangeShortName] {
			continue
		}
		if item.Price < 2.0 || item.Price > 10.0 {
			continue
		}
		if item.MarketCap < 50_000_000 || item.MarketCap > 500_000_000 {
			continue
		}
		dollarVol := item.Volume * item.Price
		if dollarVol < 300_000 {
			continue
		}
		out = append(out, UniverseSymbol{
			Ticker:       item.Symbol,
			Name:         item.CompanyName,
			Exchange:     item.ExchangeShortName,
			Price:        item.Price,
			MarketCapM:   item.MarketCap / 1_000_000,
			AvgDollarVol: dollarVol,
		})
	}
	return out
}
