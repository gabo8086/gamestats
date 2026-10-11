package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// examplePath resuelve una ruta dentro de contracts/examples/ desde este paquete.
func examplePath(name string) string {
	return filepath.Join("..", "..", "..", "contracts", "examples", name)
}

func readBatch(t *testing.T, name string) Batch {
	t.Helper()

	raw, err := os.ReadFile(examplePath(name))
	if err != nil {
		t.Fatalf("no se pudo leer %s: %v", name, err)
	}

	var batch Batch
	if err := json.Unmarshal(raw, &batch); err != nil {
		t.Fatalf("no se pudo decodificar %s: %v", name, err)
	}
	return batch
}

// Los ejemplos del contrato tienen que decodificar con las structs de Go sin perder nada.
func TestEjemplosDecodificanEnElSobre(t *testing.T) {
	casos := []struct {
		archivo string
		eventos int
		gameID  string
	}{
		{"racing-match-ok.json", 15, "racing"},
		{"combat-match-ok.json", 12, "combat"},
		{"combat-vs-bot-match-ok.json", 14, "combat"},
		{"blackjack-match-ok.json", 47, "blackjack"},
		{"battleship-match-ok.json", 18, "battleship"},
	}

	for _, c := range casos {
		t.Run(c.archivo, func(t *testing.T) {
			batch := readBatch(t, c.archivo)

			if got := len(batch.Events); got != c.eventos {
				t.Errorf("eventos = %d, se esperaban %d", got, c.eventos)
			}
			if batch.GameID != c.gameID {
				t.Errorf("gameId = %q, se esperaba %q", batch.GameID, c.gameID)
			}

			for _, ev := range batch.Events {
				if ev.EventID == "" || ev.MatchID == "" || ev.Type == "" {
					t.Errorf("evento con campos obligatorios vacios: %+v", ev)
				}
				if !ev.IsMatchEvent() && (ev.PlayerID == nil || *ev.PlayerID == "") {
					t.Errorf("%s (%s) deberia traer playerId", ev.EventID, ev.Type)
				}
			}

			if primero := batch.Events[0].Type; primero != TypeMatchStarted {
				t.Errorf("primer evento = %q, se esperaba %q", primero, TypeMatchStarted)
			}
			if ultimo := batch.Events[len(batch.Events)-1].Type; ultimo != TypeMatchFinished {
				t.Errorf("ultimo evento = %q, se esperaba %q", ultimo, TypeMatchFinished)
			}
		})
	}
}

// Go no interpreta data: la guarda como JSON crudo y la reenvia tal cual.
func TestDataQuedaComoJSONCrudo(t *testing.T) {
	batch := readBatch(t, "racing-match-ok.json")

	var conData *Event
	for i, ev := range batch.Events {
		if ev.Type == "LAP_COMPLETED" {
			conData = &batch.Events[i]
			break
		}
	}
	if conData == nil {
		t.Fatal("el ejemplo de carreras no tiene ningun LAP_COMPLETED")
	}

	var campos map[string]any
	if err := json.Unmarshal(conData.Data, &campos); err != nil {
		t.Fatalf("data no es un objeto JSON valido: %v", err)
	}
	for _, campo := range []string{"lap", "lapTimeMs", "position"} {
		if _, ok := campos[campo]; !ok {
			t.Errorf("data de LAP_COMPLETED sin el campo %q", campo)
		}
	}
}

// El sobre es cerrado: un campo que no esta en el contrato se rechaza (razon unknown_field).
func TestDecodeEventRechazaCamposDesconocidos(t *testing.T) {
	raw, err := os.ReadFile(examplePath("invalid-events.json"))
	if err != nil {
		t.Fatalf("no se pudo leer invalid-events.json: %v", err)
	}

	var archivo struct {
		Cases []struct {
			ExpectedReason string          `json:"expectedReason"`
			Event          json.RawMessage `json:"event"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &archivo); err != nil {
		t.Fatalf("no se pudo decodificar invalid-events.json: %v", err)
	}

	var probados int
	for _, caso := range archivo.Cases {
		if caso.ExpectedReason != ReasonUnknownField {
			continue
		}
		probados++
		if _, err := DecodeEvent(caso.Event); err == nil {
			t.Errorf("DecodeEvent acepto un evento con un campo fuera del contrato")
		}
	}
	if probados == 0 {
		t.Fatal("invalid-events.json no tiene ningun caso unknown_field")
	}
}

// Un evento del contrato se decodifica sin error con el sobre cerrado.
func TestDecodeEventAceptaUnEventoDelContrato(t *testing.T) {
	batch := readBatch(t, "combat-match-ok.json")

	raw, err := json.Marshal(batch.Events[0])
	if err != nil {
		t.Fatalf("no se pudo serializar el evento: %v", err)
	}
	if _, err := DecodeEvent(raw); err != nil {
		t.Errorf("DecodeEvent rechazo un evento valido: %v", err)
	}
}

// Cada caso de invalid-events.json que el esquema puede detectar (validAgainstSchema: false) tiene
// que salir de Parse con exactamente la razon esperada. Los demas necesitan el estado de la
// partida y se prueban en el paquete ingest.
func TestParseDaLaRazonDeCadaCasoInvalido(t *testing.T) {
	raw, err := os.ReadFile(examplePath("invalid-events.json"))
	if err != nil {
		t.Fatalf("no se pudo leer invalid-events.json: %v", err)
	}

	var archivo struct {
		Cases []struct {
			ExpectedReason     string          `json:"expectedReason"`
			Note               string          `json:"note"`
			ValidAgainstSchema bool            `json:"validAgainstSchema"`
			Event              json.RawMessage `json:"event"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &archivo); err != nil {
		t.Fatalf("no se pudo decodificar invalid-events.json: %v", err)
	}

	for _, caso := range archivo.Cases {
		t.Run(caso.Note, func(t *testing.T) {
			_, rej := Parse(caso.Event)
			if caso.ValidAgainstSchema {
				if rej != nil {
					t.Fatalf("el sobre es valido y Parse lo rechazo con %q", rej.Reason)
				}
				return
			}
			if rej == nil {
				t.Fatalf("Parse acepto el evento; se esperaba %q", caso.ExpectedReason)
			}
			if rej.Reason != caso.ExpectedReason {
				t.Errorf("razon = %q, se esperaba %q", rej.Reason, caso.ExpectedReason)
			}
			if rej.EventID == "" {
				t.Error("el rechazo no dice de que evento se trata")
			}
		})
	}
}

func TestParseRechazaJSONMalFormado(t *testing.T) {
	for _, cuerpo := range []string{`{"eventId": "e-1",`, `no es json`, `[1, 2]`, `42`} {
		_, rej := Parse([]byte(cuerpo))
		if rej == nil || rej.Reason != ReasonMalformedJSON {
			t.Errorf("Parse(%q) = %+v, se esperaba %q", cuerpo, rej, ReasonMalformedJSON)
		}
	}
}

func TestParseDiceQueCampoSobra(t *testing.T) {
	_, rej := Parse([]byte(`{"eventId":"e-1","extra":1}`))
	if rej == nil || rej.Reason != ReasonUnknownField || rej.Detail != "extra" {
		t.Errorf("rechazo = %+v, se esperaba unknown_field con detail \"extra\"", rej)
	}
}

// playerId puede faltar en los MATCH_*, pero no puede ser cadena vacia en los demas.
func TestParsePlayerID(t *testing.T) {
	base := `{"eventId":"e-1","timestamp":"2026-10-05T14:00:00Z","gameId":"racing",` +
		`"gameVersion":"1.0","matchId":"m-1",`

	if _, rej := Parse([]byte(base + `"type":"MATCH_FINISHED"}`)); rej != nil {
		t.Errorf("MATCH_FINISHED sin playerId rechazado: %+v", rej)
	}
	_, rej := Parse([]byte(base + `"type":"PENALTY","playerId":""}`))
	if rej == nil || rej.Reason != ReasonMissingPlayerID {
		t.Errorf("rechazo = %+v, se esperaba %q", rej, ReasonMissingPlayerID)
	}
}
