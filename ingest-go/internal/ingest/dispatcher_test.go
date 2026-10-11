package ingest

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"gamestats/ingest/internal/contract"
)

// Igual que en el paquete contract: las pruebas usan los ejemplos del contrato, no datos propios.
func examplePath(name string) string {
	return filepath.Join("..", "..", "..", "contracts", "examples", name)
}

var ejemplosOK = []string{
	"racing-match-ok.json",
	"combat-match-ok.json",
	"combat-vs-bot-match-ok.json",
	"blackjack-match-ok.json",
	"battleship-match-ok.json",
}

// loteCrudo es un lote del contrato con cada evento como JSON crudo, tal como lo mandaria el
// simulador a POST /events.
type loteCrudo struct {
	MatchID string            `json:"matchId"`
	Events  []json.RawMessage `json:"events"`
}

func readExample(t *testing.T, name string) loteCrudo {
	t.Helper()
	raw, err := os.ReadFile(examplePath(name))
	if err != nil {
		t.Fatalf("no se pudo leer %s: %v", name, err)
	}
	var lote loteCrudo
	if err := json.Unmarshal(raw, &lote); err != nil {
		t.Fatalf("no se pudo decodificar %s: %v", name, err)
	}
	return lote
}

// recorder es un Sender que guarda los lotes en un channel para que la prueba los espere.
func recorder() (Sender, chan contract.Batch) {
	lotes := make(chan contract.Batch, 16)
	return func(b contract.Batch) error {
		lotes <- b
		return nil
	}, lotes
}

func waitBatch(t *testing.T, lotes chan contract.Batch) contract.Batch {
	t.Helper()
	select {
	case b := <-lotes:
		return b
	case <-time.After(2 * time.Second):
		t.Fatal("la partida no llego a analisis")
		return contract.Batch{}
	}
}

// eventually espera hasta un segundo a que las metricas cumplan la condicion. Hace falta porque
// deliver cuenta el lote justo despues de que el Sender devuelve, no antes.
func eventually(d *Dispatcher, cond func(Snapshot) bool) Snapshot {
	limite := time.Now().Add(time.Second)
	for !cond(d.Metrics()) && time.Now().Before(limite) {
		time.Sleep(5 * time.Millisecond)
	}
	return d.Metrics()
}

func ingestAll(t *testing.T, d *Dispatcher, eventos []json.RawMessage) {
	t.Helper()
	for _, raw := range eventos {
		if rej := d.Ingest(raw); rej != nil {
			t.Fatalf("evento valido rechazado: %+v", rej)
		}
	}
}

// mismoJSON compara dos valores por su forma JSON, que es lo que viaja hacia Scala.
func mismoJSON(t *testing.T, a, b any) bool {
	t.Helper()
	normalizar := func(v any) any {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		var out any
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	return reflect.DeepEqual(normalizar(a), normalizar(b))
}

// Cada partida del contrato, enviada evento por evento, tiene que llegar a Scala identica al lote
// del ejemplo: mismos eventos, mismo orden, data sin tocar.
func TestEjemplosLleganEnterosAAnalisis(t *testing.T) {
	for _, nombre := range ejemplosOK {
		t.Run(nombre, func(t *testing.T) {
			send, lotes := recorder()
			d := NewDispatcher(send)
			esperado := readExample(t, nombre)

			ingestAll(t, d, esperado.Events)
			lote := waitBatch(t, lotes)

			var original any
			raw, _ := os.ReadFile(examplePath(nombre))
			_ = json.Unmarshal(raw, &original)
			if !mismoJSON(t, lote, original) {
				t.Errorf("el lote enviado no coincide con %s", nombre)
			}
		})
	}
}

// Los eventos que llegan desordenados salen ordenados por timestamp. Se invierte todo lo que hay
// entre MATCH_STARTED y MATCH_FINISHED: el peor desorden que no provoca rechazos.
func TestOrdenaPorTimestampAntesDeEnviar(t *testing.T) {
	send, lotes := recorder()
	d := NewDispatcher(send)
	ejemplo := readExample(t, "racing-match-ok.json")

	desordenados := slices.Clone(ejemplo.Events)
	slices.Reverse(desordenados[1 : len(desordenados)-1])
	ingestAll(t, d, desordenados)

	lote := waitBatch(t, lotes)
	for i := 1; i < len(lote.Events); i++ {
		if lote.Events[i].Time().Before(lote.Events[i-1].Time()) {
			t.Fatalf("evento %d (%s) antes que el %d", i, lote.Events[i].EventID, i-1)
		}
	}
}

// Los casos de invalid-events.json que solo se detectan con el estado de la partida. Cada uno se
// prueba con un despachador nuevo, despues de procesar sus precedingEvents.
func TestRechazosQueDependenDelEstado(t *testing.T) {
	raw, err := os.ReadFile(examplePath("invalid-events.json"))
	if err != nil {
		t.Fatal(err)
	}
	var archivo struct {
		Cases []struct {
			ExpectedReason     string            `json:"expectedReason"`
			Note               string            `json:"note"`
			ValidAgainstSchema bool              `json:"validAgainstSchema"`
			PrecedingEvents    []json.RawMessage `json:"precedingEvents"`
			Event              json.RawMessage   `json:"event"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &archivo); err != nil {
		t.Fatal(err)
	}

	probados := 0
	for _, caso := range archivo.Cases {
		if !caso.ValidAgainstSchema {
			continue
		}
		probados++
		t.Run(caso.Note, func(t *testing.T) {
			send, _ := recorder()
			d := NewDispatcher(send)
			ingestAll(t, d, caso.PrecedingEvents)

			rej := d.Ingest(caso.Event)
			if rej == nil {
				t.Fatalf("evento aceptado; se esperaba %q", caso.ExpectedReason)
			}
			if rej.Reason != caso.ExpectedReason {
				t.Errorf("razon = %q, se esperaba %q", rej.Reason, caso.ExpectedReason)
			}
		})
	}
	if probados == 0 {
		t.Fatal("invalid-events.json no tiene casos que dependan del estado")
	}
}

// Un evento rechazado no cuenta como "ya aceptado": si se corrige y se reenvia con el mismo
// eventId, tiene que entrar.
func TestUnRechazoNoReservaElEventID(t *testing.T) {
	send, _ := recorder()
	d := NewDispatcher(send)
	inicio := `{"eventId":"e-1","timestamp":"2026-10-05T14:00:00Z","gameId":"combat","gameVersion":"1.0",` +
		`"matchId":"m-1","type":"MATCH_STARTED"}`
	reconexion := `{"eventId":"e-2","timestamp":"2026-10-05T14:00:05Z","gameId":"combat","gameVersion":"1.0",` +
		`"matchId":"m-1","playerId":"p1","type":"PLAYER_RECONNECTED"}`
	desconexion := `{"eventId":"e-3","timestamp":"2026-10-05T14:00:01Z","gameId":"combat","gameVersion":"1.0",` +
		`"matchId":"m-1","playerId":"p1","type":"PLAYER_DISCONNECTED"}`

	ingestAll(t, d, []json.RawMessage{json.RawMessage(inicio)})
	if rej := d.Ingest([]byte(reconexion)); rej == nil || rej.Reason != contract.ReasonReconnectWithoutDisconnect {
		t.Fatalf("rechazo = %+v, se esperaba reconnect_without_disconnect", rej)
	}
	ingestAll(t, d, []json.RawMessage{json.RawMessage(desconexion), json.RawMessage(reconexion)})
}

// Las cinco partidas del contrato a la vez, cada una desde su propia goroutine, como llegarian
// del simulador. Con `go test -race` esta prueba es la que vigila los accesos al estado compartido.
func TestPartidasEnParalelo(t *testing.T) {
	send, lotes := recorder()
	d := NewDispatcher(send)

	total := 0
	var wg sync.WaitGroup
	for _, nombre := range ejemplosOK {
		ejemplo := readExample(t, nombre)
		total += len(ejemplo.Events)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, raw := range ejemplo.Events {
				if rej := d.Ingest(raw); rej != nil {
					t.Errorf("%s: evento valido rechazado: %+v", nombre, rej)
				}
			}
		}()
	}
	wg.Wait()

	recibidas := map[string]bool{}
	for range ejemplosOK {
		recibidas[waitBatch(t, lotes).MatchID] = true
	}
	if len(recibidas) != len(ejemplosOK) {
		t.Errorf("llegaron %d partidas distintas, se esperaban %d", len(recibidas), len(ejemplosOK))
	}

	m := eventually(d, func(m Snapshot) bool { return m.BatchesSent == len(ejemplosOK) })
	if m.EventsReceived != total || m.EventsValid != total || m.EventsInvalid != 0 {
		t.Errorf("metricas = %+v, se esperaban %d eventos validos", m, total)
	}
	if m.ActiveMatches != 0 || m.BatchesSent != len(ejemplosOK) {
		t.Errorf("activas = %d, enviadas = %d", m.ActiveMatches, m.BatchesSent)
	}
}

func TestMetricasCuentanPorRazon(t *testing.T) {
	send, _ := recorder()
	d := NewDispatcher(send)

	inicio := `{"eventId":"e-1","timestamp":"2026-10-05T14:00:00Z","gameId":"racing","gameVersion":"1.0",` +
		`"matchId":"m-1","type":"MATCH_STARTED"}`
	d.Ingest([]byte(inicio))
	d.Ingest([]byte(inicio)) // duplicado
	d.Ingest([]byte(`no es json`))
	d.Ingest([]byte(`{"eventId":"e-2","extra":true}`))

	m := d.Metrics()
	if m.EventsReceived != 4 || m.EventsValid != 1 || m.EventsInvalid != 3 {
		t.Errorf("metricas = %+v", m)
	}
	if m.ActiveMatches != 1 {
		t.Errorf("activas = %d, se esperaba 1", m.ActiveMatches)
	}
	for razon, esperado := range map[string]int{
		contract.ReasonDuplicateEvent:  1,
		contract.ReasonMalformedJSON:   1,
		contract.ReasonUnknownField:    1,
		contract.ReasonMatchNotStarted: 0,
	} {
		if got, ok := m.RejectionsByReason[razon]; !ok || got != esperado {
			t.Errorf("%s = %d (presente: %v), se esperaba %d", razon, got, ok, esperado)
		}
	}
	if len(m.RejectionsByReason) != len(contract.AllReasons) {
		t.Errorf("GET /metrics deberia listar las %d razones", len(contract.AllReasons))
	}
}

// Si analisis no acepta el lote, la partida no se pierde: queda contada y guardada.
func TestLoteFallidoQuedaRegistrado(t *testing.T) {
	enviado := make(chan struct{})
	d := NewDispatcher(func(contract.Batch) error {
		defer close(enviado)
		return errors.New("analisis caido")
	})
	ingestAll(t, d, readExample(t, "combat-match-ok.json").Events)
	<-enviado

	if m := eventually(d, func(m Snapshot) bool { return m.BatchesFailed > 0 }); m.BatchesFailed != 1 || m.BatchesSent != 0 {
		t.Errorf("metricas = %+v, se esperaba un lote fallido", m)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.failed) != 1 || d.failed[0].MatchID != "m-combat-001" {
		t.Errorf("lotes guardados = %d", len(d.failed))
	}
}
