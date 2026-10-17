package main

import (
	"math/rand"
	"strconv"
	"time"
)

const (
	BetPlaced   = "BET_PLACED"
	CardDealt   = "CARD_DEALT"
	PlayerHit   = "PLAYER_HIT"
	PlayerStand = "PLAYER_STAND"
	RoundResult = "ROUND_RESULT"
)

var (
	rangos = []string{"A", "2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K"}
	palos  = []string{"H", "D", "C", "S"}
)

// valorDeMano suma las cartas con la regla del as: vale 11 salvo que la mano se pase de 21, y ahi
// baja a 1, de a uno. Es el mismo calculo que hace el analizador en Scala.
//
// El simulador lo necesita porque el valor de la mano **no viaja en los eventos**: para que el
// outcome que emite sea coherente con las cartas, hay que derivarlo, no elegirlo.
func valorDeMano(cartas []string) int {
	total, ases := 0, 0
	for _, carta := range cartas {
		rango := carta[:len(carta)-1]
		switch rango {
		case "A":
			total += 11
			ases++
		case "10", "J", "Q", "K":
			total += 10
		default:
			n, _ := strconv.Atoi(rango)
			total += n
		}
	}
	for total > 21 && ases > 0 {
		total -= 10
		ases--
	}
	return total
}

// resultadoDeRonda deduce el outcome comparando las dos manos. Nada se decide al azar aqui.
func resultadoDeRonda(mano, crupier []string) string {
	vj, vc := valorDeMano(mano), valorDeMano(crupier)
	switch {
	case vj > 21:
		return "BUST"
	case vj == 21 && len(mano) == 2:
		return "BLACKJACK"
	case vc > 21:
		return "WIN"
	case vj > vc:
		return "WIN"
	case vj == vc:
		return "PUSH"
	default:
		return "LOSE"
	}
}

// pagoDe traduce el outcome al neto para el jugador. El blackjack paga 3 a 2.
func pagoDe(outcome string, apuesta int) int {
	switch outcome {
	case "BLACKJACK":
		return apuesta * 3 / 2
	case "WIN":
		return apuesta
	case "PUSH":
		return 0
	default: // LOSE y BUST
		return -apuesta
	}
}

// ronda es una mano completa: lo apostado, las cartas de cada lado y las que el jugador pidio.
type ronda struct {
	apuesta int
	jugador []string
	crupier []string
	pedidas []string
}

// construirBlackjack arma la partida a partir de las rondas. El outcome y el pago se derivan; el
// generador no los elige.
func construirBlackjack(matchID string, inicio time.Time, rondas []ronda) partida {
	c := nuevo(matchID, "blackjack", inicio)
	c.add(0, MatchStarted, nil, map[string]any{"matchMode": "PVE", "decks": 6})
	c.unirse(1*time.Second, "p1", false, "")
	c.unirse(2*time.Second, "dealer", true, "hard")

	t := 10 * time.Second
	for _, r := range rondas {
		c.add(t, BetPlaced, jugador("p1"), map[string]any{"amount": r.apuesta})
		t += 2 * time.Second

		// Se reparte alternando, como en la mesa: una al jugador y una al crupier.
		for i := 0; i < 2; i++ {
			c.add(t, CardDealt, jugador("p1"), map[string]any{"card": r.jugador[i], "to": "p1"})
			t += time.Second
			c.add(t, CardDealt, jugador("p1"), map[string]any{"card": r.crupier[i], "to": "dealer"})
			t += time.Second
		}

		mano := append([]string(nil), r.jugador...)
		for _, carta := range r.pedidas {
			c.add(t, PlayerHit, jugador("p1"), nil)
			t += time.Second
			c.add(t, CardDealt, jugador("p1"), map[string]any{"card": carta, "to": "p1"})
			t += time.Second
			mano = append(mano, carta)
		}

		outcome := resultadoDeRonda(mano, r.crupier)
		if outcome != "BUST" {
			c.add(t, PlayerStand, jugador("p1"), nil)
			t += time.Second
		}
		c.add(t, RoundResult, jugador("p1"), map[string]any{
			"outcome": outcome,
			"payout":  pagoDe(outcome, r.apuesta),
		})
		t += 4 * time.Second
	}

	c.add(t, MatchFinished, nil, nil)
	return c.partida()
}

// blackjackConTilt: tres rondas perdidas seguidas y despues la apuesta sube de 10 a 30.
//
// Las cartas estan escritas a mano para garantizar las tres derrotas; el outcome igual se deriva de
// ellas, asi que si alguien cambia una carta el resultado cambia solo y no queda inconsistente.
func blackjackConTilt(matchID string, inicio time.Time) partida {
	return construirBlackjack(matchID, inicio, []ronda{
		{apuesta: 10, jugador: []string{"10H", "7S"}, crupier: []string{"KD", "9C"}},
		{apuesta: 10, jugador: []string{"9H", "8D"}, crupier: []string{"AS", "9S"}},
		{apuesta: 10, jugador: []string{"10C", "6H"}, crupier: []string{"7D", "QH"}, pedidas: []string{"8S"}},
		{apuesta: 30, jugador: []string{"AH", "KS"}, crupier: []string{"10D", "8C"}},
		{apuesta: 30, jugador: []string{"10S", "10H"}, crupier: []string{"QC", "10D"}},
		{apuesta: 30, jugador: []string{"AD", "9C"}, crupier: []string{"9H", "8H"}},
	})
}

// blackjackAlAzar reparte cartas de verdad y deja que el resultado salga de ellas.
//
// El jugador pide mientras no llegue a 17, que es la regla de la casa y alcanza para que aparezcan
// manos pasadas de 21 sin forzarlas.
func blackjackAlAzar(matchID string, inicio time.Time, r *rand.Rand) partida {
	carta := func() string {
		return rangos[r.Intn(len(rangos))] + palos[r.Intn(len(palos))]
	}

	apuestas := []int{5, 10, 20, 25, 50}
	var rondas []ronda
	for i := 0; i < 3+r.Intn(4); i++ {
		mano := []string{carta(), carta()}
		crupier := []string{carta(), carta()}

		var pedidas []string
		for valorDeMano(append(append([]string(nil), mano...), pedidas...)) < 17 {
			pedidas = append(pedidas, carta())
		}

		rondas = append(rondas, ronda{
			apuesta: apuestas[r.Intn(len(apuestas))],
			jugador: mano,
			crupier: crupier,
			pedidas: pedidas,
		})
	}
	return construirBlackjack(matchID, inicio, rondas)
}
