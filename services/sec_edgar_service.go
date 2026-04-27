package services

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

const regulatoryRefreshInterval = 30 * time.Second
const regulatoryHalfLifeHours = 24.0

type regulatoryEntry struct {
	BaseScore  float64
	DetectedAt time.Time
	EventDesc  string
}

// SECEdgarService polls EDGAR and GlobeNewswire for regulatory events.
type SECEdgarService struct {
	httpClient *http.Client
	universe   *PennyUniverseService
	mu         sync.RWMutex
	entries    map[string]regulatoryEntry // keyed by ticker; keeps highest-score entry
	logger     *logrus.Logger
}

// NewSECEdgarService creates the service.
func NewSECEdgarService(universe *PennyUniverseService, httpClient *http.Client) *SECEdgarService {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	return &SECEdgarService{
		httpClient: httpClient,
		universe:   universe,
		entries:    make(map[string]regulatoryEntry),
		logger:     logger,
	}
}

// Start runs the polling loop until ctx is cancelled.
func (s *SECEdgarService) Start(ctx context.Context) {
	s.poll()
	ticker := time.NewTicker(regulatoryRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.poll()
		}
	}
}

// GetRegulatoryScore returns the current decayed regulatory score and event description.
func (s *SECEdgarService) GetRegulatoryScore(ticker string) (float64, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.entries[ticker]
	if !ok {
		return 0, ""
	}
	return scoreWithDecay(e.BaseScore, e.DetectedAt, regulatoryHalfLifeHours), e.EventDesc
}

func (s *SECEdgarService) poll() {
	tickers := tickerSet(s.universe.GetTickers())
	s.pollEdgar(tickers)
	s.pollGlobeNewswire(tickers)
}

// atomFeed is a minimal ATOM feed parser.
type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	Title   string `xml:"title"`
	Updated string `xml:"updated"`
	Summary string `xml:"summary"`
}

func (s *SECEdgarService) fetchAtom(url string) ([]atomEntry, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ProphetBot/1.0 (contact: trading@example.com)")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var feed atomFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("atom parse: %w", err)
	}
	return feed.Entries, nil
}

func (s *SECEdgarService) pollEdgar(tickers map[string]bool) {
	const edgarURL = "https://www.sec.gov/cgi-bin/browse-edgar?action=getcurrent&type=8-K&dateb=&owner=include&count=40&search_text=&output=atom"
	entries, err := s.fetchAtom(edgarURL)
	if err != nil {
		s.logger.WithError(err).Warn("SECEdgarService: EDGAR poll failed")
		return
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, entry := range entries {
		ticker := extractTickerFromTitle(entry.Title, tickers)
		if ticker == "" {
			continue
		}
		desc := fmt.Sprintf("8-K filed %s", now.Format("15:04 ET"))
		s.upsertEntry(ticker, 40.0, now, desc)
	}
}

func (s *SECEdgarService) pollGlobeNewswire(tickers map[string]bool) {
	const gnwURL = "https://www.globenewswire.com/RssFeed/country/US"
	entries, err := s.fetchAtom(gnwURL)
	if err != nil {
		s.logger.WithError(err).Warn("SECEdgarService: GlobeNewswire poll failed")
		return
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, entry := range entries {
		combined := entry.Title + " " + entry.Summary
		for ticker := range tickers {
			if strings.Contains(combined, ticker) {
				desc := fmt.Sprintf("PR wire mention %s", now.Format("15:04 ET"))
				s.upsertEntry(ticker, 25.0, now, desc)
			}
		}
	}
}

// upsertEntry keeps the highest-score entry per ticker. Caller must hold mu.Lock.
func (s *SECEdgarService) upsertEntry(ticker string, base float64, now time.Time, desc string) {
	existing, ok := s.entries[ticker]
	if !ok || base > existing.BaseScore {
		s.entries[ticker] = regulatoryEntry{BaseScore: base, DetectedAt: now, EventDesc: desc}
	}
}

// extractTickerFromTitle finds a universe ticker in an EDGAR entry title.
// EDGAR 8-K titles look like: "8-K - ACME CORP (0001234567) (Issuer)"
func extractTickerFromTitle(title string, tickers map[string]bool) string {
	upper := strings.ToUpper(title)
	for ticker := range tickers {
		if strings.Contains(upper, " "+ticker+" ") ||
			strings.Contains(upper, "("+ticker+")") ||
			strings.HasSuffix(upper, " "+ticker) {
			return ticker
		}
	}
	return ""
}

// tickerSet converts a slice of ticker strings into a set (map) for O(1) lookup.
func tickerSet(tickers []string) map[string]bool {
	set := make(map[string]bool, len(tickers))
	for _, t := range tickers {
		set[t] = true
	}
	return set
}
