package main

import (
	"fmt"
	"math/rand"
	"time"
)

const (
	ShotFired = "SHOT_FIRED"
	ShipSunk  = "SHIP_SUNK"
	TableroN  = 10
)

type barco struct {
	nombre string
	tam    int
}

type disparo struct {
	segundo int
	quien   string
	coord   string
	acierto bool
	// hunde indica el barco que se va a pique con este disparo, y de quien era. El SHIP_SUNK lleva
	// en el playerId al **dueno** del barco, no a quien disparo, igual que la victima en combate.
	hunde *hundimiento
}

type hundimiento struct {
	dueno  string
	nombre string
	tam    int
}

func construirBattleship(
	matchID string,
	inicio time.Time,
	humano, maquina string,
	flota []barco,
	disparos []disparo,
) partida {
	c := nuevo(matchID, "battleship", inicio)

	lista := make([]map[string]any, 0, len(flota))
	for _, b := range flota {
		lista = append(lista, map[string]any{"ship": b.nombre, "size": b.tam})
	}
	c.add(0, MatchStarted, nil, map[string]any{
		"matchMode": "PVE",
		"boardSize": TableroN,
		"fleet":     lista,
	})
	c.unirse(1*time.Second, humano, false, "")
	c.unirse(2*time.Second, maquina, true, "easy")

	for _, d := range disparos {
		c.add(time.Duration(d.segundo)*time.Second, ShotFired, jugador(d.quien), map[string]any{
			"coord": d.coord,
			"hit":   d.acierto,
		})
		if d.hunde != nil {
			c.add(time.Duration(d.segundo+1)*time.Second, ShipSunk, jugador(d.hunde.dueno), map[string]any{
				"ship": d.hunde.nombre,
				"size": d.hunde.tam,
			})
		}
	}

	ultimo := 0
	for _, d := range disparos {
		if d.segundo > ultimo {
			ultimo = d.segundo
		}
	}
	c.add(time.Duration(ultimo+5)*time.Second, MatchFinished, nil, nil)
	return c.partida()
}

// battleshipConRacha: p1 encadena cinco aciertos seguidos y hunde la flota rival completa.
//
// La flota va reducida a dos barcos a proposito, para que el ejemplo quepa: con la flota estandar
// de cinco harian falta diecisiete aciertos y el archivo seria ilegible.
func battleshipConRacha(matchID string, inicio time.Time) partida {
	flota := []barco{{"destroyer", 2}, {"cruiser", 3}}
	return construirBattleship(matchID, inicio, "p1", "bot1", flota, []disparo{
		{10, "p1", "A1", false, nil},
		{14, "bot1", "D1", true, nil},
		{18, "p1", "B2", true, nil},
		{22, "bot1", "D2", true, &hundimiento{"p1", "destroyer", 2}},
		{26, "p1", "B3", true, &hundimiento{"bot1", "destroyer", 2}},
		{30, "bot1", "F5", false, nil},
		{34, "p1", "C5", true, nil},
		{38, "bot1", "G7", false, nil},
		{42, "p1", "C6", true, nil},
		{46, "bot1", "H2", false, nil},
		{50, "p1", "C7", true, &hundimiento{"bot1", "cruiser", 3}},
	})
}

// battleshipAlAzar reparte aciertos y fallos al azar. Los barcos se hunden cuando se acumulan
// tantos aciertos como casillas tiene el siguiente barco de la flota, asi el dato queda coherente:
// nunca se hunde un barco con menos impactos de los que mide.
func battleshipAlAzar(matchID string, inicio time.Time, r *rand.Rand) partida {
	flota := []barco{{"destroyer", 2}, {"cruiser", 3}}

	coord := func() string {
		return fmt.Sprintf("%c%d", rune('A'+r.Intn(TableroN)), 1+r.Intn(TableroN))
	}

	var disparos []disparo
	segundo := 10
	impactos := map[string]int{"p1": 0, "bot1": 0}
	hundidos := map[string]int{"p1": 0, "bot1": 0}
	turno := "p1"
	rival := map[string]string{"p1": "bot1", "bot1": "p1"}

	for i := 0; i < 10+r.Intn(10); i++ {
		acierto := r.Float64() < 0.5
		d := disparo{segundo: segundo, quien: turno, coord: coord(), acierto: acierto}

		if acierto {
			impactos[turno]++
			siguiente := hundidos[turno]
			if siguiente < len(flota) && impactos[turno] >= flota[siguiente].tam {
				d.hunde = &hundimiento{
					dueno:  rival[turno],
					nombre: flota[siguiente].nombre,
					tam:    flota[siguiente].tam,
				}
				hundidos[turno]++
				impactos[turno] = 0
			}
		}

		disparos = append(disparos, d)
		segundo += 3 + r.Intn(5)
		turno = rival[turno]
	}
	return construirBattleship(matchID, inicio, "p1", "bot1", flota, disparos)
}
