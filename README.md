# GameStats

Plataforma que recibe eventos de partidas de videojuegos (simulados), los valida y organiza, y
calcula estadísticas y detecta patrones. Proyecto 1 del curso **Lenguajes de Programación** (ITCR).

Dos módulos, dos paradigmas:

```
simulador ──JSON──> [ Go: ingestión ] ──HTTP/JSON──> [ Scala: análisis ] ──> resultados JSON
 (eventos)          valida, agrupa por partida,       estadísticas + reglas    GET /results/...
                    concurrencia, estado              (puerto 8081)
                    (puerto 8080)
```

- **Go** (`ingest-go/`) — imperativo y concurrente: ingestión, validación, estado mutable por
  partida, una goroutine por partida con channels.
- **Scala** (`analytics-scala/`) — funcional: datos inmutables, `filter` / `map` / `groupBy` /
  `fold`, funciones puras.

La división no es casual: el enunciado (§6) fija qué responsabilidades van en cada lenguaje, y el
objetivo académico (§13) es poder justificar por qué cada parte encaja con su paradigma.

## Estado

**Fase 0 (diseño y andamiaje) — en revisión.** Los dos módulos compilan y pasan sus pruebas, pero
todavía no hacen el trabajo real: Go responde `/health` y Scala también. Las decisiones de diseño
están en la tabla de [`CLAUDE.md`](CLAUDE.md) y **ninguna está aprobada todavía**: Samuel tiene que
revisarlas antes de que empiece el Día 1.

## Estructura

| Carpeta | Qué es | Responsable |
|---|---|---|
| [`contracts/`](contracts/) | formato de eventos, lote y resultados: la fuente de verdad | ambos |
| [`ingest-go/`](ingest-go/) | módulo de ingestión (Go) | Gabriel |
| [`analytics-scala/`](analytics-scala/) | módulo de análisis (Scala) | Samuel |
| [`simulator/`](simulator/) | fuente de eventos simulada | Samuel |
| [`docs/`](docs/) | enunciado del curso | ambos |

Cambios en `contracts/` requieren aprobación de los dos: todo lo demás depende de esos archivos.

## Requisitos

| Herramienta | Versión | Para qué |
|---|---|---|
| Go | 1.24+ | módulo de ingestión |
| JDK | 21 | módulo de análisis |
| sbt | 1.13.0 | build de Scala |
| Docker + Compose | reciente | levantar todo integrado |
| Python + `jsonschema` | 3.12+ | validar los ejemplos del contrato |

Para correr solo con Docker basta Docker: las versiones de Go, JDK y sbt están fijadas en los
Dockerfiles.

## Ejecución

### Todo integrado

```bash
docker compose up --build
curl http://localhost:8080/health    # ingestión (Go)
curl http://localhost:8081/health    # análisis (Scala)
```

Dentro de Compose los servicios se llaman por nombre (`http://analytics:8081`), nunca `localhost`:
cada contenedor tiene su propio `localhost`.

### Por módulo

```bash
cd ingest-go       && go vet ./... && go test ./... && go build ./...
cd analytics-scala && sbt test
```

### Validar el contrato

```bash
pip install jsonschema
python contracts/validate.py
```

Comprueba que los ejemplos cumplen los esquemas y las reglas que JSON Schema no puede expresar
(`eventId` único, eventos ordenados por `timestamp`, un caso por cada razón de rechazo).

## Juegos

Dos juegos de forma deliberadamente distinta, para demostrar que el núcleo es genérico: agregar un
juego significa agregar un analizador en Scala, sin tocar Go ni el sobre del evento.

- **Carreras** (`racing`) — vueltas, tiempos, posiciones y penalizaciones. Regla de secuencia:
  **remontada** (un jugador estuvo último en alguna vuelta y terminó ganando).
- **Combate** (`combat`) — eliminaciones, armas y daño. Regla de secuencia: **racha** (3
  eliminaciones en ≤10 s sin morir en medio).

El detalle de cada estadística y de cada regla está en [`CLAUDE.md`](CLAUDE.md), y la forma exacta de
la salida en [`contracts/results.md`](contracts/results.md).

## Documentación

| Archivo | Qué contiene |
|---|---|
| [`CLAUDE.md`](CLAUDE.md) | decisiones de diseño, definiciones precisas, reparto de trabajo |
| [`contracts/event.schema.json`](contracts/event.schema.json) | el sobre común del evento |
| [`contracts/batch.schema.json`](contracts/batch.schema.json) | el lote que Go envía a Scala |
| [`contracts/results.md`](contracts/results.md) | forma del JSON de resultados |
| [`contracts/rejections.md`](contracts/rejections.md) | razones de rechazo de eventos |
| [`docs/enunciado.md`](docs/enunciado.md) | enunciado del curso (el `.docx` original está al lado) |

## Entrega

El enunciado §14 dice *"domingo 18 de Setiembre de 2026"*, fecha que ya pasó: es un error de tipeo
en el documento. **Confirmar la fecha real con el profesor.**
