// Package contract refleja en Go el sobre de evento definido en contracts/event.schema.json.
//
// Es lo unico que Go sabe del formato: el campo Data queda como JSON crudo porque la ingestion
// no necesita entender lo especifico de cada juego. Eso es responsabilidad del modulo Scala.
package contract

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"
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

// Rejection es un evento rechazado, tal como aparece en la respuesta de POST /events
// (contracts/rejections.md). Las pruebas se apoyan en Reason; Detail es texto libre para depurar.
type Rejection struct {
	EventID string `json:"eventId,omitempty"`
	Reason  string `json:"reason"`
	Detail  string `json:"detail,omitempty"`
}

// Tipos de evento generales, comunes a todos los juegos.
const (
	TypeMatchStarted       = "MATCH_STARTED"
	TypePlayerJoined       = "PLAYER_JOINED"
	TypePlayerDisconnected = "PLAYER_DISCONNECTED"
	TypePlayerReconnected  = "PLAYER_RECONNECTED"
	TypeMatchFinished      = "MATCH_FINISHED"
)

// Razones de rechazo. El catalogo completo y el porque de cada una estan en contracts/rejections.md.
const (
	ReasonMalformedJSON              = "malformed_json"
	ReasonMissingRequiredField       = "missing_required_field"
	ReasonInvalidTimestamp           = "invalid_timestamp"
	ReasonMissingPlayerID            = "missing_player_id"
	ReasonUnknownField               = "unknown_field"
	ReasonDuplicateEvent             = "duplicate_event"
	ReasonMatchNotStarted            = "match_not_started"
	ReasonMatchAlreadyFinished       = "match_already_finished"
	ReasonReconnectWithoutDisconnect = "reconnect_without_disconnect"
)

// AllReasons es el catalogo en el orden de contracts/rejections.md. GET /metrics lo usa para
// mostrar cada razon aunque todavia no haya ocurrido ninguna vez.
var AllReasons = []string{
	ReasonMalformedJSON,
	ReasonMissingRequiredField,
	ReasonInvalidTimestamp,
	ReasonMissingPlayerID,
	ReasonUnknownField,
	ReasonDuplicateEvent,
	ReasonMatchNotStarted,
	ReasonMatchAlreadyFinished,
	ReasonReconnectWithoutDisconnect,
}

// IsMatchEvent dice si el evento es de los MATCH_*, los unicos que pueden venir sin playerId.
func (e Event) IsMatchEvent() bool {
	return strings.HasPrefix(e.Type, "MATCH_")
}

// Time devuelve el timestamp ya interpretado. Solo tiene sentido sobre un evento que paso Parse.
func (e Event) Time() time.Time {
	t, _ := time.Parse(time.RFC3339Nano, e.Timestamp)
	return t
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

// Parse decodifica un evento y valida su sobre. Cubre las razones que el esquema puede detectar
// mirando el evento aislado; las que necesitan el historial de la partida (duplicados, partida no
// iniciada o ya terminada, reconexion huerfana) son del despachador.
func Parse(raw []byte) (Event, *Rejection) {
	ev, err := DecodeEvent(raw)
	if err != nil {
		rej := &Rejection{EventID: peekEventID(raw), Reason: ReasonMalformedJSON, Detail: err.Error()}
		// encoding/json no exporta un tipo para este error, solo el mensaje.
		if campo, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
			rej.Reason = ReasonUnknownField
			rej.Detail = strings.Trim(campo, `"`)
		}
		return Event{}, rej
	}
	if rej := Validate(ev); rej != nil {
		return Event{}, rej
	}
	return ev, nil
}

// Validate aplica las reglas del sobre a un evento ya decodificado.
func Validate(ev Event) *Rejection {
	rechazo := func(razon, detalle string) *Rejection {
		return &Rejection{EventID: ev.EventID, Reason: razon, Detail: detalle}
	}

	obligatorios := []struct{ nombre, valor string }{
		{"eventId", ev.EventID},
		{"timestamp", ev.Timestamp},
		{"gameId", ev.GameID},
		{"gameVersion", ev.GameVersion},
		{"matchId", ev.MatchID},
		{"type", ev.Type},
	}
	for _, campo := range obligatorios {
		if campo.valor == "" {
			return rechazo(ReasonMissingRequiredField, campo.nombre)
		}
	}

	if _, err := time.Parse(time.RFC3339Nano, ev.Timestamp); err != nil {
		return rechazo(ReasonInvalidTimestamp, ev.Timestamp)
	}

	if !ev.IsMatchEvent() && (ev.PlayerID == nil || *ev.PlayerID == "") {
		return rechazo(ReasonMissingPlayerID, ev.Type)
	}

	return nil
}

// peekEventID intenta rescatar el eventId de un evento que no paso la decodificacion estricta,
// para que el rechazo diga de que evento se trata. Si no se puede, queda vacio.
func peekEventID(raw []byte) string {
	var solo struct {
		EventID string `json:"eventId"`
	}
	_ = json.Unmarshal(raw, &solo)
	return solo.EventID
}
