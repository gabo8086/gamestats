package main

// CasoInvalido tiene la misma forma que los de contracts/examples/invalid-events.json, para que las
// dos colecciones se puedan usar indistintamente.
//
// ValidAgainstSchema dice si el evento es estructuralmente valido: los que llevan true solo se
// pueden rechazar con el estado de la partida que mantiene Go, no con el esquema. PrecedingEvents
// son los eventos que Go tiene que haber procesado antes para que el rechazo ocurra.
type CasoInvalido struct {
	ExpectedReason     string `json:"expectedReason"`
	Note               string `json:"note"`
	ValidAgainstSchema bool   `json:"validAgainstSchema"`
	PrecedingEvents    []any  `json:"precedingEvents"`
	// Event y RawBody son excluyentes: malformed_json no es representable como objeto, porque
	// justamente no es JSON valido.
	Event   any    `json:"event,omitempty"`
	RawBody string `json:"rawBody,omitempty"`
}

type ColeccionInvalida struct {
	Comment string         `json:"$comment"`
	Cases   []CasoInvalido `json:"cases"`
}

// casosInvalidos genera un caso por cada razon de contracts/rejections.md.
func casosInvalidos(momento string) ColeccionInvalida {
	matchID := "m-sim-invalid"

	// sobre devuelve un evento completo y valido al que cada caso le cambia lo que necesita. Se
	// trabaja con mapas y no con Event porque varios casos tienen que **omitir** un campo
	// obligatorio o agregar uno de mas, y una struct no deja hacer ninguna de las dos.
	sobre := func(cambios map[string]any) map[string]any {
		base := map[string]any{
			"eventId":     "x-000000",
			"timestamp":   momento,
			"gameId":      "racing",
			"gameVersion": GameVersion,
			"matchId":     matchID,
			"playerId":    nil,
			"type":        MatchStarted,
			"action":      nil,
			"data":        nil,
		}
		for k, v := range cambios {
			base[k] = v
		}
		return base
	}

	arranque := sobre(map[string]any{
		"eventId": "x-000100",
		"data":    map[string]any{"matchMode": "PVP", "laps": 3, "track": "Valle Seco"},
	})

	sinCampo := sobre(map[string]any{"eventId": "x-000002"})
	delete(sinCampo, "matchId")

	return ColeccionInvalida{
		Comment: "Generado por el simulador. Un caso por cada razon de contracts/rejections.md. " +
			"'rawBody' aparece solo en malformed_json, que no es representable como objeto.",
		Cases: []CasoInvalido{
			{
				ExpectedReason:  "malformed_json",
				Note:            "el cuerpo se corta a la mitad",
				PrecedingEvents: []any{},
				RawBody:         `{"eventId": "x-000001", "timestamp":`,
			},
			{
				ExpectedReason:  "missing_required_field",
				Note:            "falta matchId",
				PrecedingEvents: []any{},
				Event:           sinCampo,
			},
			{
				ExpectedReason:  "invalid_timestamp",
				Note:            "fecha en formato local, no RFC 3339",
				PrecedingEvents: []any{},
				Event:           sobre(map[string]any{"eventId": "x-000003", "timestamp": "05/10/2026 16:00:00"}),
			},
			{
				ExpectedReason:  "missing_player_id",
				Note:            "LAP_COMPLETED no es MATCH_*, asi que playerId es obligatorio",
				PrecedingEvents: []any{arranque},
				Event: sobre(map[string]any{
					"eventId": "x-000004", "type": LapCompleted,
					"data": map[string]any{"lap": 1, "lapTimeMs": 80000, "position": 1},
				}),
			},
			{
				ExpectedReason:  "unknown_field",
				Note:            "el sobre es cerrado: un campo de mas se rechaza, no se ignora",
				PrecedingEvents: []any{},
				Event:           sobre(map[string]any{"eventId": "x-000005", "serverRegion": "cr-central"}),
			},
			{
				ExpectedReason:     "duplicate_event",
				Note:               "el mismo eventId que el evento anterior",
				ValidAgainstSchema: true,
				PrecedingEvents:    []any{arranque},
				Event: sobre(map[string]any{
					"eventId": "x-000100",
					"data":    map[string]any{"matchMode": "PVP", "laps": 3, "track": "Valle Seco"},
				}),
			},
			{
				ExpectedReason:     "match_not_started",
				Note:               "primer evento de la partida y no es MATCH_STARTED",
				ValidAgainstSchema: true,
				PrecedingEvents:    []any{},
				Event: sobre(map[string]any{
					"eventId": "x-000006", "matchId": "m-sim-sin-inicio",
					"type": PlayerJoined, "playerId": "p1",
					"data": map[string]any{"playerType": Human},
				}),
			},
			{
				ExpectedReason:     "reconnect_without_disconnect",
				Note:               "se reconecta sin haberse desconectado antes",
				ValidAgainstSchema: true,
				PrecedingEvents:    []any{arranque},
				Event: sobre(map[string]any{
					"eventId": "x-000007", "type": PlayerReconnected, "playerId": "p1",
				}),
			},
			{
				ExpectedReason:     "match_already_finished",
				Note:               "llega una vuelta despues del MATCH_FINISHED de esa partida",
				ValidAgainstSchema: true,
				PrecedingEvents: []any{
					arranque,
					sobre(map[string]any{"eventId": "x-000101", "type": MatchFinished}),
				},
				Event: sobre(map[string]any{
					"eventId": "x-000008", "type": LapCompleted, "playerId": "p1",
					"data": map[string]any{"lap": 1, "lapTimeMs": 80000, "position": 1},
				}),
			},
		},
	}
}
