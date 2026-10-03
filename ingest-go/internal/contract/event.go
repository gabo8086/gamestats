// Package contract refleja en Go el sobre de evento definido en contracts/event.schema.json.
//
// Es lo unico que Go sabe del formato: el campo Data queda como JSON crudo porque la ingestion
// no necesita entender lo especifico de cada juego. Eso es responsabilidad del modulo Scala.
package contract

import (
	"bytes"
	"encoding/json"
	"strings"
)

// Event es el sobre comun de todo evento. Los punteros distinguen "ausente o null" de "vacio",
// que es justo lo que la validacion necesita diferenciar.
type Event struct {
	EventID     string          `json:"eventId"`
	Timestamp   string          `json:"timestamp"`
	GameID      string          `json:"gameId"`
	GameVersion string          `json:"gameVersion"`
	MatchID     string          `json:"matchId"`
	PlayerID    *string         `json:"playerId"`
	Type        string          `json:"type"`
	Action      *string         `json:"action"`
	Data        json.RawMessage `json:"data"`
}

// Batch es el cuerpo del POST /analyze hacia Scala (contracts/batch.schema.json).
type Batch struct {
	MatchID     string  `json:"matchId"`
	GameID      string  `json:"gameId"`
	GameVersion string  `json:"gameVersion"`
	Events      []Event `json:"events"`
}

// Tipos de evento generales, comunes a todos los juegos.
const (
	TypeMatchStarted  = "MATCH_STARTED"
	TypePlayerJoined  = "PLAYER_JOINED"
	TypeMatchFinished = "MATCH_FINISHED"
)

// Razones de rechazo. El catalogo completo y el porque de cada una estan en contracts/rejections.md.
const (
	ReasonMalformedJSON        = "malformed_json"
	ReasonMissingRequiredField = "missing_required_field"
	ReasonInvalidTimestamp     = "invalid_timestamp"
	ReasonMissingPlayerID      = "missing_player_id"
	ReasonUnknownField         = "unknown_field"
	ReasonDuplicateEvent       = "duplicate_event"
	ReasonMatchNotStarted      = "match_not_started"
	ReasonMatchAlreadyFinished = "match_already_finished"
)

// IsMatchEvent dice si el evento es de los MATCH_*, los unicos que pueden venir sin playerId.
func (e Event) IsMatchEvent() bool {
	return strings.HasPrefix(e.Type, "MATCH_")
}

// DecodeEvent lee un evento rechazando campos que no estan en el contrato (razon unknown_field).
// El sobre es cerrado a proposito: un campo de mas suele ser un error del productor, y preferimos
// avisarle a aceptarlo en silencio.
func DecodeEvent(raw []byte) (Event, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	var ev Event
	if err := dec.Decode(&ev); err != nil {
		return Event{}, err
	}
	return ev, nil
}
