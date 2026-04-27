package services

import (
	"context"
	"prophet-trader/interfaces"
	"testing"
	"time"
)

// --- stubs ---

type stubLister struct {
	positions []*ManagedPosition
}

func (s *stubLister) ListManagedPositions(_ string) []*ManagedPosition {
	return s.positions
}

type stubTrading struct {
	portfolio float64
}

func (s *stubTrading) GetAccount(_ context.Context) (*interfaces.Account, error) {
	return &interfaces.Account{PortfolioValue: s.portfolio}, nil
}
func (s *stubTrading) PlaceOrder(_ context.Context, _ *interfaces.Order) (*interfaces.OrderResult, error) {
	return nil, nil
}
func (s *stubTrading) CancelOrder(_ context.Context, _ string) error { return nil }
func (s *stubTrading) GetOrder(_ context.Context, _ string) (*interfaces.Order, error) {
	return nil, nil
}
func (s *stubTrading) ListOrders(_ context.Context, _ string) ([]*interfaces.Order, error) {
	return nil, nil
}
func (s *stubTrading) GetPositions(_ context.Context) ([]*interfaces.Position, error) {
	return nil, nil
}
func (s *stubTrading) PlaceOptionsOrder(_ context.Context, _ *interfaces.OptionsOrder) (*interfaces.OrderResult, error) {
	return nil, nil
}
func (s *stubTrading) GetOptionsChain(_ context.Context, _ string, _ time.Time) ([]*interfaces.OptionContract, error) {
	return nil, nil
}
func (s *stubTrading) GetOptionsQuote(_ context.Context, _ string) (*interfaces.OptionsQuote, error) {
	return nil, nil
}
func (s *stubTrading) GetOptionsPosition(_ context.Context, _ string) (*interfaces.OptionsPosition, error) {
	return nil, nil
}
func (s *stubTrading) ListOptionsPositions(_ context.Context) ([]*interfaces.OptionsPosition, error) {
	return nil, nil
}

// --- helpers ---

func managedPos(symbol string, agent AgentSource, status string, allocation float64) *ManagedPosition {
	return &ManagedPosition{
		Symbol:            symbol,
		Status:            status,
		AllocationDollars: allocation,
		Tags:              []string{AgentTag(agent)},
	}
}

func defaultConfig() TradeGuardConfig {
	return TradeGuardConfig{
		PennyMaxCapitalPct:      0.20,
		PennyMaxPositionDollars: 500,
	}
}

// --- tests ---

func TestGuard_PennyCannotBuyMainSymbol(t *testing.T) {
	lister := &stubLister{
		positions: []*ManagedPosition{
			managedPos("AAPL", AgentMain, "ACTIVE", 1000),
		},
	}
	g := NewTradeGuard(lister, &stubTrading{portfolio: 10000}, defaultConfig())
	err := g.CheckBuy(context.Background(), AgentPenny, "AAPL", 100)
	if err == nil {
		t.Fatal("expected error: penny buying main symbol")
	}
}

func TestGuard_MainCannotBuyPennySymbol(t *testing.T) {
	lister := &stubLister{
		positions: []*ManagedPosition{
			managedPos("MEME", AgentPenny, "ACTIVE", 200),
		},
	}
	g := NewTradeGuard(lister, &stubTrading{portfolio: 10000}, defaultConfig())
	err := g.CheckBuy(context.Background(), AgentMain, "MEME", 500)
	if err == nil {
		t.Fatal("expected error: main buying penny symbol")
	}
}

func TestGuard_PennyExceedsPerPositionCap(t *testing.T) {
	g := NewTradeGuard(&stubLister{}, &stubTrading{portfolio: 100000}, defaultConfig())
	err := g.CheckBuy(context.Background(), AgentPenny, "XYZ", 600) // cap is 500
	if err == nil {
		t.Fatal("expected error: penny position exceeds per-position cap")
	}
}

func TestGuard_PennyExceedsCapitalCap(t *testing.T) {
	lister := &stubLister{
		positions: []*ManagedPosition{
			managedPos("AAA", AgentPenny, "ACTIVE", 1800), // already $1800 of $2000 cap
		},
	}
	g := NewTradeGuard(lister, &stubTrading{portfolio: 10000}, defaultConfig()) // cap = 20% * 10000 = 2000
	err := g.CheckBuy(context.Background(), AgentPenny, "BBB", 300)             // 1800+300 > 2000
	if err == nil {
		t.Fatal("expected error: penny capital cap exceeded")
	}
}

func TestGuard_PennyBuyAllowed(t *testing.T) {
	lister := &stubLister{
		positions: []*ManagedPosition{
			managedPos("AAA", AgentPenny, "ACTIVE", 500),
		},
	}
	g := NewTradeGuard(lister, &stubTrading{portfolio: 10000}, defaultConfig()) // cap = 2000
	err := g.CheckBuy(context.Background(), AgentPenny, "BBB", 400)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGuard_ClosedPositionNotConflict(t *testing.T) {
	lister := &stubLister{
		positions: []*ManagedPosition{
			managedPos("AAPL", AgentMain, "CLOSED", 1000),
		},
	}
	g := NewTradeGuard(lister, &stubTrading{portfolio: 10000}, defaultConfig())
	err := g.CheckBuy(context.Background(), AgentPenny, "AAPL", 100)
	if err != nil {
		t.Fatalf("closed position should not block: %v", err)
	}
}

func TestGuard_SellBlockedByOpponent(t *testing.T) {
	lister := &stubLister{
		positions: []*ManagedPosition{
			managedPos("TSLA", AgentMain, "ACTIVE", 1000),
		},
	}
	g := NewTradeGuard(lister, nil, defaultConfig())
	err := g.CheckSell(context.Background(), AgentPenny, "TSLA")
	if err == nil {
		t.Fatal("expected error: penny selling main-owned symbol")
	}
}

func TestGuard_RawOrderTracking(t *testing.T) {
	g := NewTradeGuard(&stubLister{}, nil, defaultConfig())
	g.RecordRawBuy(AgentMain, "NVDA")

	// Penny should not be able to buy NVDA now
	err := g.CheckBuy(context.Background(), AgentPenny, "NVDA", 100)
	if err == nil {
		t.Fatal("expected error: penny buying raw-main symbol")
	}

	// After main sells, penny should be able to buy
	g.RecordRawSell(AgentMain, "NVDA")
	err = g.CheckBuy(context.Background(), AgentPenny, "NVDA", 100)
	if err != nil {
		t.Fatalf("expected no error after raw sell: %v", err)
	}
}

func TestGuard_UntaggedPositionTreatedAsMain(t *testing.T) {
	// Legacy position with no agent tag should be owned by main
	lister := &stubLister{
		positions: []*ManagedPosition{
			{Symbol: "IBM", Status: "ACTIVE", AllocationDollars: 500, Tags: []string{}},
		},
	}
	g := NewTradeGuard(lister, nil, defaultConfig())
	err := g.CheckBuy(context.Background(), AgentPenny, "IBM", 100)
	if err == nil {
		t.Fatal("expected error: untagged position should block penny")
	}
}

func TestGuard_EmptyAgentSourceDefaultsToMain(t *testing.T) {
	lister := &stubLister{
		positions: []*ManagedPosition{
			managedPos("GOOG", AgentPenny, "ACTIVE", 200),
		},
	}
	g := NewTradeGuard(lister, nil, defaultConfig())
	// Empty agent source = main; penny holds GOOG → should block
	err := g.CheckBuy(context.Background(), "", "GOOG", 500)
	if err == nil {
		t.Fatal("expected error: empty agent treated as main, cannot buy penny symbol")
	}
}
