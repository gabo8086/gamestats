// Modulo de ingestion de GameStats (paradigma imperativo/concurrente).
//
// Estado: esqueleto de la Fase 0. Solo levanta el servidor y responde /health, lo justo para que
// docker compose y el CI tengan algo que verificar. Lo que falta, en orden (ver CLAUDE.md):
//
//	Dia 1: POST /events -> validacion del sobre y catalogo de rechazos (contracts/rejections.md).
//	Dia 2: una goroutine por partida, con un despachador que enruta por matchId a traves de channels,
//	       y el estado por partida (jugadores activos, eventos acumulados, si inicio/termino).
//	Dia 3: al llegar MATCH_FINISHED, ordenar los eventos por timestamp y hacer POST /analyze a Scala.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"
)

// config es todo lo que el modulo lee del entorno. Dentro de docker compose los servicios se
// llaman por nombre, nunca localhost: de ahi el valor por defecto de analyticsURL.
type config struct {
	port         string
	analyticsURL string
}

func loadConfig() config {
	return config{
		port:         envOr("PORT", "8080"),
		analyticsURL: envOr("ANALYTICS_URL", "http://analytics:8081"),
	}
}

func envOr(clave, porDefecto string) string {
	if v := os.Getenv(clave); v != "" {
		return v
	}
	return porDefecto
}

func newMux(cfg config) *http.ServeMux {
	mux := http.NewServeMux()

	// /health existe para el CI y para los healthchecks de docker compose.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "ok",
			"service": "ingest-go",
		})
	})

	// Declarado aqui para que el contrato de la API sea visible desde la Fase 0, aunque la
	// implementacion llegue el Dia 1.
	mux.HandleFunc("POST /events", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotImplemented, map[string]string{
			"error": "POST /events todavia no esta implementado (Dia 1)",
		})
	})

	return mux
}

func writeJSON(w http.ResponseWriter, status int, cuerpo any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(cuerpo); err != nil {
		log.Printf("no se pudo escribir la respuesta: %v", err)
	}
}

func main() {
	cfg := loadConfig()

	srv := &http.Server{
		Addr:              ":" + cfg.port,
		Handler:           newMux(cfg),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("ingest-go escuchando en :%s (analytics en %s)", cfg.port, cfg.analyticsURL)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("el servidor se detuvo: %v", err)
	}
}
