package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthRespondeOK(t *testing.T) {
	mux := newMux(config{port: "8080", analyticsURL: "http://analytics:8081"})

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
