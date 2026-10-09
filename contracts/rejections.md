# Razones de rechazo

Go **nunca descarta un evento en silencio**: todo evento rechazado se registra con su `eventId` (si lo trae) y una de las
razones de esta tabla. El código de razón es parte del contrato: el simulador y las pruebas se apoyan en él.

| Código | Cuándo | Lo detecta |
|---|---|---|
| `malformed_json` | El cuerpo no es JSON válido | parseo |
| `missing_required_field` | Falta o está vacío alguno de `eventId`, `timestamp`, `gameId`, `gameVersion`, `matchId`, `type` | esquema |
| `invalid_timestamp` | `timestamp` presente pero no es RFC 3339 | esquema |
| `missing_player_id` | Evento que no es `MATCH_*` sin `playerId` (ausente, `null` o vacío) | esquema |
| `unknown_field` | El sobre trae un campo que no está en el contrato | esquema |
| `duplicate_event` | `eventId` ya aceptado antes | estado de Go |
| `match_not_started` | Primer evento de un `matchId` que no es `MATCH_STARTED` | estado de Go |
| `match_already_finished` | Evento de un `matchId` que ya recibió `MATCH_FINISHED` | estado de Go |
| `reconnect_without_disconnect` | `PLAYER_RECONNECTED` de un jugador que no estaba desconectado | estado de Go |

`reconnect_without_disconnect` entra con los eventos de desconexión (decisión 15). Es la única razón **opcional** de
la tabla: sin ella una reconexión huérfana simplemente no cuenta para nada, pero rechazarla evita que las
estadísticas de desconexión queden descuadradas sin que nadie se entere. Si Samuel prefiere no llevar ese estado en
Go, se quita de aquí y del ejemplo.

Las razones de la columna "esquema" se pueden comprobar con `contracts/event.schema.json` sobre el evento aislado.
Las de "estado de Go" necesitan el historial de la partida, por eso `contracts/examples/invalid-events.json` marca cada
caso con `validAgainstSchema` y, cuando hace falta, con los `precedingEvents` que dejan la partida en el estado necesario.

## Respuesta de `POST /events`

```json
{
  "accepted": 14,
  "rejected": [
    { "eventId": "e-900001", "reason": "missing_required_field", "detail": "gameVersion" }
  ]
}
```

HTTP 200 cuando se aceptó al menos un evento, 400 cuando el cuerpo entero es inválido (`malformed_json`).
Un lote con eventos mezclados válidos e inválidos responde 200: los válidos se procesan y los inválidos se listan.
`detail` es texto libre para depurar; las pruebas se apoyan en `reason`, no en `detail`.

## Fuera de alcance por ahora

- Partida que nunca recibe `MATCH_FINISHED`: queda viva en memoria y nunca se envía a Scala. Si sobra tiempo, un
  timeout configurable que la cierre y la envíe marcada como incompleta.
- `PLAYER_LEFT` / `PLAYER_DISCONNECTED` / `PLAYER_RECONNECTED`: el enunciado los menciona como posibles; no los
  usamos, y por eso un evento con esos tipos entra como cualquier otro (el sobre es válido) pero ningún analizador
  lo interpreta.
