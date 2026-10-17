# Simulador de eventos

> **Responsable: Gabriel.** Escrito en Go, como fija la decisión 17 de [`../docs/diseno.md`](../docs/diseno.md).

Hace de sistema de juegos externo: GameStats no implementa los videojuegos, solo consume lo que esta
fuente produce (enunciado §8). El contrato de lo que genera está en
[`../contracts/event.schema.json`](../contracts/event.schema.json).

## Uso

```bash
go run .                                   # escribe out/ con los JSON
go run . -analyze http://localhost:8081    # manda los lotes directo a Scala
go run . -post http://localhost:8080       # manda los eventos a Go, con retardo
go test ./...                              # 22 pruebas
```

Dentro de la imagen de Docker se invoca como `gamestats simulator <args>`, y toma `INGEST_URL` y
`ANALYTICS_URL` del entorno, así que ahí funciona sin pasarle nada.

| Opción | Para qué |
|---|---|
| `-seed N` | semilla del generador; la misma semilla da el mismo flujo (por defecto 7) |
| `-matches N` | partidas al azar, además de las 6 guionadas (por defecto 6) |
| `-disorder N` | pares de eventos adyacentes a intercambiar (por defecto 8) |
| `-out RUTA` | dónde escribir los JSON |
| `-no-files` | no escribir nada, solo enviar |
| `-delay-ms N` | retardo entre eventos con `-post` (por defecto 50) |

## Qué genera

```
out/
  events.json              todos los eventos válidos, en orden de emisión (desordenado a propósito)
  invalid-events.json      un caso por cada razón de ../contracts/rejections.md
  summary.json             qué se generó y qué reglas deberían dispararse
  batches/<matchId>.json   un lote por partida, listo para POST /analyze
```

Con los valores por defecto son **12 partidas y 234 eventos**, y aparecen los quince tipos de evento
del contrato. `out/` está en `.gitignore`: la generación es determinista, así que versionarla sería
guardar algo que el programa reproduce en un segundo.

Los `batches/` existen para poder probar el módulo Scala **sin depender de la ingestión**.

## Por qué en Go

El simulador no entra en la nota de paradigmas, pero uno **concurrente** en Go refuerza el paradigma
imperativo del proyecto y evita sumar un tercer lenguaje al despliegue. Antes estaba en Python; la
decisión 17 lo cambió cuando el requisito pasó a cuatro juegos.

Usa solo la biblioteca estándar: `encoding/json`, `net/http`, `math/rand`, `sync`.

## Concurrencia y determinismo a la vez

**Cada partida se genera en su propia goroutine.** No es decorativo: es el mismo modelo que usa el
módulo de ingestión para procesarlas, y aquí se ve que varias partidas avanzan a la vez.

El determinismo se mantiene con dos decisiones:

- Cada partida recibe **su propio generador de azar**, sembrado a partir de la semilla global y de
  su índice. Dos partidas no comparten estado, así que no hace falta ningún candado.
- Los `eventId` **se reparten después**, cuando ya se ordenaron las partidas. Si se asignaran dentro
  de las goroutines dependerían de cuál termina primero y el flujo dejaría de ser reproducible.

`go test -race` corre limpio, y hay una prueba que genera el mismo flujo dos veces y lo compara
evento por evento.

## Partidas guionadas

Que una regla se dispare por casualidad no sirve para mostrarla. Seis partidas tienen los datos
escritos a mano:

| Partida | Qué demuestra |
|---|---|
| `m-sim-racing-comeback` | p3 cierra la vuelta 1 último y gana: dispara `comeback` |
| `m-sim-racing-penalty` | p1 es el más rápido pero los 5 s de penalización le dan la victoria a p2 |
| `m-sim-combat-streak` | p3 elimina 3 veces en 8 s sin morir: `streak`, más dos `revenge` y una reconexión |
| `m-sim-combat-abandono` | p5 se desconecta y no vuelve: abandono |
| `m-sim-blackjack-tilt` | tres derrotas seguidas y la apuesta sube de 10 a 30: `tilt` |
| `m-sim-battleship-streak` | p1 encadena cinco aciertos y hunde la flota: `hitStreak` |

La segunda muestra que el ganador sale de **combinar dos tipos de evento distintos** y no de uno
solo. Las dos de combate cubren los dos lados de la desconexión, que el análisis distingue.

## Coherencia de los datos

Dos juegos tienen datos que saldrían contradictorios si se generaran al azar sin cuidado:

- **Carreras**: la `position` de cada vuelta **se deriva** del tiempo acumulado, nunca se inventa.
  El `timestamp` de cada `LAP_COMPLETED` es el instante real en que ese jugador cruza la meta, o sea
  inicio + acumulado; por eso los eventos de los distintos jugadores quedan intercalados solos.
- **Blackjack**: el `outcome` de cada ronda **se deduce** comparando las manos, con la misma regla
  del as que aplica el analizador. Si alguien cambia una carta, el resultado cambia solo y no queda
  inconsistente.

Las dos propiedades se comprueban en **todas** las partidas generadas, no solo en las guionadas.

## Desorden acotado

El flujo sale desordenado a propósito, porque la ingestión tiene que ordenar por `timestamp` antes
de enviar el lote. Pero el desorden **nunca mueve un evento `MATCH_*`**: si un `MATCH_STARTED`
quedara detrás de otro evento de su partida, la ingestión lo rechazaría con `match_not_started` con
toda la razón, y estaríamos probando un rechazo inventado por el simulador en vez de un caso real.
