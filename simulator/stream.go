package main

import (
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"
)

// Base es el instante de arranque del flujo. Fijo a proposito: con la misma semilla, el flujo
// generado es byte a byte el mismo, y eso es lo que permite repetir una demo y fijar resultados
// esperados en las pruebas.
var Base = time.Date(2026, 10, 5, 14, 0, 0, 0, time.UTC)

// Flujo es todo lo que produce el simulador.
type Flujo struct {
	Eventos   []Event
	Lotes     map[string]Batch
	Resumen   Resumen
	Invalidos ColeccionInvalida
}

type Resumen struct {
	Seed         int64             `json:"seed"`
	Matches      int               `json:"matches"`
	Events       int               `json:"events"`
	SwappedPairs int               `json:"swappedPairs"`
	ByGame       map[string]int    `json:"byGame"`
	Scripted     map[string]string `json:"scripted"`
}

// Generar produce el flujo completo.
//
// **Cada partida se genera en su propia goroutine.** No es decorativo: es el mismo modelo que usa
// el modulo de Go para procesarlas, y aqui se ve que varias partidas avanzan a la vez.
//
// El determinismo se mantiene porque cada partida recibe su propio generador de azar, sembrado a
// partir de la semilla global y de su indice, y porque **los eventId se reparten despues**, cuando
// ya se ordenaron las partidas. Si se asignaran dentro de las goroutines dependerian de cual
// termina primero y el flujo dejaria de ser reproducible.
func Generar(semilla int64, partidasAlAzar, desorden int) Flujo {
	type tarea struct {
		indice int
		hacer  func(*rand.Rand) partida
	}

	// Las guionadas van primero y con indice fijo, para que sus eventId no se muevan aunque cambie
	// el numero de partidas al azar: una demo repetida habla siempre de los mismos identificadores.
	tareas := []tarea{
		{0, func(*rand.Rand) partida { return carreraConRemontada("m-sim-racing-comeback", Base) }},
		{1, func(*rand.Rand) partida {
			return carreraConPenalizacionDecisiva("m-sim-racing-penalty", Base.Add(30*time.Second))
		}},
		{2, func(*rand.Rand) partida {
			return combateConRachaYVenganza("m-sim-combat-streak", Base.Add(15*time.Second))
		}},
		{3, func(*rand.Rand) partida {
			return combateConAbandono("m-sim-combat-abandono", Base.Add(45*time.Second))
		}},
		{4, func(*rand.Rand) partida { return blackjackConTilt("m-sim-blackjack-tilt", Base.Add(20*time.Second)) }},
		{5, func(*rand.Rand) partida {
			return battleshipConRacha("m-sim-battleship-streak", Base.Add(25*time.Second))
		}},
	}

	guionadas := len(tareas)
	for i := 0; i < partidasAlAzar; i++ {
		indice := guionadas + i
		numero := i
		tareas = append(tareas, tarea{indice, func(r *rand.Rand) partida {
			return partidaAlAzar(numero, r)
		}})
	}

	resultados := make([]partida, len(tareas))
	var wg sync.WaitGroup
	for _, t := range tareas {
		wg.Add(1)
		go func(t tarea) {
			defer wg.Done()
			// Semilla propia por partida: dos partidas distintas no comparten estado, asi que no
			// hace falta ningun candado y el resultado no depende del orden de ejecucion.
			r := rand.New(rand.NewSource(semilla*1_000_003 + int64(t.indice)))
			resultados[t.indice] = t.hacer(r)
		}(t)
	}
	wg.Wait()

	// A partir de aqui todo es determinista: orden fijo de partidas y reparto de eventId.
	sort.SliceStable(resultados, func(i, j int) bool { return resultados[i].matchID < resultados[j].matchID })

	siguiente := 0
	lotes := make(map[string]Batch, len(resultados))
	var todos []Event
	porJuego := map[string]int{}

	for _, p := range resultados {
		ordenados := append([]Event(nil), p.eventos...)
		sort.SliceStable(ordenados, func(i, j int) bool { return ordenados[i].Timestamp < ordenados[j].Timestamp })
		for i := range ordenados {
			siguiente++
			ordenados[i].EventID = fmt.Sprintf("s-%06d", siguiente)
		}
		lotes[p.matchID] = Batch{MatchID: p.matchID, GameID: p.gameID, GameVersion: GameVersion, Events: ordenados}
		todos = append(todos, ordenados...)
		porJuego[p.gameID]++
	}

	sort.SliceStable(todos, func(i, j int) bool { return todos[i].Timestamp < todos[j].Timestamp })
	flujo := desordenar(todos, desorden, rand.New(rand.NewSource(semilla)))

	return Flujo{
		Eventos:   flujo,
		Lotes:     lotes,
		Invalidos: casosInvalidos(marca(Base.Add(2 * time.Hour))),
		Resumen: Resumen{
			Seed: semilla, Matches: len(resultados), Events: len(flujo),
			SwappedPairs: desorden, ByGame: porJuego,
			Scripted: map[string]string{
				"m-sim-racing-comeback":   "p3 cierra la vuelta 1 ultimo y gana: dispara 'comeback'",
				"m-sim-racing-penalty":    "p1 es el mas rapido pero la penalizacion le da la victoria a p2",
				"m-sim-combat-streak":     "p3 elimina 3 veces en 8 s sin morir: 'streak', mas dos 'revenge' y una reconexion",
				"m-sim-combat-abandono":   "p5 se desconecta y no vuelve: abandono",
				"m-sim-blackjack-tilt":    "tres derrotas seguidas y la apuesta sube de 10 a 30: 'tilt'",
				"m-sim-battleship-streak": "p1 encadena cinco aciertos y hunde la flota: 'hitStreak'",
			},
		},
	}
}

// partidaAlAzar reparte los juegos de forma pareja entre las partidas no guionadas.
func partidaAlAzar(numero int, r *rand.Rand) partida {
	arranque := Base.Add(time.Duration(r.Intn(120)) * time.Second)

	switch numero % 4 {
	case 0:
		jugadores := []string{}
		for n := 1; n <= 3+r.Intn(2); n++ {
			jugadores = append(jugadores, fmt.Sprintf("p%d", n))
		}
		return carreraAlAzar(fmt.Sprintf("m-sim-racing-%02d", numero), arranque, jugadores, 3+r.Intn(3), r)
	case 1:
		jugadores := []participante{}
		for n := 1; n <= 3+r.Intn(2); n++ {
			jugadores = append(jugadores, participante{id: fmt.Sprintf("p%d", n)})
		}
		// Una de cada dos lleva bot, para que haya partidas PVP y PVE entre las del azar.
		if numero%8 == 1 {
			jugadores = append(jugadores, participante{id: "bot1", bot: true, nivel: "easy"})
		}
		return combateAlAzar(fmt.Sprintf("m-sim-combat-%02d", numero), arranque, jugadores, r)
	case 2:
		return blackjackAlAzar(fmt.Sprintf("m-sim-blackjack-%02d", numero), arranque, r)
	default:
		return battleshipAlAzar(fmt.Sprintf("m-sim-battleship-%02d", numero), arranque, r)
	}
}

// desordenar intercambia pares de eventos adyacentes para que el flujo llegue desordenado.
//
// Go tiene que ordenar por timestamp antes de mandar el lote, asi que conviene darle trabajo. Pero
// el desorden no puede inventar rechazos falsos: si un evento se adelanta a su MATCH_STARTED o se
// atrasa detras de su MATCH_FINISHED, Go lo rechazaria con razon y no estariamos probando lo que
// queremos. Por eso solo se intercambian pares donde **ninguno** de los dos es un MATCH_*.
func desordenar(eventos []Event, cuantos int, r *rand.Rand) []Event {
	mezclados := append([]Event(nil), eventos...)

	var candidatos []int
	for i := 0; i+1 < len(mezclados); i++ {
		if !esDePartida(mezclados[i].Type) && !esDePartida(mezclados[i+1].Type) {
			candidatos = append(candidatos, i)
		}
	}

	r.Shuffle(len(candidatos), func(i, j int) { candidatos[i], candidatos[j] = candidatos[j], candidatos[i] })
	if cuantos > len(candidatos) {
		cuantos = len(candidatos)
	}
	for _, p := range candidatos[:cuantos] {
		mezclados[p], mezclados[p+1] = mezclados[p+1], mezclados[p]
	}
	return mezclados
}

func esDePartida(tipo string) bool {
	return tipo == MatchStarted || tipo == MatchFinished
}
