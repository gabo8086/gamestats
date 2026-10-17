package main

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

// Lo que se comprueba no es "el programa corre" sino las propiedades de las que depende la demo:
// que el flujo sea reproducible, que los datos sean coherentes y que las reglas guionadas se
// disparen de verdad. Que una regla ocurra por casualidad no sirve para mostrarla.

const (
	partidasDePrueba = 8
	desordenDePrueba = 8
)

func flujoDePrueba(t *testing.T) Flujo {
	t.Helper()
	return Generar(7, partidasDePrueba, desordenDePrueba)
}

func instante(t *testing.T, marca string) time.Time {
	t.Helper()
	momento, err := time.Parse("2006-01-02T15:04:05.000Z", marca)
	if err != nil {
		t.Fatalf("timestamp ilegible %q: %v", marca, err)
	}
	return momento
}

// --------------------------------------------------------------------- generacion

func TestLaMismaSemillaDaElMismoFlujo(t *testing.T) {
	// Sin esto no se puede repetir una demo ni fijar un resultado esperado. Importa especialmente
	// porque las partidas se generan en goroutines: el orden en que terminan no debe influir.
	uno := Generar(7, partidasDePrueba, desordenDePrueba)
	otro := Generar(7, partidasDePrueba, desordenDePrueba)

	if len(uno.Eventos) != len(otro.Eventos) {
		t.Fatalf("cantidades distintas: %d y %d", len(uno.Eventos), len(otro.Eventos))
	}
	for i := range uno.Eventos {
		if uno.Eventos[i].EventID != otro.Eventos[i].EventID ||
			uno.Eventos[i].Timestamp != otro.Eventos[i].Timestamp ||
			uno.Eventos[i].Type != otro.Eventos[i].Type {
			t.Fatalf("difieren en la posicion %d: %+v vs %+v", i, uno.Eventos[i], otro.Eventos[i])
		}
	}
}

func TestSemillasDistintasDanFlujosDistintos(t *testing.T) {
	uno := Generar(7, partidasDePrueba, desordenDePrueba)
	otro := Generar(99, partidasDePrueba, desordenDePrueba)

	iguales := len(uno.Eventos) == len(otro.Eventos)
	if iguales {
		for i := range uno.Eventos {
			if uno.Eventos[i].Timestamp != otro.Eventos[i].Timestamp {
				iguales = false
				break
			}
		}
	}
	if iguales {
		t.Fatal("dos semillas distintas produjeron el mismo flujo")
	}
}

func TestLosEventIdNoSeRepiten(t *testing.T) {
	vistos := map[string]bool{}
	for _, e := range flujoDePrueba(t).Eventos {
		if vistos[e.EventID] {
			t.Fatalf("eventId repetido: %s", e.EventID)
		}
		vistos[e.EventID] = true
	}
}

func TestHayPartidasDeLosCuatroJuegos(t *testing.T) {
	flujo := flujoDePrueba(t)
	for _, juego := range []string{"racing", "combat", "blackjack", "battleship"} {
		if flujo.Resumen.ByGame[juego] == 0 {
			t.Errorf("no hay ninguna partida de %s", juego)
		}
	}
}

func TestLasPartidasSeSolapanEnElTiempo(t *testing.T) {
	// El enunciado §8 pide partidas en paralelo: es lo que le da trabajo concurrente a la
	// ingestion. Se comprueba buscando dos partidas cuyas ventanas se crucen.
	flujo := flujoDePrueba(t)

	type ventana struct{ desde, hasta time.Time }
	ventanas := map[string]ventana{}
	for id, lote := range flujo.Lotes {
		ventanas[id] = ventana{
			desde: instante(t, lote.Events[0].Timestamp),
			hasta: instante(t, lote.Events[len(lote.Events)-1].Timestamp),
		}
	}

	for a, va := range ventanas {
		for b, vb := range ventanas {
			if a < b && va.desde.Before(vb.hasta) && vb.desde.Before(va.hasta) {
				return
			}
		}
	}
	t.Fatal("ninguna partida se solapa con otra")
}

func TestElFlujoLlegaDesordenado(t *testing.T) {
	// La ingestion tiene que ordenar por timestamp; si el simulador entrega todo ordenado, esa
	// parte nunca se ejercita.
	eventos := flujoDePrueba(t).Eventos
	for i := 1; i < len(eventos); i++ {
		if eventos[i].Timestamp < eventos[i-1].Timestamp {
			return
		}
	}
	t.Fatal("el flujo salio ordenado")
}

func TestElDesordenNoMueveLosEventosDePartida(t *testing.T) {
	// Si un MATCH_STARTED quedara detras de otro evento de su partida, la ingestion lo rechazaria
	// con match_not_started y seria un rechazo inventado por el simulador, no un caso real.
	flujo := flujoDePrueba(t)

	for id := range flujo.Lotes {
		var delFlujo []Event
		for _, e := range flujo.Eventos {
			if e.MatchID == id {
				delFlujo = append(delFlujo, e)
			}
		}
		if delFlujo[0].Type != MatchStarted {
			t.Errorf("%s: el primer evento del flujo es %s", id, delFlujo[0].Type)
		}
		if delFlujo[len(delFlujo)-1].Type != MatchFinished {
			t.Errorf("%s: el ultimo evento del flujo es %s", id, delFlujo[len(delFlujo)-1].Type)
		}
	}
}

func TestLosLotesVanOrdenados(t *testing.T) {
	// El contrato dice que el lote llega ordenado por timestamp y que el analisis no reordena.
	for id, lote := range flujoDePrueba(t).Lotes {
		for i := 1; i < len(lote.Events); i++ {
			if lote.Events[i].Timestamp < lote.Events[i-1].Timestamp {
				t.Fatalf("%s: lote desordenado en la posicion %d", id, i)
			}
		}
	}
}

func TestElSobreCumpleElContrato(t *testing.T) {
	rfc3339 := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`)

	for _, e := range flujoDePrueba(t).Eventos {
		if e.EventID == "" || e.GameID == "" || e.GameVersion == "" || e.MatchID == "" || e.Type == "" {
			t.Fatalf("campo obligatorio vacio en %+v", e)
		}
		if !rfc3339.MatchString(e.Timestamp) {
			t.Fatalf("timestamp fuera de RFC 3339: %q", e.Timestamp)
		}
		// playerId es obligatorio salvo en los MATCH_*.
		if strings.HasPrefix(e.Type, "MATCH_") {
			if e.PlayerID != nil {
				t.Fatalf("%s trae playerId: %s", e.Type, *e.PlayerID)
			}
		} else if e.PlayerID == nil || *e.PlayerID == "" {
			t.Fatalf("%s sin playerId (%s)", e.Type, e.EventID)
		}
	}
}

// --------------------------------------------------------------------- carreras

func TestLaPosicionSaleDelTiempoAcumulado(t *testing.T) {
	// Se comprueba en **todas** las carreras generadas, no solo en las guionadas.
	for id, lote := range flujoDePrueba(t).Lotes {
		if lote.GameID != "racing" {
			continue
		}

		acumulado := map[string]int{}
		porVuelta := map[int]map[string]int{}
		posiciones := map[int]map[string]int{}

		for _, e := range lote.Events {
			if e.Type != LapCompleted {
				continue
			}
			data := e.Data.(map[string]any)
			vuelta := data["lap"].(int)
			acumulado[*e.PlayerID] += data["lapTimeMs"].(int)

			if porVuelta[vuelta] == nil {
				porVuelta[vuelta] = map[string]int{}
				posiciones[vuelta] = map[string]int{}
			}
			porVuelta[vuelta][*e.PlayerID] = acumulado[*e.PlayerID]
			posiciones[vuelta][*e.PlayerID] = data["position"].(int)
		}

		for vuelta, tiempos := range porVuelta {
			orden := make([]string, 0, len(tiempos))
			for j := range tiempos {
				orden = append(orden, j)
			}
			sort.Slice(orden, func(i, j int) bool { return tiempos[orden[i]] < tiempos[orden[j]] })

			for puesto, j := range orden {
				if posiciones[vuelta][j] != puesto+1 {
					t.Errorf("%s vuelta %d: %s tiene posicion %d pero por acumulado le toca %d",
						id, vuelta, j, posiciones[vuelta][j], puesto+1)
				}
			}
		}
	}
}

func TestLaRemontadaGuionadaCumpleSuDefinicion(t *testing.T) {
	lote := flujoDePrueba(t).Lotes["m-sim-racing-comeback"]

	jugadores := map[string]bool{}
	ajustado := map[string]int{}
	posiciones := map[string][]int{}
	for _, e := range lote.Events {
		switch e.Type {
		case PlayerJoined:
			jugadores[*e.PlayerID] = true
		case LapCompleted:
			data := e.Data.(map[string]any)
			ajustado[*e.PlayerID] += data["lapTimeMs"].(int)
			posiciones[*e.PlayerID] = append(posiciones[*e.PlayerID], data["position"].(int))
		case Penalty:
			ajustado[*e.PlayerID] += e.Data.(map[string]any)["seconds"].(int) * 1000
		}
	}

	ganador := ""
	for j, tiempo := range ajustado {
		if ganador == "" || tiempo < ajustado[ganador] {
			ganador = j
		}
	}
	if ganador != "p3" {
		t.Fatalf("deberia ganar p3 por tiempo ajustado, gano %s", ganador)
	}

	ultimo := len(jugadores)
	for _, p := range posiciones[ganador] {
		if p == ultimo {
			return
		}
	}
	t.Fatalf("el ganador nunca estuvo en la ultima posicion: %v", posiciones[ganador])
}

func TestLaPenalizacionGuionadaCambiaAlGanador(t *testing.T) {
	lote := flujoDePrueba(t).Lotes["m-sim-racing-penalty"]

	soloVueltas := map[string]int{}
	penalizado := map[string]int{}
	for _, e := range lote.Events {
		switch e.Type {
		case LapCompleted:
			soloVueltas[*e.PlayerID] += e.Data.(map[string]any)["lapTimeMs"].(int)
		case Penalty:
			penalizado[*e.PlayerID] = e.Data.(map[string]any)["seconds"].(int) * 1000
		}
	}

	masRapido, ajustadoMenor := "", ""
	for j, tiempo := range soloVueltas {
		if masRapido == "" || tiempo < soloVueltas[masRapido] {
			masRapido = j
		}
		if ajustadoMenor == "" || tiempo+penalizado[j] < soloVueltas[ajustadoMenor]+penalizado[ajustadoMenor] {
			ajustadoMenor = j
		}
	}

	if masRapido != "p1" {
		t.Errorf("el mas rapido deberia ser p1, es %s", masRapido)
	}
	if ajustadoMenor != "p2" {
		t.Errorf("la penalizacion deberia dar vuelta el resultado a favor de p2, gana %s", ajustadoMenor)
	}
}

// --------------------------------------------------------------------- combate

func TestLaRachaGuionadaCumpleSuDefinicion(t *testing.T) {
	lote := flujoDePrueba(t).Lotes["m-sim-combat-streak"]

	var suyas []Event
	var muertes []time.Time
	for _, e := range lote.Events {
		if e.Type != PlayerEliminated {
			continue
		}
		if *e.PlayerID == "p3" {
			suyas = append(suyas, e)
		}
		if e.Data.(map[string]any)["victimId"].(string) == "p3" {
			muertes = append(muertes, instante(t, e.Timestamp))
		}
	}

	if len(suyas) < 3 {
		t.Fatalf("p3 deberia tener al menos 3 eliminaciones, tiene %d", len(suyas))
	}

	desde, hasta := instante(t, suyas[0].Timestamp), instante(t, suyas[2].Timestamp)
	if ventana := hasta.Sub(desde); ventana > 10*time.Second {
		t.Fatalf("la ventana de la racha es %v, mas de 10 s", ventana)
	}
	for _, m := range muertes {
		if !m.Before(desde) && !m.After(hasta) {
			t.Fatal("p3 muere dentro de su propia racha")
		}
	}
}

func TestLaVenganzaGuionadaExiste(t *testing.T) {
	lote := flujoDePrueba(t).Lotes["m-sim-combat-streak"]

	type par struct {
		autor, victima string
		cuando         time.Time
	}
	var bajas []par
	for _, e := range lote.Events {
		if e.Type == PlayerEliminated {
			bajas = append(bajas, par{*e.PlayerID, e.Data.(map[string]any)["victimId"].(string), instante(t, e.Timestamp)})
		}
	}

	for _, primera := range bajas {
		for _, despues := range bajas {
			if despues.autor == primera.victima && despues.victima == primera.autor &&
				despues.cuando.After(primera.cuando) {
				return
			}
		}
	}
	t.Fatal("no hay ninguna venganza en la partida guionada")
}

func TestHayReconexionYAbandono(t *testing.T) {
	// El analisis distingue los dos casos; el simulador tiene que producir ambos.
	flujo := flujoDePrueba(t)

	conRegreso := flujo.Lotes["m-sim-combat-streak"]
	desconecta, reconecta := 0, 0
	for _, e := range conRegreso.Events {
		switch e.Type {
		case PlayerDisconnected:
			desconecta++
		case PlayerReconnected:
			reconecta++
		}
	}
	if desconecta == 0 || reconecta == 0 {
		t.Errorf("la partida con reconexion tiene %d cortes y %d regresos", desconecta, reconecta)
	}

	abandono := flujo.Lotes["m-sim-combat-abandono"]
	cortes, regresos := 0, 0
	for _, e := range abandono.Events {
		switch e.Type {
		case PlayerDisconnected:
			cortes++
		case PlayerReconnected:
			regresos++
		}
	}
	if cortes == 0 {
		t.Error("la partida de abandono no tiene ninguna desconexion")
	}
	if regresos != 0 {
		t.Errorf("la partida de abandono tiene %d regresos, deberia no tener ninguno", regresos)
	}
}

// --------------------------------------------------------------------- blackjack

func TestElOutcomeConcuerdaConLasCartas(t *testing.T) {
	// La coherencia de blackjack: el resultado de cada ronda tiene que salir de las manos, no estar
	// elegido. Se comprueba en todas las partidas generadas, no solo en la guionada.
	for id, lote := range flujoDePrueba(t).Lotes {
		if lote.GameID != "blackjack" {
			continue
		}

		var mano, crupier []string
		ronda := 0
		for _, e := range lote.Events {
			switch e.Type {
			case BetPlaced:
				mano, crupier = nil, nil
				ronda++
			case CardDealt:
				data := e.Data.(map[string]any)
				if data["to"].(string) == "p1" {
					mano = append(mano, data["card"].(string))
				} else {
					crupier = append(crupier, data["card"].(string))
				}
			case RoundResult:
				esperado := resultadoDeRonda(mano, crupier)
				obtenido := e.Data.(map[string]any)["outcome"].(string)
				if obtenido != esperado {
					t.Errorf("%s ronda %d: outcome %s pero %v (%d) contra %v (%d) da %s",
						id, ronda, obtenido, mano, valorDeMano(mano), crupier, valorDeMano(crupier), esperado)
				}
			}
		}
	}
}

func TestElTiltGuionadoSubeLaApuestaTrasTresDerrotas(t *testing.T) {
	lote := flujoDePrueba(t).Lotes["m-sim-blackjack-tilt"]

	var apuestas []int
	var resultados []string
	for _, e := range lote.Events {
		switch e.Type {
		case BetPlaced:
			apuestas = append(apuestas, e.Data.(map[string]any)["amount"].(int))
		case RoundResult:
			resultados = append(resultados, e.Data.(map[string]any)["outcome"].(string))
		}
	}

	perdio := func(o string) bool { return o == "LOSE" || o == "BUST" }
	for i := 0; i+3 < len(resultados); i++ {
		if perdio(resultados[i]) && perdio(resultados[i+1]) && perdio(resultados[i+2]) &&
			apuestas[i+3] > apuestas[i+2] {
			return
		}
	}
	t.Fatalf("no hay tilt: apuestas %v, resultados %v", apuestas, resultados)
}

func TestElValorDeLaManoAplicaLaReglaDelAs(t *testing.T) {
	casos := []struct {
		cartas []string
		valor  int
	}{
		{[]string{"10H", "7S"}, 17},
		{[]string{"AS", "KS"}, 21},
		{[]string{"AS", "9C", "5H"}, 15}, // el as baja a 1
		{[]string{"AS", "AD", "9C"}, 21}, // degrada uno solo, no los dos
		{[]string{"AS", "AD", "AH", "AC", "7S"}, 21},
		{[]string{"10C", "6H", "8S"}, 24}, // pasada: devuelve el total real
		{nil, 0},
	}
	for _, c := range casos {
		if valor := valorDeMano(c.cartas); valor != c.valor {
			t.Errorf("%v deberia valer %d, vale %d", c.cartas, c.valor, valor)
		}
	}
}

// --------------------------------------------------------------------- battleship

func TestLaRachaDeAciertosGuionadaExiste(t *testing.T) {
	lote := flujoDePrueba(t).Lotes["m-sim-battleship-streak"]

	mejor, actual := 0, 0
	for _, e := range lote.Events {
		if e.Type != ShotFired || *e.PlayerID != "p1" {
			continue
		}
		if e.Data.(map[string]any)["hit"].(bool) {
			actual++
			if actual > mejor {
				mejor = actual
			}
		} else {
			actual = 0
		}
	}
	if mejor < 3 {
		t.Fatalf("la racha mas larga de p1 es de %d aciertos, hacen falta 3", mejor)
	}
}

func TestNoSeHundeUnBarcoSinSuficientesImpactos(t *testing.T) {
	// Coherencia de battleship: un barco de tres casillas no puede hundirse con dos aciertos.
	for id, lote := range flujoDePrueba(t).Lotes {
		if lote.GameID != "battleship" {
			continue
		}

		impactos := map[string]int{}
		for _, e := range lote.Events {
			switch e.Type {
			case ShotFired:
				if e.Data.(map[string]any)["hit"].(bool) {
					impactos[*e.PlayerID]++
				}
			case ShipSunk:
				tam := e.Data.(map[string]any)["size"].(int)
				// El dueno del barco es el playerId; lo hundio el otro.
				var autor string
				for _, j := range []string{"p1", "bot1"} {
					if j != *e.PlayerID {
						autor = j
					}
				}
				if impactos[autor] < tam {
					t.Errorf("%s: se hundio un barco de %d con solo %d impactos de %s",
						id, tam, impactos[autor], autor)
				}
			}
		}
	}
}

// --------------------------------------------------------------------- casos invalidos

func TestHayUnCasoPorCadaRazonDelCatalogo(t *testing.T) {
	// Si alguien agrega una razon a contracts/rejections.md, esta prueba lo recuerda.
	catalogo, err := os.ReadFile("../contracts/rejections.md")
	if err != nil {
		t.Skipf("no se pudo leer rejections.md: %v", err)
	}

	patron := regexp.MustCompile("(?m)^\\| `([a-z_]+)` \\|")
	esperadas := map[string]bool{}
	for _, m := range patron.FindAllStringSubmatch(string(catalogo), -1) {
		esperadas[m[1]] = true
	}
	if len(esperadas) == 0 {
		t.Fatal("no se pudieron leer las razones de rejections.md")
	}

	generadas := map[string]bool{}
	for _, c := range casosInvalidos("2026-10-05T16:00:00.000Z").Cases {
		generadas[c.ExpectedReason] = true
	}
	for razon := range esperadas {
		if !generadas[razon] {
			t.Errorf("falta un caso invalido para %s", razon)
		}
	}
}

func TestCadaCasoTraeEventoOCuerpoCrudo(t *testing.T) {
	for _, c := range casosInvalidos("2026-10-05T16:00:00.000Z").Cases {
		if c.Event == nil && c.RawBody == "" {
			t.Errorf("%s no trae ni event ni rawBody", c.ExpectedReason)
		}
		if c.Event != nil && c.RawBody != "" {
			t.Errorf("%s trae los dos, deberia traer uno", c.ExpectedReason)
		}
	}
}
