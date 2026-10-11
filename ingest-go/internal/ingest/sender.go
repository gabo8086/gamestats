package ingest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gamestats/ingest/internal/contract"
)

// retryDelay es la espera antes del unico reintento. Variable para que las pruebas no esperen.
var retryDelay = 500 * time.Millisecond

// HTTPSender manda cada lote a POST {baseURL}/analyze. Si falla por red o por un 5xx, reintenta
// una vez: son errores pasajeros (Scala reiniciando, por ejemplo). Un 4xx no se reintenta, porque
// Scala ya dijo que el lote es invalido y mandarlo igual daria la misma respuesta.
func HTTPSender(baseURL string) Sender {
	cliente := &http.Client{Timeout: 10 * time.Second}
	url := strings.TrimRight(baseURL, "/") + "/analyze"

	return func(lote contract.Batch) error {
		cuerpo, err := json.Marshal(lote)
		if err != nil {
			return err
		}

		reintentable, err := post(cliente, url, cuerpo)
		if err != nil && reintentable {
			time.Sleep(retryDelay)
			_, err = post(cliente, url, cuerpo)
		}
		return err
	}
}

func post(cliente *http.Client, url string, cuerpo []byte) (reintentable bool, err error) {
	resp, err := cliente.Post(url, "application/json", bytes.NewReader(cuerpo))
	if err != nil {
		return true, err
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 == 2 {
		return false, nil
	}
	detalle, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return resp.StatusCode >= 500, fmt.Errorf("analisis respondio %d: %s", resp.StatusCode, detalle)
}
