# ingest-go — módulo de ingestión

> **Responsable: Gabriel.** Al terminar la Fase 0 esto es un esqueleto: compila, pasa `go test` y
> responde `/health`. La ingestión de verdad llega el Día 1.

Recibe los eventos, los valida, los agrupa por partida y, cuando una partida termina, se los manda a
Scala. Es la parte **imperativa y concurrente** del proyecto: estado mutable controlado, goroutines
y channels.

No calcula estadísticas ni aplica reglas. Eso es de Scala.

## Comandos

```bash
go vet ./...
go test ./...
go build ./...
go run .          # levanta en el puerto 8080 (PORT lo cambia)
```

## Qué hay ahora

| Archivo | Qué es |
|---|---|
| `main.go` | servidor, configuración por entorno, `/health` |
| `internal/contract/event.go` | el sobre del evento y el catálogo de razones de rechazo |
| `internal/contract/event_test.go` | pruebas contra los ejemplos de `contracts/examples/` |

Las pruebas leen los JSON de `contracts/examples/` en vez de traer sus propios datos: así, si el
contrato cambia y Go no se actualiza, las pruebas fallan. El contrato es la fuente de verdad.

## Decisiones de diseño

**`Data` es `json.RawMessage`.** Go no interpreta lo específico de cada juego, solo lo transporta.
Agregar un juego nuevo no toca una sola línea de este módulo: por eso el campo queda como JSON crudo
y no como una struct por juego.

**El sobre es cerrado.** `DecodeEvent` usa `DisallowUnknownFields`, así que un campo que no está en
el contrato se rechaza con la razón `unknown_field`. Un campo de más casi siempre es un error de
quien produce los eventos, y preferimos avisarle a aceptarlo en silencio.

**Punteros en `PlayerID` y `Action`.** `*string` distingue ausente o `null` de cadena vacía, que es
justo lo que la validación necesita diferenciar: `playerId` ausente en un `LAP_COMPLETED` es un
rechazo (`missing_player_id`), y ausente en un `MATCH_STARTED` es correcto.

**Solo biblioteca estándar.** `net/http`, `encoding/json` y `sync` alcanzan para todo lo que hace
este módulo. No hay dependencias externas, por eso no hay `go.sum`.

## Lo que falta

- **Día 1:** `POST /events`, validación del sobre y el catálogo de rechazos de
  [`../contracts/rejections.md`](../contracts/rejections.md).
- **Día 2:** una goroutine por partida con un despachador que enruta por `matchId` a través de
  channels, y el estado por partida (jugadores activos, eventos acumulados, si inició / terminó).
- **Día 3:** al llegar `MATCH_FINISHED`, ordenar los eventos por `timestamp` y hacer `POST /analyze`
  a Scala, con un reintento si Scala responde con error.
