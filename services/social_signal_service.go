package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

const socialRefreshInterval = 30 * time.Second
const stockTwitsRefreshInterval = 2 * time.Minute
const socialHalfLifeHours = 4.0
const mentionWindowDuration = 30 * time.Minute

var tickerRegex = regexp.MustCompile(`\$([A-Z]{2,5})\b`)

type mentionRecord struct {
	Ticker    string
	Timestamp time.Time
}

type socialEntry struct {
	BaseScore  float64
	DetectedAt time.Time
	Context    string
}

// SocialSignalService polls Reddit and StockTwits for social signals.
type SocialSignalService struct {
	httpClient    *http.Client
	universe      *PennyUniverseService
	mu            sync.RWMutex
	entries       map[string]socialEntry
	mentionWindow []mentionRecord // sliding 30-min window of Reddit mentions
	logger        *logrus.Logger
}

// NewSocialSignalService creates the service.
func NewSocialSignalService(universe *PennyUniverseService, httpClient *http.Client) *SocialSignalService {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	return &SocialSignalService{
		httpClient: httpClient,
		universe:   universe,
		entries:    make(map[string]socialEntry),
		logger:     logger,
	}
}

// Start runs both Reddit and StockTwits loops until ctx is cancelled.
func (s *SocialSignalService) Start(ctx context.Context) {
	go s.runReddit(ctx)
	go s.runStockTwits(ctx)
	<-ctx.Done()
}

// GetSocialScore returns the current decayed social score and context for a ticker.
func (s *SocialSignalService) GetSocialScore(ticker string) (float64, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.entries[ticker]
	if !ok {
		return 0, ""
	}
	return scoreWithDecay(e.BaseScore, e.DetectedAt, socialHalfLifeHours), e.Context
}

func (s *SocialSignalService) runReddit(ctx context.Context) {
	s.pollReddit()
	ticker := time.NewTicker(socialRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.pollReddit()
		}
	}
}

func (s *SocialSignalService) runStockTwits(ctx context.Context) {
	ticker := time.NewTicker(stockTwitsRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.pollStockTwitsForTopMentioned()
		}
	}
}

type redditListing struct {
	Data struct {
		Children []struct {
			Data struct {
				Title    string `json:"title"`
				Selftext string `json:"selftext"`
			} `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

func (s *SocialSignalService) pollReddit() {
	subreddits := []string{"pennystocks", "RobinHoodPennyStocks"}
	tickers := tickerSet(s.universe.GetTickers())
	now := time.Now()
	var newMentions []mentionRecord

	for _, sub := range subreddits {
		url := fmt.Sprintf("https://www.reddit.com/r/%s/new.json?limit=100", sub)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "ProphetBot/1.0 (contact: trading@example.com)")
		resp, err := s.httpClient.Do(req)
		if err != nil {
			s.logger.WithError(err).Warnf("SocialSignalService: Reddit r/%s failed", sub)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			s.logger.WithField("status", resp.StatusCode).Warnf("SocialSignalService: Reddit r/%s returned non-200", sub)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var listing redditListing
		if err := json.Unmarshal(body, &listing); err != nil {
			continue
		}
		for _, child := range listing.Data.Children {
			combined := strings.ToUpper(child.Data.Title + " " + child.Data.Selftext)
			for _, m := range tickerRegex.FindAllStringSubmatch(combined, -1) {
				if len(m) < 2 {
					continue
				}
				t := m[1]
				if tickers[t] {
					newMentions = append(newMentions, mentionRecord{Ticker: t, Timestamp: now})
				}
			}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.mentionWindow = append(s.mentionWindow, newMentions...)
	s.pruneWindow(now)
	s.recomputeRedditScores(now)
}

func (s *SocialSignalService) pruneWindow(now time.Time) {
	cutoff := now.Add(-mentionWindowDuration)
	i := 0
	for i < len(s.mentionWindow) && s.mentionWindow[i].Timestamp.Before(cutoff) {
		i++
	}
	s.mentionWindow = s.mentionWindow[i:]
}

func (s *SocialSignalService) recomputeRedditScores(now time.Time) {
	counts := make(map[string]int)
	for _, m := range s.mentionWindow {
		counts[m.Ticker]++
	}
	total := 0
	for _, c := range counts {
		total += c
	}
	avgCount := 1
	if len(counts) > 0 {
		avg := total / len(counts)
		if avg > 1 {
			avgCount = avg
		}
	}
	for ticker, count := range counts {
		velocity := float64(count) / float64(avgCount)
		mentionPts := min64(velocity/2.0, 1.0) * 10.0
		// Preserve any existing StockTwits sentiment by reading the old entry's sentimentPts component.
		// sentimentPts is the portion of the existing score above the mention portion.
		var sentimentPts float64
		if existing, ok := s.entries[ticker]; ok {
			oldMentionPts := min64((float64(count)/float64(avgCount))/2.0, 1.0) * 10.0
			sentimentPts = existing.BaseScore - oldMentionPts
			if sentimentPts < 0 {
				sentimentPts = 0
			}
		}
		score := min64(mentionPts+sentimentPts, 20.0)
		signalCtx := fmt.Sprintf("mentions=%d velocity=%.1fx", count, velocity)
		s.entries[ticker] = socialEntry{BaseScore: score, DetectedAt: now, Context: signalCtx}
	}
}

func (s *SocialSignalService) pollStockTwitsForTopMentioned() {
	s.mu.RLock()
	type kv struct {
		ticker string
		score  float64
	}
	var ranked []kv
	for t, e := range s.entries {
		ranked = append(ranked, kv{t, e.BaseScore})
	}
	s.mu.RUnlock()

	sort.Slice(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })
	limit := 5
	if len(ranked) < limit {
		limit = len(ranked)
	}
	for i := 0; i < limit; i++ {
		s.fetchStockTwits(ranked[i].ticker)
	}
}

type stResponse struct {
	Messages []struct {
		Entities struct {
			Sentiment *struct {
				Basic string `json:"basic"`
			} `json:"sentiment"`
		} `json:"entities"`
	} `json:"messages"`
}

func (s *SocialSignalService) fetchStockTwits(ticker string) {
	url := fmt.Sprintf("https://api.stocktwits.com/api/2/streams/symbol/%s.json", ticker)
	resp, err := s.httpClient.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	body, _ := io.ReadAll(resp.Body)
	var st stResponse
	if err := json.Unmarshal(body, &st); err != nil {
		return
	}
	bullish, bearish := 0, 0
	for _, m := range st.Messages {
		if m.Entities.Sentiment == nil {
			continue
		}
		switch m.Entities.Sentiment.Basic {
		case "Bullish":
			bullish++
		case "Bearish":
			bearish++
		}
	}
	total := bullish + bearish
	if total == 0 {
		return
	}
	ratio := float64(bullish) / float64(total)
	var sentimentPts float64
	if ratio > 0.65 {
		sentimentPts = 10.0
	} else if ratio > 0.55 {
		sentimentPts = 5.0
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	existing := s.entries[ticker]
	// Re-derive mention points from context to avoid compounding sentiment across updates.
	// Add sentiment on top of the existing mention-only score (capped at 20).
	newScore := min64(existing.BaseScore+sentimentPts, 20.0)
	signalCtx := fmt.Sprintf("%s st_bullish=%.0f%%", existing.Context, ratio*100)
	s.entries[ticker] = socialEntry{BaseScore: newScore, DetectedAt: existing.DetectedAt, Context: signalCtx}
}

func min64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
