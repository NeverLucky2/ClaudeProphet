package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"prophet-trader/services"

	"github.com/gin-gonic/gin"
)

func setupPennyRouter(agg *services.PennySignalAggregator) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	pc := NewPennyController(agg)
	r.GET("/api/v1/penny/candidates", pc.HandleGetCandidates)
	r.GET("/api/v1/penny/signal/:ticker", pc.HandleGetSignalDetail)
	r.GET("/api/v1/penny/universe", pc.HandleGetUniverse)
	r.POST("/api/v1/penny/scan", pc.HandleScanNow)
	return r
}

// emptyAggregator creates an aggregator with zero-value sub-services.
// Safe because these tests only exercise the HTTP layer — aggregate() is never called,
// so the unexported maps and clients on the sub-services are never accessed.
func emptyAggregator() *services.PennySignalAggregator {
	return services.NewPennySignalAggregator(
		&services.PennyUniverseService{},
		&services.PennyScreenerService{},
		&services.SECEdgarService{},
		&services.SocialSignalService{},
	)
}

func parseBody(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}
	return body
}

func TestPennyController_GetCandidates_Empty(t *testing.T) {
	r := setupPennyRouter(emptyAggregator())
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/penny/candidates?min_score=60", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := parseBody(t, w)
	if body["count"].(float64) != 0 {
		t.Errorf("expected count=0, got %v", body["count"])
	}
}

func TestPennyController_GetSignalDetail_NotFound(t *testing.T) {
	r := setupPennyRouter(emptyAggregator())
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/penny/signal/NONE", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestPennyController_InvalidMinScore(t *testing.T) {
	r := setupPennyRouter(emptyAggregator())
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/penny/candidates?min_score=abc", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestPennyController_ScanNow(t *testing.T) {
	r := setupPennyRouter(emptyAggregator())
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/penny/scan", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := parseBody(t, w)
	if body["status"] != "refreshing" {
		t.Errorf("expected status=refreshing, got %v", body["status"])
	}
}

func TestPennyController_GetUniverse_Empty(t *testing.T) {
	r := setupPennyRouter(emptyAggregator())
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/penny/universe", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := parseBody(t, w)
	if body["count"].(float64) != 0 {
		t.Errorf("expected count=0, got %v", body["count"])
	}
}
