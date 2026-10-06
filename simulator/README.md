# Simulador de eventos

> **Responsable: Gabriel.** Pasó a este lado con el intercambio de roles del 3 de octubre de 2026.

Hace de sistema de juegos externo: GameStats no implementa los videojuegos, solo consume lo que esta
fuente produce (enunciado §8). El contrato de lo que genera está en
[`../contracts/event.schema.json`](../contracts/event.schema.json).

## Uso

```bash
python simulate.py                                   # escribe out/ con los JSON
python simulate.py --analyze http://localhost:8081   # manda los lotes directo a Scala
python simulate.py --post http://localhost:8080      # manda los eventos a Go, con retardo
python -m unittest                                   # 19 pruebas
```

Sin argumentos toma `ANALYTICS_URL` e `INGEST_URL` del entorno, que es lo que define el
`docker-compose.yml`, para que el servicio funcione sin pasarle nada.

| Opción | Para qué |
|---|---|
| `--seed N` | semilla del generador; la misma semilla da el mismo flujo (por defecto 7) |
| `--matches N` | partidas al azar, además de las 3 guionadas (por defecto 6) |
| `--disorder N` | pares de eventos adyacentes a intercambiar (por defecto 8) |
| `--out RUTA` | dónde escribir los JSON |
| `--no-files` | no escribir nada, solo enviar |
| `--delay-ms N` | retardo entre eventos con `--post` (por defecto 50) |

## Qué genera

```
out/
  events.json              todos los eventos válidos, en orden de emisión (desordenado a propósito)
  invalid-events.json      un caso por cada razón de ../contracts/rejections.md
  summary.json             qué se generó y qué reglas deberían dispararse
  batches/<matchId>.json   un lote por partida, listo para POST /analyze
```

`out/` está en `.gitignore`: la generación es determinista, así que versionarla sería guardar algo
que el script reproduce en un segundo.

Los `batches/` existen para poder probar el módulo Scala **sin depender de que la ingestión esté
lista**. Es lo que permitió validar todo el análisis mientras `ingest-go/` seguía en esqueleto.

## Decisiones

### Lenguaje: Python 3, solo biblioteca estándar

El simulador no entra en la nota de paradigmas — no es el módulo imperativo ni el funcional — así
que el criterio fue minimizar fricción. El proyecto ya usa Python en el CI para
`contracts/validate.py`, y con `urllib` de la estándar no hace falta ni una dependencia ni un
toolchain nuevo. Scala habría permitido reusar las case classes del análisis, pero mete un segundo
`main` en el build de sbt y complica el `assembly`.

### Determinista

Con la misma semilla sale exactamente el mismo flujo. Sin eso no se puede repetir una demo ni fijar
un resultado esperado en una prueba.

### Partidas guionadas, no solo azar

Que una regla se dispare por casualidad no sirve para mostrarla. Hay tres partidas con los tiempos
escritos a mano:

| Partida | Qué demuestra |
|---|---|
| `m-sim-racing-comeback` | p3 cierra la vuelta 1 en último lugar y gana la carrera: dispara `comeback` |
| `m-sim-racing-penalty` | p1 es el más rápido pero los 5 s de penalización le dan la victoria a p2 |
| `m-sim-combat-streak` | p3 elimina 3 veces en 8 s sin morir: dispara `streak`; además hay dos `revenge` |

La segunda es la que muestra que el ganador sale de **combinar dos tipos de evento distintos**
(`LAP_COMPLETED` + `PENALTY`) y no de uno solo.

### Coherencia de los datos de carreras

La `position` de cada vuelta **se deriva** del tiempo acumulado, no se inventa. Si p1 lleva menos
acumulado que p2 al cerrar la vuelta 2, p1 va delante en esa vuelta. El `timestamp` de cada
`LAP_COMPLETED` es el instante real en que ese jugador cruza la meta, o sea inicio + acumulado; por
eso los eventos de los distintos jugadores quedan intercalados solos.

`test_simulate.py` lo comprueba en **todas** las carreras generadas, no solo en las guionadas.

### Partidas en paralelo

Las ventanas de tiempo de las partidas se solapan, así que los eventos de distintos `matchId` salen
intercalados en el flujo. Es lo que le da trabajo concurrente al módulo de Go, que lo pide el
enunciado §8.

### Desorden acotado

El flujo sale desordenado a propósito, porque Go tiene que ordenar por `timestamp` antes de enviar
el lote. Pero el desorden **nunca mueve un evento `MATCH_*`**: si un `MATCH_STARTED` quedara detrás
de otro evento de su partida, Go lo rechazaría con `match_not_started` con toda la razón, y
estaríamos probando un rechazo inventado por el simulador en vez de un caso real.

## Un bug que encontró este simulador

Al mandarle los lotes generados al analizador aparecieron "rachas" de **una** eliminación. La causa
estaba en `CombatAnalyzer`: en Scala `List(a).sliding(3)` devuelve igual un grupo de un elemento —
`sliding` no exige que la ventana esté completa — así que cualquier jugador con una o dos
eliminaciones producía una racha falsa. Las pruebas del módulo no lo veían porque buscaban la racha
con `find` y la correcta aparecía primero.

Es el argumento a favor de tener datos variados: los ejemplos del contrato tienen pocos jugadores y
no destapaban el caso.
