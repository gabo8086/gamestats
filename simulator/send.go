package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

var cliente = &http.Client{Timeout: 10 * time.Second}

// enviar hace un POST con el cuerpo en JSON. Devuelve 0 como estado cuando no se pudo ni conectar,
// que es un caso distinto de "el servicio contesto mal" y conviene no confundirlos.
func enviar(url string, cuerpo any) (int, string) {
	datos, err := json.Marshal(cuerpo)
	if err != nil {
		return 0, err.Error()
	}

	resp, err := cliente.Post(url, "application/json", bytes.NewReader(datos))
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()

	respuesta, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(respuesta)
}

// mandarLotes manda cada lote a POST /analyze del modulo de analisis, saltandose la ingestion.
//
// Sirve para probar el analisis sin depender de que la ingestion este lista, y para la demo cuando
// se quiere mostrar solo la parte funcional.
func mandarLotes(base string, lotes map[string]Batch) int {
	ids := make([]string, 0, len(lotes))
	for id := range lotes {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	fallos := 0
	for _, id := range ids {
		estado, cuerpo := enviar(strings.TrimRight(base, "/")+"/analyze", lotes[id])
		marca := "ok "
		if estado != http.StatusOK {
			marca = "FALLO"
			fallos++
		}
		fmt.Printf("  [%s] %-28s HTTP %d\n", marca, id, estado)
		if estado != http.StatusOK {
			fmt.Printf("         %s\n", recortar(cuerpo, 200))
		}
	}
	return fallos
}

// mandarEventos manda los eventos uno a uno a POST /events de la ingestion, con retardo entre cada
// uno. El retardo es para la demo en vivo: con los matchId intercalados se ve que la ingestion
// atiende varias partidas a la vez.
//
// Solo se imprimen los primeros fallos: si el endpoint entero esta mal, repetir el mismo error cien
// veces no agrega informacion. Si no hay nadie escuchando se corta, porque ahi no tiene sentido
// seguir.
func mandarEventos(base string, eventos []Event, retardo time.Duration) int {
	url := strings.TrimRight(base, "/") + "/events"
	fallos := 0

	for i, evento := range eventos {
		estado, cuerpo := enviar(url, evento)
		switch {
		case estado == http.StatusOK || estado == http.StatusCreated || estado == http.StatusAccepted:
			if (i+1)%20 == 0 {
				fmt.Printf("  enviados %d/%d\n", i+1, len(eventos))
			}
		default:
			fallos++
			if fallos <= 3 {
				fmt.Printf("  [FALLO] %s %-18s HTTP %d %s\n",
					evento.EventID, evento.Type, estado, recortar(cuerpo, 120))
			}
			if estado == 0 {
				fmt.Println("  se corta el envio: el servicio no responde")
				return fallos
			}
		}
		if retardo > 0 {
			time.Sleep(retardo)
		}
	}
	return fallos
}

func recortar(texto string, largo int) string {
	texto = strings.TrimSpace(texto)
	if len(texto) <= largo {
		return texto
	}
	return texto[:largo]
}
