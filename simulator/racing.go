package main

import (
	"math/rand"
	"sort"
	"time"
)

const (
	LapCompleted = "LAP_COMPLETED"
	Penalty      = "PENALTY"
)

var pistas = []string{"Circuito Volcan", "Bahia Norte", "Valle Seco", "Puerto Viejo"}

type penalizacion struct {
	jugador  string
	segundos int
	razon    string
}

// construirCarrera arma una partida de carreras a partir de los tiempos de vuelta de cada jugador.
//
// La coherencia de los datos sale de aqui: la position de una vuelta **se deriva** del tiempo
// acumulado, no se inventa. Si p1 lleva menos acumulado que p2 al cerrar la vuelta 2, p1 va delante
// en esa vuelta. Sin esto las estadisticas salen contradictorias y la remontada no se puede
// justificar en la defensa.
//
// El timestamp de cada LAP_COMPLETED es el instante real en que ese jugador cruza la meta, o sea
// inicio + acumulado. Por eso los eventos de los distintos jugadores quedan intercalados solos.
func construirCarrera(
	matchID string,
	inicio time.Time,
	jugadores []string,
	tiempos map[string][]int,
	pista string,
	penalizaciones []penalizacion,
) partida {
	vueltas := len(tiempos[jugadores[0]])

	c := nuevo(matchID, "racing", inicio)
	c.add(0, MatchStarted, nil, map[string]any{
		"matchMode": "PVP",
		"laps":      vueltas,
		"track":     pista,
	})
	for i, id := range jugadores {
		c.unirse(time.Duration(200*(i+1))*time.Millisecond, id, false, "")
	}

	acumulado := make(map[string]int, len(jugadores))
	for vuelta := 0; vuelta < vueltas; vuelta++ {
		for _, id := range jugadores {
			acumulado[id] += tiempos[id][vuelta]
		}

		// El puesto es por tiempo acumulado al cerrar esta vuelta. Se ordena una copia para no
		// depender del orden de recorrido de un mapa, que en Go no esta definido.
		orden := append([]string(nil), jugadores...)
		sort.Slice(orden, func(i, j int) bool { return acumulado[orden[i]] < acumulado[orden[j]] })
		posicion := make(map[string]int, len(orden))
		for i, id := range orden {
			posicion[id] = i + 1
		}

		for _, id := range jugadores {
			c.add(time.Duration(acumulado[id])*time.Millisecond, LapCompleted, jugador(id), map[string]any{
				"lap":       vuelta + 1,
				"lapTimeMs": tiempos[id][vuelta],
				"position":  posicion[id],
			})
		}
	}

	for _, p := range penalizaciones {
		// Se registra a mitad de carrera. No altera los tiempos de vuelta, solo el tiempo ajustado
		// que calcula Scala.
		c.add(time.Duration(acumulado[p.jugador]/2)*time.Millisecond, Penalty, jugador(p.jugador),
			map[string]any{"seconds": p.segundos, "reason": p.razon})
	}

	ultimo := 0
	for _, id := range jugadores {
		if acumulado[id] > ultimo {
			ultimo = acumulado[id]
		}
	}
	c.add(time.Duration(ultimo)*time.Millisecond+5*time.Second, MatchFinished, nil, nil)
	return c.partida()
}

// carreraConRemontada: p3 cierra la primera vuelta ultimo y gana la carrera.
//
// Los tiempos estan escritos a mano para que la remontada ocurra con seguridad. Acumulados:
//
//	vuelta 1   p1  80000 (1.o)   p2  81000 (2.o)   p3  90000 (3.o, ultimo)
//	vuelta 2   p2 163000 (1.o)   p1 165000 (2.o)   p3 166000 (3.o)
//	vuelta 3   p3 238000 (1.o)   p2 246000 (2.o)   p1 250000 (3.o)
//
// p3 estuvo en la ultima posicion (position == 3 == numero de jugadores) y gana por tiempo
// ajustado: es exactamente la definicion de remontada de docs/diseno.md.
func carreraConRemontada(matchID string, inicio time.Time) partida {
	return construirCarrera(matchID, inicio,
		[]string{"p1", "p2", "p3"},
		map[string][]int{
			"p1": {80_000, 85_000, 85_000},
			"p2": {81_000, 82_000, 83_000},
			"p3": {90_000, 76_000, 72_000},
		},
		"Circuito Volcan", nil)
}

// carreraConPenalizacionDecisiva: p1 llega primero por tiempo de vuelta pero pierde por la
// penalizacion.
//
// Sin penalizar: p1 240000, p2 243000. Con los 5 s de p1 el tiempo ajustado queda p1 245000 contra
// p2 243000, asi que gana p2. Es el caso que demuestra que el ganador sale de combinar dos tipos de
// evento distintos y no de uno solo.
func carreraConPenalizacionDecisiva(matchID string, inicio time.Time) partida {
	return construirCarrera(matchID, inicio,
		[]string{"p1", "p2", "p4"},
		map[string][]int{
			"p1": {80_000, 80_000, 80_000},
			"p2": {81_000, 81_000, 81_000},
			"p4": {84_000, 83_000, 85_000},
		},
		"Bahia Norte",
		[]penalizacion{{"p1", 5, "track_cut"}})
}

// carreraAlAzar: cada jugador tiene un ritmo base y variacion vuelta a vuelta.
func carreraAlAzar(matchID string, inicio time.Time, jugadores []string, vueltas int, r *rand.Rand) partida {
	tiempos := make(map[string][]int, len(jugadores))
	for _, id := range jugadores {
		ritmo := 74_000 + r.Intn(19_000)
		serie := make([]int, vueltas)
		for v := range serie {
			serie[v] = ritmo + r.Intn(8_000) - 3_000
		}
		tiempos[id] = serie
	}

	var penalizaciones []penalizacion
	if r.Float64() < 0.4 {
		razones := []string{"track_cut", "collision", "speeding_pit"}
		segundos := []int{3, 5, 6, 10}
		penalizaciones = append(penalizaciones, penalizacion{
			jugador:  jugadores[r.Intn(len(jugadores))],
			segundos: segundos[r.Intn(len(segundos))],
			razon:    razones[r.Intn(len(razones))],
		})
	}

	return construirCarrera(matchID, inicio, jugadores, tiempos, pistas[r.Intn(len(pistas))], penalizaciones)
}
