package ingest

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"gamestats/ingest/internal/contract"
)

func init() { retryDelay = 0 }

// servidor responde con los codigos dados, uno por peticion, y cuenta cuantas recibio.
func servidor(t *testing.T, codigos ...int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var llamadas atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(llamadas.Add(1)) - 1
		if r.URL.Path != "/analyze" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("peticion inesperada: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(codigos[min(n, len(codigos)-1)])
	}))
	t.Cleanup(srv.Close)
	return srv, &llamadas
}

var loteDePrueba = contract.Batch{MatchID: "m-1", GameID: "racing", GameVersion: "1.0"}

func TestSenderReintentaUnaVezAnteUn5xx(t *testing.T) {
	srv, llamadas := servidor(t, http.StatusServiceUnavailable, http.StatusOK)
	if err := HTTPSender(srv.URL + "/")(loteDePrueba); err != nil {
		t.Errorf("error = %v, el reintento deberia haber funcionado", err)
	}
	if llamadas.Load() != 2 {
		t.Errorf("llamadas = %d, se esperaban 2", llamadas.Load())
	}
}

func TestSenderSeRindeTrasElReintento(t *testing.T) {
	srv, llamadas := servidor(t, http.StatusInternalServerError)
	if err := HTTPSender(srv.URL)(loteDePrueba); err == nil {
		t.Error("se esperaba un error")
	}
	if llamadas.Load() != 2 {
		t.Errorf("llamadas = %d, se esperaban 2 (un intento y un reintento)", llamadas.Load())
	}
}

// Un 400 es Scala diciendo que el lote es invalido: reintentarlo daria lo mismo.
func TestSenderNoReintentaUn4xx(t *testing.T) {
	srv, llamadas := servidor(t, http.StatusBadRequest)
	if err := HTTPSender(srv.URL)(loteDePrueba); err == nil {
		t.Error("se esperaba un error")
	}
	if llamadas.Load() != 1 {
		t.Errorf("llamadas = %d, se esperaba 1", llamadas.Load())
	}
}

func TestSenderSinServidor(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	if err := HTTPSender(url)(loteDePrueba); err == nil {
		t.Error("se esperaba un error de conexion")
	}
}
