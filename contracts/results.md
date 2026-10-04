# Formato de resultados

> **Estado: propuesta — pendiente de revisión de Samuel.** Esta es la salida del módulo Scala, así que la decisión final
> es suya. Lo que sigue es un punto de partida concreto para poder discutirlo sobre algo escrito.

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

## Códigos de respuesta

| Endpoint | 200 | 404 |
|---|---|---|
| `GET /results/matches/{id}` | la partida fue analizada | no hay resultados para ese `matchId` |
| `GET /results/players/{id}` | el jugador aparece en al menos una partida analizada | no aparece en ninguna |
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
