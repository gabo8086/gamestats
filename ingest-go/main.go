// Modulo de ingestion de GameStats (paradigma imperativo/concurrente).
//
// Este archivo es solo el borde HTTP: configuracion, rutas y traduccion entre JSON y el
// despachador. La logica vive en internal/contract (validacion del sobre) e internal/ingest
// (estado por partida, goroutines y envio a Scala).
//
//	POST /events   uno o varios eventos; responde cuantos se aceptaron y por que se rechazo el resto
//	GET  /metrics  metricas de la ingestion
//	GET  /health   para docker compose y el CI
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"gamestats/ingest/internal/contract"
	"gamestats/ingest/internal/ingest"
)

// maxBodyBytes limita el cuerpo de POST /events. Sobra para cualquier partida simulada.
const maxBodyBytes = 10 << 20

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

func newMux(d *ingest.Dispatcher) *http.ServeMux {
	mux := http.NewServeMux()

	// /health existe para el CI y para los healthchecks de docker compose.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "ok",
			"service": "ingest-go",
		})
	})

	mux.HandleFunc("POST /events", func(w http.ResponseWriter, r *http.Request) {
		cuerpo, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
		if err != nil {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": err.Error()})
			return
		}

		respuesta := eventsResponse{Rejected: []contract.Rejection{}}
		for _, raw := range splitEvents(cuerpo) {
			if rej := d.Ingest(raw); rej != nil {
				respuesta.Rejected = append(respuesta.Rejected, *rej)
			} else {
				respuesta.Accepted++
			}
		}

		// contracts/rejections.md: 200 si se acepto al menos uno; 400 si no entro ninguno.
		status := http.StatusOK
		if respuesta.Accepted == 0 && len(respuesta.Rejected) > 0 {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, respuesta)
	})

	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, d.Metrics())
	})

	return mux
}

// eventsResponse es la respuesta de POST /events (contracts/rejections.md).
type eventsResponse struct {
	Accepted int                  `json:"accepted"`
	Rejected []contract.Rejection `json:"rejected"`
}

// splitEvents acepta un evento suelto (lo que manda el simulador, uno por peticion) o un arreglo
// de eventos. Si el cuerpo no es un arreglo JSON valido se devuelve entero como un solo evento, y
// la validacion lo rechaza con malformed_json.
func splitEvents(cuerpo []byte) []json.RawMessage {
	if bytes.HasPrefix(bytes.TrimSpace(cuerpo), []byte("[")) {
		var varios []json.RawMessage
		if err := json.Unmarshal(cuerpo, &varios); err == nil {
			return varios
		}
	}
	return []json.RawMessage{cuerpo}
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
		Handler:           newMux(ingest.NewDispatcher(ingest.HTTPSender(cfg.analyticsURL))),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("ingest-go escuchando en :%s (analytics en %s)", cfg.port, cfg.analyticsURL)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("el servidor se detuvo: %v", err)
	}
}
