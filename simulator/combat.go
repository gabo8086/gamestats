package main

import (
	"math/rand"
	"time"
)

const PlayerEliminated = "PLAYER_ELIMINATED"

var (
	armas  = []string{"rifle", "shotgun", "pistol", "sniper"}
	arenas = []string{"Bodega", "Azotea", "Subsuelo", "Mercado"}
)

type baja struct {
	segundo int
	autor   string
	victima string
	arma    string
	dano    int
}

type participante struct {
	id    string
	bot   bool
	nivel string
}

// construirCombate arma una partida de combate a partir de la lista de bajas.
//
// En PLAYER_ELIMINATED el playerId del sobre es **quien elimina** y la victima va en data. Los bots
// participan como jugadores cualesquiera: pueden eliminar y ser eliminados, y lo unico que los
// distingue es su playerType.
func construirCombate(
	matchID string,
	inicio time.Time,
	jugadores []participante,
	bajas []baja,
	arena string,
	modo string,
	cortes []corte,
) partida {
	hayBots := false
	for _, j := range jugadores {
		if j.bot {
			hayBots = true
		}
	}
	matchMode := "PVP"
	if hayBots {
		matchMode = "PVE"
	}

	c := nuevo(matchID, "combat", inicio)
	c.add(0, MatchStarted, nil, map[string]any{
		"matchMode": matchMode,
		"mode":      modo,
		"arena":     arena,
	})
	for i, j := range jugadores {
		c.unirse(time.Duration(200*(i+1))*time.Millisecond, j.id, j.bot, j.nivel)
	}

	for _, b := range bajas {
		c.add(time.Duration(b.segundo)*time.Second, PlayerEliminated, jugador(b.autor), map[string]any{
			"victimId": b.victima,
			"weapon":   b.arma,
			"damage":   b.dano,
		})
	}

	for _, d := range cortes {
		c.add(time.Duration(d.seDesconecta)*time.Second, PlayerDisconnected, jugador(d.jugador), nil)
		if d.vuelve >= 0 {
			c.add(time.Duration(d.vuelve)*time.Second, PlayerReconnected, jugador(d.jugador), nil)
		}
	}

	ultima := 0
	for _, b := range bajas {
		if b.segundo > ultima {
			ultima = b.segundo
		}
	}
	c.add(time.Duration(ultima+10)*time.Second, MatchFinished, nil, nil)
	return c.partida()
}

// corte describe una desconexion. vuelve < 0 significa que el jugador no regresa: abandono.
type corte struct {
	jugador      string
	seDesconecta int
	vuelve       int
}

// combateConRachaYVenganza dispara las dos reglas de combate y ademas mete bots y una desconexion.
//
// Racha de p3: elimina en los segundos 50, 53 y 58 —tres bajas en una ventana de 8 s— y no aparece
// como victima entre medio; lo matan recien en el 80.
//
// Venganzas: bot1 elimina a p1 en el segundo 15 y p1 se lo cobra en el 30; p3 elimina a p2 en el 50
// y p2 se lo cobra en el 80.
//
// p4 se desconecta en el segundo 40 y vuelve en el 70: 30 s, dentro de la ventana de reconexion.
func combateConRachaYVenganza(matchID string, inicio time.Time) partida {
	return construirCombate(matchID, inicio,
		[]participante{
			{id: "p1"}, {id: "p2"}, {id: "p3"},
			{id: "bot1", bot: true, nivel: "easy"},
			{id: "p4"},
		},
		[]baja{
			{15, "bot1", "p1", "pistol", 70},
			{30, "p1", "bot1", "rifle", 115},
			{50, "p3", "p2", "shotgun", 90},
			{53, "p3", "bot1", "shotgun", 105},
			{58, "p3", "p1", "rifle", 125},
			{80, "p2", "p3", "sniper", 140},
		},
		"Azotea", "deathmatch",
		[]corte{{jugador: "p4", seDesconecta: 40, vuelve: 70}})
}

// combateConAbandono: un jugador se va y no vuelve. Sirve para que el analisis tenga el caso de
// abandono y no solo el de reconexion.
func combateConAbandono(matchID string, inicio time.Time) partida {
	return construirCombate(matchID, inicio,
		[]participante{
			{id: "p1"}, {id: "p5"},
			{id: "bot2", bot: true, nivel: "hard"},
		},
		[]baja{
			{12, "p1", "bot2", "rifle", 110},
			{25, "bot2", "p5", "shotgun", 95},
			{40, "p1", "bot2", "sniper", 130},
		},
		"Subsuelo", "deathmatch",
		[]corte{{jugador: "p5", seDesconecta: 30, vuelve: -1}})
}

// combateAlAzar: las bajas se reparten al azar y las reglas pueden o no dispararse.
func combateAlAzar(matchID string, inicio time.Time, jugadores []participante, r *rand.Rand) partida {
	var bajas []baja
	segundo := 8 + r.Intn(13)
	for i := 0; i < 4+r.Intn(6); i++ {
		a := r.Intn(len(jugadores))
		v := r.Intn(len(jugadores))
		for v == a {
			v = r.Intn(len(jugadores))
		}
		bajas = append(bajas, baja{
			segundo: segundo,
			autor:   jugadores[a].id,
			victima: jugadores[v].id,
			arma:    armas[r.Intn(len(armas))],
			dano:    60 + r.Intn(91),
		})
		segundo += 4 + r.Intn(22)
	}
	return construirCombate(matchID, inicio, jugadores, bajas, arenas[r.Intn(len(arenas))], "deathmatch", nil)
}
