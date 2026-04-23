package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ClaudeService handles interactions with Anthropic's Claude AI API
type ClaudeService struct {
	apiKey     string
	httpClient *http.Client
	model      string
}

// claudeMessageRequest represents a request to the Claude Messages API
type claudeMessageRequest struct {
	Model     string           `json:"model"`
	MaxTokens int              `json:"max_tokens"`
	System    string           `json:"system,omitempty"`
	Messages  []claudeMessage  `json:"messages"`
}

// claudeMessage represents a single message in the conversation
type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// claudeMessageResponse represents the response from the Claude Messages API
type claudeMessageResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// CleanedNews represents a token-efficient news summary
type CleanedNews struct {
	GeneratedAt      time.Time         `json:"generated_at"`
	SourceCount      int               `json:"source_count"`
	ArticleCount     int               `json:"article_count"`
	MarketSentiment  string            `json:"market_sentiment"`
	KeyThemes        []string          `json:"key_themes"`
	StockMentions    map[string]string `json:"stock_mentions"`
	ActionableItems  []string          `json:"actionable_items"`
	ExecutiveSummary string            `json:"executive_summary"`
	FullAnalysis     string            `json:"full_analysis"`
}

// NewClaudeService creates a new Claude service
func NewClaudeService(apiKey string) *ClaudeService {
	if apiKey == "" {
		apiKey = os.Getenv("CLAUDE_API_KEY")
	}

	return &ClaudeService{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		model: "claude-haiku-4-5-20251001",
	}
}

// CleanNewsForTrading takes raw news items and creates a token-efficient summary
// optimized for trading decisions
func (cs *ClaudeService) CleanNewsForTrading(newsItems []NewsItem) (*CleanedNews, error) {
	if len(newsItems) == 0 {
		return nil, fmt.Errorf("no news items provided")
	}

	// Build the news text
	var newsText strings.Builder
	for i, item := range newsItems {
		newsText.WriteString(fmt.Sprintf("[%d] %s\n", i+1, item.Title))
		if item.Description != "" {
			// Clean HTML tags from description
			cleanDesc := strings.ReplaceAll(item.Description, "<", "")
			cleanDesc = strings.ReplaceAll(cleanDesc, ">", "")
			newsText.WriteString(fmt.Sprintf("   %s\n", cleanDesc[:minLen(200, len(cleanDesc))]))
		}
		newsText.WriteString(fmt.Sprintf("   Source: %s | Published: %s\n\n", item.Source, item.PubDate))
	}

	systemPrompt := `You are a financial analyst AI. Your role is to analyze news articles and produce concise, structured trading intelligence reports in JSON format. Focus on stock symbols, market sentiment, and actionable trading insights. Always respond with valid JSON only — no markdown, no explanation outside the JSON.`

	userPrompt := fmt.Sprintf(`Analyze the following %d news articles and create a CONCISE trading intelligence report.

NEWS ARTICLES:
%s

Provide a JSON response with this EXACT structure:
{
  "market_sentiment": "BULLISH|BEARISH|NEUTRAL",
  "key_themes": ["theme1", "theme2", "theme3"],
  "stock_mentions": {
    "SYMBOL": "POSITIVE|NEGATIVE|NEUTRAL with 1-sentence reason"
  },
  "actionable_items": ["brief actionable insight 1", "brief actionable insight 2"],
  "executive_summary": "2-3 sentence summary of the market situation"
}

Focus on:
- Stock symbols and their sentiment
- Market-moving themes
- Actionable trading insights
- Overall market direction

Keep it BRIEF and DENSE. Maximum 200 tokens total.`, len(newsItems), newsText.String())

	// Call Claude
	response, err := cs.generateContent(systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("failed to generate content: %w", err)
	}

	// Parse the JSON response
	var cleanedNews CleanedNews
	cleanedNews.GeneratedAt = time.Now()
	cleanedNews.SourceCount = countUniqueSources(newsItems)
	cleanedNews.ArticleCount = len(newsItems)
	cleanedNews.FullAnalysis = response

	// Try to extract JSON from the response
	jsonStart := strings.Index(response, "{")
	jsonEnd := strings.LastIndex(response, "}")
	if jsonStart >= 0 && jsonEnd > jsonStart {
		jsonStr := response[jsonStart : jsonEnd+1]
		var parsed struct {
			MarketSentiment  string            `json:"market_sentiment"`
			KeyThemes        []string          `json:"key_themes"`
			StockMentions    map[string]string `json:"stock_mentions"`
			ActionableItems  []string          `json:"actionable_items"`
			ExecutiveSummary string            `json:"executive_summary"`
		}
		if err := json.Unmarshal([]byte(jsonStr), &parsed); err == nil {
			cleanedNews.MarketSentiment = parsed.MarketSentiment
			cleanedNews.KeyThemes = parsed.KeyThemes
			cleanedNews.StockMentions = parsed.StockMentions
			cleanedNews.ActionableItems = parsed.ActionableItems
			cleanedNews.ExecutiveSummary = parsed.ExecutiveSummary
		}
	}

	return &cleanedNews, nil
}

// generateContent calls the Claude Messages API
func (cs *ClaudeService) generateContent(systemPrompt, userPrompt string) (string, error) {
	reqBody := claudeMessageRequest{
		Model:     cs.model,
		MaxTokens: 1024,
		System:    systemPrompt,
		Messages: []claudeMessage{
			{Role: "user", Content: userPrompt},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", cs.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := cs.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	var claudeResp claudeMessageResponse
	if err := json.Unmarshal(body, &claudeResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(claudeResp.Content) == 0 {
		return "", fmt.Errorf("no content in response")
	}

	return claudeResp.Content[0].Text, nil
}

// Helper functions
func minLen(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func countUniqueSources(items []NewsItem) int {
	sources := make(map[string]bool)
	for _, item := range items {
		if item.Source != "" {
			sources[item.Source] = true
		}
	}
	return len(sources)
}
