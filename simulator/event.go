package main

import "time"

// GameVersion viaja en todos los eventos; el contrato la exige y hoy es la misma para los cuatro
// juegos.
const GameVersion = "1.0"

// Tipos de evento generales, los unicos que comparten los cuatro juegos.
const (
	MatchStarted       = "MATCH_STARTED"
	PlayerJoined       = "PLAYER_JOINED"
	PlayerDisconnected = "PLAYER_DISCONNECTED"
	PlayerReconnected  = "PLAYER_RECONNECTED"
	MatchFinished      = "MATCH_FINISHED"
)

// Valores de PLAYER_JOINED.data.playerType.
const (
	Human = "HUMAN"
	Bot   = "BOT"
)

// Event es el sobre comun, espejo de contracts/event.schema.json. El orden de los campos es el del
// contrato porque encoding/json serializa en el orden de declaracion, y asi los archivos generados
// se leen igual que los ejemplos de contracts/examples/.
//
// PlayerID y Action son punteros para poder emitir null: el sobre es cerrado, de modo que omitir un
// campo seria tan incorrecto como inventarlo.
type Event struct {
	EventID     string  `json:"eventId"`
	Timestamp   string  `json:"timestamp"`
	GameID      string  `json:"gameId"`
	GameVersion string  `json:"gameVersion"`
	MatchID     string  `json:"matchId"`
	PlayerID    *string `json:"playerId"`
	Type        string  `json:"type"`
	Action      *string `json:"action"`
	Data        any     `json:"data"`
}

// Batch es el cuerpo del POST /analyze, espejo de contracts/batch.schema.json.
type Batch struct {
	MatchID     string  `json:"matchId"`
	GameID      string  `json:"gameId"`
	GameVersion string  `json:"gameVersion"`
	Events      []Event `json:"events"`
}

// marca formatea un instante como lo exige el contrato: UTC, milisegundos y sufijo Z.
func marca(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

func jugador(id string) *string { return &id }

// partida es lo que produce cada generador antes de que se le asignen los eventId: el identificador
// se reparte despues, de una sola vez, para que no dependa del orden en que terminen las goroutines.
type partida struct {
	matchID string
	gameID  string
	eventos []Event
}

// constructor acumula los eventos de una partida con desplazamientos sobre su instante de arranque.
type constructor struct {
	matchID string
	gameID  string
	inicio  time.Time
	eventos []Event
}

func nuevo(matchID, gameID string, inicio time.Time) *constructor {
	return &constructor{matchID: matchID, gameID: gameID, inicio: inicio}
}

func (c *constructor) add(desde time.Duration, tipo string, quien *string, data any) {
	c.eventos = append(c.eventos, Event{
		Timestamp:   marca(c.inicio.Add(desde)),
		GameID:      c.gameID,
		GameVersion: GameVersion,
		MatchID:     c.matchID,
		PlayerID:    quien,
		Type:        tipo,
		Data:        data,
	})
}

// unirse emite el PLAYER_JOINED con su playerType. Los bots llevan ademas botLevel.
func (c *constructor) unirse(desde time.Duration, id string, bot bool, nivel string) {
	data := map[string]any{"playerType": Human}
	if bot {
		data = map[string]any{"playerType": Bot, "botLevel": nivel}
	}
	c.add(desde, PlayerJoined, jugador(id), data)
}

func (c *constructor) partida() partida {
	return partida{matchID: c.matchID, gameID: c.gameID, eventos: c.eventos}
}
