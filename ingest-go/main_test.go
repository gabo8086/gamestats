package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gamestats/ingest/internal/contract"
	"gamestats/ingest/internal/ingest"
)

func TestHealthRespondeOK(t *testing.T) {
	mux := nuevoMux()

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d, se esperaba %d", rec.Code, http.StatusOK)
	}

	var cuerpo map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatalf("la respuesta no es JSON: %v", err)
	}
	if cuerpo["status"] != "ok" {
		t.Errorf("status = %q, se esperaba \"ok\"", cuerpo["status"])
	}
}

func TestLoadConfigUsaLosValoresPorDefecto(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("ANALYTICS_URL", "")

	cfg := loadConfig()

	if cfg.port != "8080" {
		t.Errorf("port = %q, se esperaba \"8080\"", cfg.port)
	}
	// Dentro de compose el host es el nombre del servicio, no localhost.
	if cfg.analyticsURL != "http://analytics:8081" {
		t.Errorf("analyticsURL = %q, se esperaba \"http://analytics:8081\"", cfg.analyticsURL)
	}
}

func TestLoadConfigRespetaElEntorno(t *testing.T) {
	t.Setenv("PORT", "9090")
	t.Setenv("ANALYTICS_URL", "http://otro:1234")

	cfg := loadConfig()

	if cfg.port != "9090" {
		t.Errorf("port = %q, se esperaba \"9090\"", cfg.port)
	}
	if cfg.analyticsURL != "http://otro:1234" {
		t.Errorf("analyticsURL = %q, se esperaba \"http://otro:1234\"", cfg.analyticsURL)
	}
}

func post(t *testing.T, mux http.Handler, cuerpo string) (int, eventsResponse) {
	t.Helper()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(cuerpo)))

	var resp eventsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("la respuesta no es JSON: %v", err)
	}
	return rec.Code, resp
}

const (
	inicio = `{"eventId":"e-1","timestamp":"2026-10-05T14:00:00Z","gameId":"racing","gameVersion":"1.0",` +
		`"matchId":"m-1","type":"MATCH_STARTED"}`
	union = `{"eventId":"e-2","timestamp":"2026-10-05T14:00:01Z","gameId":"racing","gameVersion":"1.0",` +
		`"matchId":"m-1","playerId":"p1","type":"PLAYER_JOINED","data":{"playerType":"HUMAN"}}`
)

func nuevoMux() *http.ServeMux {
	return newMux(ingest.NewDispatcher(func(contract.Batch) error { return nil }))
}

// El simulador manda un evento por peticion.
func TestPostEventsAceptaUnEventoSuelto(t *testing.T) {
	codigo, resp := post(t, nuevoMux(), inicio)
	if codigo != http.StatusOK || resp.Accepted != 1 || len(resp.Rejected) != 0 {
		t.Errorf("codigo = %d, respuesta = %+v", codigo, resp)
	}
}

// Un arreglo mezclado responde 200: los validos entran y los invalidos se listan con su razon.
func TestPostEventsArregloMezclado(t *testing.T) {
	codigo, resp := post(t, nuevoMux(), "["+inicio+","+union+","+inicio+"]")
	if codigo != http.StatusOK || resp.Accepted != 2 {
		t.Fatalf("codigo = %d, respuesta = %+v", codigo, resp)
	}
	if len(resp.Rejected) != 1 || resp.Rejected[0].Reason != contract.ReasonDuplicateEvent ||
		resp.Rejected[0].EventID != "e-1" {
		t.Errorf("rechazados = %+v", resp.Rejected)
	}
}

func TestPostEventsSinNingunoValidoResponde400(t *testing.T) {
	for _, cuerpo := range []string{`{"eventId":`, union, `[` + union + `]`} {
		codigo, resp := post(t, nuevoMux(), cuerpo)
		if codigo != http.StatusBadRequest || resp.Accepted != 0 || len(resp.Rejected) != 1 {
			t.Errorf("%s: codigo = %d, respuesta = %+v", cuerpo, codigo, resp)
		}
	}
}

func TestGetMetrics(t *testing.T) {
	mux := nuevoMux()
	post(t, mux, "["+inicio+","+inicio+"]")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("codigo = %d", rec.Code)
	}

	var m ingest.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("la respuesta no es JSON: %v", err)
	}
	if m.EventsReceived != 2 || m.EventsValid != 1 || m.ActiveMatches != 1 ||
		m.RejectionsByReason[contract.ReasonDuplicateEvent] != 1 {
		t.Errorf("metricas = %+v", m)
	}
}
