# ingest-go — módulo de ingestión

> **Responsable: Samuel.** El andamiaje de la Fase 0 lo escribió Gabriel antes del intercambio de
> roles.

Recibe los eventos, los valida, los agrupa por partida y, cuando una partida termina, se los manda a
Scala. Es la parte **imperativa y concurrente** del proyecto: estado mutable controlado, goroutines
y channels.

No calcula estadísticas ni aplica reglas. Eso es de Scala.

## Comandos

```bash
go vet ./...
go test -race ./...
go build ./...
go run .          # levanta en el puerto 8080 (PORT lo cambia; ANALYTICS_URL apunta a Scala)
```

## API

| Ruta | Qué hace |
|---|---|
| `POST /events` | Un evento suelto o un arreglo de eventos. Responde `{accepted, rejected:[{eventId, reason, detail}]}` |
| `GET /metrics` | Métricas de la ingestión (abajo) |
| `GET /health` | Para docker compose y el CI |

`POST /events` responde **200** si se aceptó al menos un evento y **400** si no entró ninguno (por
ejemplo, el cuerpo no es JSON). Es lo que dice [`../contracts/rejections.md`](../contracts/rejections.md).

```bash
curl -X POST --data-binary @evento.json http://localhost:8080/events
curl http://localhost:8080/metrics
```

### `GET /metrics`

```json
{
  "eventsReceived": 151,
  "eventsValid": 142,
  "eventsInvalid": 9,
  "rejectionsByReason": { "duplicate_event": 1, "malformed_json": 0, "...": 0 },
  "activeMatches": 2,
  "batchesSent": 10,
  "batchesFailed": 0
}
```

`rejectionsByReason` lista **todas** las razones del catálogo, también las que están en cero, para
que en la demo se vea qué se vigila aunque todavía no haya pasado. Son métricas de la ingestión, no
del juego: Go sigue sin saber qué es una vuelta.

## Archivos

| Archivo | Qué es |
|---|---|
| `main.go` | borde HTTP: configuración, rutas, JSON de entrada y salida |
| `internal/contract/event.go` | el sobre del evento, su validación y el catálogo de razones de rechazo |
| `internal/ingest/dispatcher.go` | el despachador, la goroutine por partida y su estado |
| `internal/ingest/metrics.go` | contadores de `GET /metrics` |
| `internal/ingest/sender.go` | `POST /analyze` hacia Scala, con un reintento |

Las pruebas leen los JSON de `contracts/examples/` en vez de traer sus propios datos: así, si el
contrato cambia y Go no se actualiza, las pruebas fallan. El contrato es la fuente de verdad.

## Cómo fluye un evento

```
POST /events ──> contract.Parse ──> Dispatcher.route ──channel──> goroutine de la partida
                 (sobre aislado)    (duplicados, partida           (jugadores, desconexiones,
                                     no iniciada / terminada)       eventos acumulados)
                                                                         │ MATCH_FINISHED
                                                                         v
                                                       ordenar por timestamp ──> POST /analyze
```

1. **`contract.Parse`** valida lo que se puede saber mirando el evento solo: JSON bien formado, sin
   campos de más, obligatorios presentes, `timestamp` RFC 3339, `playerId` donde corresponde.
2. **`Dispatcher.route`** mira lo que cruza partidas, bajo un `sync.Mutex`: si el `eventId` ya se
   aceptó, si la partida ya terminó, si existe. Un `MATCH_STARTED` de una partida nueva lanza su
   goroutine.
3. **La goroutine de la partida** recibe el evento por su channel, lo aplica a su estado y contesta
   por un channel de respuesta si lo aceptó. Ahí se detecta `reconnect_without_disconnect`.
4. Con `MATCH_FINISHED` la goroutine cierra la partida, ordena los eventos por `timestamp` y manda el
   lote a Scala. Después termina.

## Decisiones de diseño

**Una goroutine por partida, dueña de su estado.** El estado de cada partida (eventos, jugadores
activos, desconectados) lo toca una sola goroutine, que recibe los eventos por un channel y los
procesa de a uno. Por eso ese estado no necesita lock: la exclusión la da el channel. Las partidas
avanzan en paralelo entre sí.

**Un mutex solo para lo compartido.** El mapa de partidas vivas, las terminadas y los `eventId`
aceptados los consultan todas las peticiones. Ese estado es chico y las operaciones son de un
instante, así que un `sync.Mutex` es lo más simple. Es la mezcla que recomienda Go: channels para
pasar la propiedad de los datos, mutex para proteger un mapa.

**La respuesta es síncrona.** Cada evento viaja con un channel de respuesta y el handler espera a
que la partida conteste, porque `POST /events` tiene que devolver los rechazos con su razón.

**Nadie queda colgado cuando una partida termina.** Cada partida tiene un channel `done` que se
cierra al terminar. Quien intenta enviarle un evento hace `select` entre enviar y `done`: si la
partida terminó en el medio, el evento se rechaza con `match_already_finished` en vez de bloquearse.

**Un rechazo no reserva el `eventId`.** `duplicate_event` significa "ya se aceptó antes". Si un
evento se rechaza por otra razón y se reenvía corregido con el mismo id, entra.

**Se reordena con orden estable.** Dos eventos con el mismo `timestamp` conservan el orden de
llegada.

**Reintento solo cuando tiene sentido.** Si Scala no responde o devuelve un 5xx, se reintenta una
vez tras 500 ms. Un 4xx no se reintenta: Scala ya dijo que el lote es inválido y la respuesta sería
la misma. Si falla igual, el lote queda guardado en memoria y contado en `batchesFailed`: la partida
no se pierde, aunque reenviarla no está automatizado.

**`Data` es `json.RawMessage`.** Go no interpreta lo específico de cada juego, solo lo transporta.
Agregar un juego nuevo no toca una sola línea de este módulo.

**El sobre es cerrado.** `DecodeEvent` usa `DisallowUnknownFields`, así que un campo que no está en
el contrato se rechaza con `unknown_field`.

**Punteros en `PlayerID` y `Action`.** `*string` distingue ausente o `null` de cadena vacía, que es
justo lo que la validación necesita diferenciar.

**Solo biblioteca estándar.** `net/http`, `encoding/json`, `sync` y `slices` alcanzan. No hay
dependencias externas, por eso no hay `go.sum`.

## Limitaciones conocidas

- **Todo en memoria.** Al reiniciar se pierden las partidas vivas y el registro de `eventId`.
- **Partida sin `MATCH_FINISHED`** queda viva para siempre y nunca llega a Scala (fuera de alcance,
  ver `contracts/rejections.md`).
- **Un segundo `MATCH_STARTED`** de una partida que ya está viva se acepta como un evento más: el
  catálogo de rechazos no tiene una razón para eso.
- El conjunto de `eventId` aceptados crece sin límite. Para una demo sobra.
