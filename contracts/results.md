# Formato de resultados

> **Estado: implementado para `racing` y `combat`; propuesta para `blackjack`, `battleship` y las estadísticas de
> plataforma.** Con el reparto vigente el módulo Scala es de Gabriel, así que la forma la decide él, pero esto vive en
> `contracts/` y los cambios necesitan el acuerdo de los dos. Lo nuevo son las decisiones 12 a 15 del `CLAUDE.md`.

## Principio de diseño

El sobre de resultados es **igual para todos los juegos**; lo propio de cada juego va dentro de `gameSpecific`.
Es la misma idea que `data` en el evento: agregar un juego nuevo significa agregar un `GameAnalyzer` que llene
`gameSpecific`, sin tocar el sobre ni los endpoints. El enunciado (§12) pide justamente que un conjunto nuevo de
eventos se pueda incorporar sin reescribir el sistema.

## `GET /results/matches/{matchId}`

```json
{
  "matchId": "m-racing-001",
  "gameId": "racing",
  "gameVersion": "1.0",
  "analyzedAt": "2026-10-05T14:04:21.100Z",
  "eventCount": 15,
  "match": {
    "startedAt": "2026-10-05T14:00:00.000Z",
    "finishedAt": "2026-10-05T14:04:20.000Z",
    "durationMs": 260000,
    "playerCount": 3,
    "winnerId": "p3",
    "gameSpecific": {
      "track": "Circuito Volcan",
      "laps": 3,
      "ranking": [
        { "playerId": "p3", "adjustedTimeMs": 238000, "penaltyMs": 0 },
        { "playerId": "p1", "adjustedTimeMs": 250000, "penaltyMs": 0 },
        { "playerId": "p2", "adjustedTimeMs": 252000, "penaltyMs": 6000 }
      ]
    }
  },
  "players": [
    {
      "playerId": "p3",
      "gameSpecific": {
        "bestLapMs": 76000,
        "avgLapMs": 79333.33,
        "consistencyMs": 4027.68,
        "overtakes": 1,
        "positions": [3, 1, 1],
        "adjustedTimeMs": 238000
      }
    }
  ],
  "patterns": [
    {
      "rule": "comeback",
      "playerId": "p3",
      "detail": { "worstPosition": 3, "worstPositionLap": 1, "finalPosition": 1 }
    }
  ]
}
```

`players` lleva una entrada por jugador (el ejemplo muestra una sola por brevedad). `patterns` va vacío si no se
detectó ningún patrón: es una lista, no un booleano, porque un mismo jugador puede disparar varias reglas y porque
agregar una regla no cambia la forma del JSON.

### Lo mismo para `combat`

```json
{
  "matchId": "m-combat-001",
  "gameId": "combat",
  "gameVersion": "1.0",
  "eventCount": 12,
  "match": {
    "durationMs": 130000,
    "playerCount": 4,
    "winnerId": "p2",
    "gameSpecific": {
      "mode": "deathmatch",
      "deadliestPlayerId": "p2",
      "byWeapon": [
        { "weapon": "rifle",   "eliminations": 3, "avgDamage": 116.67 },
        { "weapon": "shotgun", "eliminations": 2, "avgDamage": 102.5 },
        { "weapon": "pistol",  "eliminations": 1, "avgDamage": 80.0 }
      ]
    }
  },
  "players": [
    {
      "playerId": "p2",
      "gameSpecific": {
        "eliminations": 3,
        "deaths": 1,
        "kd": 3.0,
        "kdUndefined": false,
        "avgDamage": 111.67
      }
    },
    {
      "playerId": "p1",
      "gameSpecific": {
        "eliminations": 2,
        "deaths": 0,
        "kd": 2.0,
        "kdUndefined": true,
        "avgDamage": 110.0
      }
    }
  ],
  "patterns": [
    {
      "rule": "streak",
      "playerId": "p2",
      "detail": {
        "eliminations": 3,
        "windowMs": 8000,
        "eventIds": ["e-100007", "e-100008", "e-100009"]
      }
    },
    {
      "rule": "revenge",
      "playerId": "p3",
      "detail": { "againstPlayerId": "p2", "eliminatedByEventId": "e-100008", "revengeEventId": "e-100010" }
    }
  ]
}
```

En `racing`, `winnerId` es el del menor tiempo ajustado; en `combat`, el de más eliminaciones. Es el analizador de
cada juego el que decide qué significa "ganar".

### `blackjack`

```json
{
  "matchId": "m-blackjack-001",
  "gameId": "blackjack",
  "match": {
    "durationMs": 215000,
    "playerCount": 2,
    "winnerId": "p1",
    "gameSpecific": {
      "rounds": 6,
      "dealerBalance": -45
    }
  },
  "players": [
    {
      "playerId": "p1",
      "gameSpecific": {
        "roundsPlayed": 6,
        "winRate": 33.33,
        "netBalance": 45,
        "avgBet": 20.0,
        "bustRate": 16.67
      }
    }
  ],
  "patterns": [
    {
      "rule": "tilt",
      "playerId": "p1",
      "detail": { "lossStreak": 3, "betBefore": 10, "betAfter": 30, "atRound": 4 }
    }
  ]
}
```

`dealerBalance` es el negativo de la suma de los `payout`: lo que gana el jugador lo pierde la casa.
`winnerId` es el jugador con saldo neto positivo, o `null` si nadie terminó en verde.

**El valor de cada mano no aparece en los resultados ni en los eventos.** Scala lo reconstruye con un `foldLeft` sobre
los `CARD_DEALT` de cada ronda, aplicando la regla del as: vale 11 salvo que pase de 21, y ahí vale 1. Por eso el
acumulador del fold lleva el total **y** la cantidad de ases, no solo un número.

### `battleship`

```json
{
  "matchId": "m-battleship-001",
  "gameId": "battleship",
  "match": {
    "durationMs": 55000,
    "playerCount": 2,
    "winnerId": "p1",
    "gameSpecific": {
      "boardSize": 10,
      "shotsToWin": 6,
      "avgShotsPerShipSunk": 3.0
    }
  },
  "players": [
    {
      "playerId": "p1",
      "gameSpecific": {
        "shots": 6,
        "hits": 5,
        "accuracy": 83.33,
        "shipsSunk": 2
      }
    }
  ],
  "patterns": [
    {
      "rule": "hitStreak",
      "playerId": "p1",
      "detail": { "hits": 5, "fromCoord": "B2", "toCoord": "C7" }
    }
  ]
}
```

`shipsSunk` de un jugador se cuenta sobre los `SHIP_SUNK` **de su rival**, porque el `playerId` de ese evento es el dueño
del barco hundido. Es la misma convención que las muertes en `combat`, a propósito: una sola regla que recordar.

`shotsToWin` y `avgShotsPerShipSunk` son del ganador.

## Estadísticas de plataforma

Las calcula el `GenericAnalyzer`, que **usa solo el sobre común** y no conoce ningún juego. Van en el bloque `generic`
de todo resultado de partida, sea del juego que sea:

```json
{
  "generic": {
    "eventsByType": { "MATCH_STARTED": 1, "PLAYER_JOINED": 4, "PLAYER_ELIMINATED": 6, "MATCH_FINISHED": 1 },
    "activityPerMinute": [ { "minute": 0, "events": 7 }, { "minute": 1, "events": 4 }, { "minute": 2, "events": 3 } ],
    "playersSeen": 4
  }
}
```

`durationMs` y `eventCount` ya viven en el sobre del resultado y también los produce este analizador; no se repiten aquí.

`activityPerMinute` agrupa los eventos por minuto transcurrido desde el `MATCH_STARTED`. Un minuto sin eventos aparece
con `events: 0`, para que la serie no tenga huecos y se pueda graficar directo.

Que esto funcione sin saber de qué juego se trata es la demostración concreta de que el sobre común alcanza: un juego
nuevo trae estas métricas gratis, sin escribir una línea.

## `GET /results/games/{gameId}/ranking`

Los mejores jugadores de un juego. Acepta `?top=N`, por defecto 10.

```json
{
  "gameId": "combat",
  "top": 10,
  "ranking": [
    { "rank": 1, "playerId": "p2", "matchesPlayed": 3, "wins": 2, "winRate": 66.67 },
    { "rank": 2, "playerId": "p1", "matchesPlayed": 3, "wins": 1, "winRate": 33.33 }
  ]
}
```

Se ordena por victorias y se desempata por `winRate`. Un jugador sin partidas en ese juego no aparece.

## `GET /results/players/{playerId}`

Agregado del jugador a través de las partidas ya analizadas.

```json
{
  "playerId": "p2",
  "matchesPlayed": 1,
  "wins": 1,
  "byGame": [
    {
      "gameId": "combat",
      "matchesPlayed": 1,
      "gameSpecific": { "eliminations": 3, "deaths": 1, "kd": 3.0, "avgDamage": 111.67 }
    }
  ],
  "matches": [
    { "matchId": "m-combat-001", "gameId": "combat", "winner": true }
  ]
}
```

`byGame` existe porque no tiene sentido sumar vueltas de carreras con eliminaciones de combate: lo agregado solo se
puede comparar dentro del mismo juego.

### Perfil global

Al mismo documento se le agregan tres bloques que **no dependen del juego** y por eso se calculan igual para los cuatro:

```json
{
  "winRate": 50.0,
  "vsBots":   { "matchesPlayed": 2, "wins": 2, "winRate": 100.0 },
  "vsHumans": { "matchesPlayed": 2, "wins": 0, "winRate": 0.0 },
  "disconnections": { "reconnections": 1, "abandons": 0 }
}
```

- `winRate` es sobre todas las partidas analizadas del jugador, sin separar por juego.
- `vsBots` / `vsHumans` parten las partidas según haya o no algún participante con `playerType: "BOT"`, que es lo que
  `PLAYER_JOINED` declara. Una partida con bots cuenta en `vsBots` aunque también haya humanos.
- `disconnections` cuenta los `PLAYER_DISCONNECTED` del jugador: **reconexión** si vuelve en ≤ 60 s, **abandono** si
  nunca aparece el `PLAYER_RECONNECTED`. La ventana es la decisión 15 y está propuesta, no cerrada.

Los tres salen del sobre y de `playerType`, sin tocar el `data` de ningún juego. Es la misma idea que el
`GenericAnalyzer`: lo que es común se calcula una vez y sirve para todos.

## Códigos de respuesta

| Endpoint | 200 | 404 |
|---|---|---|
| `GET /results/matches/{id}` | la partida fue analizada | no hay resultados para ese `matchId` |
| `GET /results/players/{id}` | el jugador aparece en al menos una partida analizada | no aparece en ninguna |
| `GET /results/games/{gameId}/ranking` | el juego tiene al menos una partida analizada | no hay partidas de ese juego |
| `POST /analyze` | 200 con el resultado de la partida | — (400 si el lote no cumple `batch.schema.json`) |

## Convenciones numéricas

Cerrarlas ahora evita que las pruebas de integración fallen por decimales.

- Los tiempos son enteros en milisegundos (`...Ms`).
- Los promedios, desviaciones y razones se **redondean a 2 decimales** en la salida JSON. El cálculo interno usa
  `Double` sin redondear; el redondeo es solo de presentación.
- **Consistencia** = desviación estándar **poblacional** (dividir entre `n`, no entre `n-1`). Con una sola vuelta
  da `0.0`.
- `avgDamage` de un jugador sin eliminaciones es `null`, no `0.0`: no hay muestras, que es distinto de un promedio cero.
- `kd` con 0 muertes devuelve el número de eliminaciones y marca `kdUndefined: true`, para que quien consuma el JSON
  no confunda "3 eliminaciones sin morir" con "K/D de 3.0".
