# GameStats — Contexto del proyecto

Proyecto 1 del curso **Lenguajes de Programación**. Grupo de 2: **Gabriel** y **Samuel**.
El enunciado completo está en `docs/enunciado.md`. Si algo de este archivo contradice el enunciado, manda el enunciado.

## Objetivo y filosofía

GameStats recibe eventos de partidas de videojuegos (simulados), los valida y organiza, y calcula estadísticas y detecta patrones.

**Prioridad absoluta: simplicidad.** No buscamos un sistema grande. Buscamos aplicar bien los dos paradigmas del curso y poder justificarlos en la defensa:
- **Go = imperativo/concurrente**: ingestión, validación, estado mutable por partida, goroutines y channels.
- **Scala = funcional**: datos inmutables, `filter`, `map`, `flatMap`, `groupBy`, `fold`, `reduce`, `collect`, composición de funciones.

No agregues frameworks, capas, patrones ni dependencias que no hagan falta. Ante la duda, la opción más simple.

## Reparto de trabajo

| Persona | Responsable de | Carpetas |
|---|---|---|
| **Gabriel** | Módulo Scala, simulador, datos de prueba y pruebas | `analytics-scala/`, `simulator/` |
| **Samuel** | Módulo Go, Docker, docker-compose, CI/CD, deployment | `ingest-go/`, `docker-compose.yml`, `.github/` |
| **Ambos** | Contrato, documentación, integración y demo | `contracts/`, `docs/` |

Los roles se intercambiaron el 3 de octubre de 2026, a pedido de Samuel: antes Gabriel llevaba Go y Samuel Scala. El andamiaje de `ingest-go/` que entra en el PR de la Fase 0 lo escribió Gabriel **antes** del intercambio; a partir del merge ese módulo es de Samuel.

Cada quien trabaja en sus carpetas para evitar conflictos de merge. Cambios en `contracts/` requieren aprobación de ambos.
Claude Code: si te piden algo fuera de las carpetas de quien te habla, avisa antes de tocarlo.

## Arquitectura

```
simulador ──JSON──> [ Go: ingestión ] ──HTTP/JSON──> [ Scala: análisis ] ──> resultados JSON
 (eventos)          valida, agrupa por partida,       estadísticas + reglas    GET /results/...
                    concurrencia, estado              puerto 8081
                    puerto 8080
```

- Comunicación entre módulos: **HTTP/REST con JSON** (decisión propuesta; confirmar en Fase 0 y documentar el porqué).
- Go y Scala son servicios independientes, cada uno con su Dockerfile, orquestados con `docker-compose`.
- Dentro de Docker Compose los servicios se llaman por nombre (`http://analytics:8081`), nunca `localhost`.

### Go (`ingest-go/`) — qué hace y qué NO hace
Hace:
1. Recibir eventos (endpoint `POST /events` y/o carga de archivo JSON del simulador).
2. Validar el sobre común del evento. Los inválidos se registran y se rechazan con la razón (no se descartan en silencio).
3. Mantener estado por partida (jugadores activos, eventos acumulados, si inició/terminó).
4. Procesar partidas/juegos en paralelo: **una goroutine por partida** que recibe sus eventos por un channel; un despachador enruta por `matchId`.
5. Cuando una partida está lista (`MATCH_FINISHED`), enviarla a Scala como lote.

NO hace: estadísticas ni reglas. Eso es de Scala.
Preferir la biblioteca estándar (`net/http`, `encoding/json`, `sync`). Tratar `data` como JSON opaco (`json.RawMessage`): Go no debe conocer los detalles de cada juego.

### Scala (`analytics-scala/`) — qué hace
1. Recibir el lote de una partida y convertirlo a case classes inmutables.
2. Elegir un analizador según `gameId` (`trait GameAnalyzer`, un objeto por juego: `RacingAnalyzer`, `CombatAnalyzer`). Agregar un juego nuevo = agregar un analizador, sin tocar el resto.
3. Calcular estadísticas y reglas con operaciones funcionales.
4. Producir resultados JSON y exponerlos (`GET /results/matches/{matchId}`, `GET /results/players/{playerId}`; ajustar en Fase 0).

Reglas de estilo Scala: sin `var`, sin colecciones mutables, sin `null`, `sealed trait` + case classes, funciones puras para el análisis y el IO (HTTP, JSON) aislado en los bordes. Usar `Option`/`Either` en vez de excepciones.

## Contrato de eventos (`contracts/`)

Fuente de verdad del formato. Todo evento comparte este sobre; lo específico del juego va en `data`.

```json
{
  "eventId": "e-000123",
  "timestamp": "2026-10-05T14:03:22.500Z",
  "gameId": "racing",
  "gameVersion": "1.0",
  "matchId": "m-racing-001",
  "playerId": "p1",
  "type": "LAP_COMPLETED",
  "action": null,
  "data": { "lap": 2, "lapTimeMs": 81230, "position": 3 }
}
```

Validación mínima en Go: `eventId`, `timestamp` (RFC 3339), `gameId`, `gameVersion`, `matchId` y `type` obligatorios y no vacíos. `playerId` obligatorio salvo en eventos `MATCH_*`. `eventId` único.

El sobre es **cerrado**: un campo que no esté en el contrato se rechaza (`unknown_field`). El catálogo completo de razones de rechazo está en `contracts/rejections.md`.

Eventos generales (todos los juegos): `MATCH_STARTED`, `PLAYER_JOINED`, `MATCH_FINISHED`.

## Juegos elegidos (propuesta; confirmar en Fase 0)

Elegimos dos juegos con forma distinta para demostrar que el sistema es genérico.

### Carreras (`gameId: "racing"`)
- Eventos: `MATCH_STARTED` (`{laps, track}`), `PLAYER_JOINED`, `LAP_COMPLETED` (`{lap, lapTimeMs, position}`), `PENALTY` (`{seconds, reason}`), `MATCH_FINISHED`.
- Por jugador: mejor vuelta, tiempo promedio por vuelta, consistencia (desviación estándar).
- Por partida: ganador y duración total.
- Combinando tipos de evento: **tiempo ajustado** = suma de `LAP_COMPLETED` + penalizaciones `PENALTY`; define el ganador.
- Regla de secuencia: **remontada** — un jugador estuvo en las últimas posiciones en alguna vuelta y terminó ganando (fold sobre su serie de posiciones).
- Adelantamientos: posición que mejora entre vueltas consecutivas.

### Combate (`gameId: "combat"`)
- Eventos: `MATCH_STARTED`, `PLAYER_JOINED`, `PLAYER_ELIMINATED` (`playerId` = quien elimina; `data`: `{victimId, weapon, damage}`), `MATCH_FINISHED`.
- Por jugador: eliminaciones, muertes, K/D, daño promedio.
- Por partida: jugador más letal y duración.
- Combinando eventos: eliminaciones por arma / daño promedio por arma.
- Regla de secuencia: **racha** — 3 eliminaciones en ≤10 s sin morir entre medio. (Opcional: **venganza** — A elimina a B y luego B elimina a A.)

## Simulador y datos de prueba (`simulator/`)

Simula al sistema de juegos externo. Debe generar: varias partidas, varios jugadores, partidas de ambos juegos **en paralelo**, casos normales y casos armados a mano que disparen cada regla (una remontada, una racha), y algunos eventos inválidos para probar la validación. Salida: archivos JSON y/o envío a `POST /events`. Los datos de racing deben ser coherentes (posiciones derivadas de tiempos acumulados).

## Fase 0 — Diseño conjunto (antes de programar)

Gabriel prepara la Fase 0 con Claude Code y la entrega a Samuel como **un único PR** (contratos, ejemplos, módulos vacíos, CI y esta tabla). **Samuel lo revisa y lo aprueba antes de que empiece el Día 1.** Nadie programa módulos hasta que ese PR esté mergeado, porque todo lo demás depende de estas decisiones.

En esta fase Claude Code **ayuda a redactar y a señalar huecos o contradicciones, pero no decide por ellos**. El andamiaje lo redactó Gabriel; con el intercambio de roles, las decisiones que afectan al módulo Go, a Docker o al CI (cuándo se envía una partida, casos límite, puertos, deployment) quedan **pendientes de revisión de Samuel**, y las del contrato las aprueban los dos.

Lo propuesto en este archivo es el punto de partida. Marcar cada decisión como ✅ cuando quien corresponda la haya aprobado en el PR.

### Decisiones a cerrar

Leyenda de "Aprueba": **A** = ambos, porque toca el contrato compartido o el reparto; **G** = la decide Gabriel (Scala, análisis, simulador, pruebas); **S** = la decide Samuel (Go, Docker, CI, deployment). Quien no decide se da por enterado.

| # | Decisión | Propuesta por defecto | Aprueba | Estado |
|---|---|---|---|---|
| 1 | Juegos | Carreras + combate | A | ⬜ |
| 2 | Tipos de evento y campos de `data` por juego | Los de las secciones de arriba, ya escritos en `contracts/event.schema.json` y en los ejemplos | A | ⬜ |
| 3 | Definición exacta de cada estadística y regla | Ver "Definiciones precisas" abajo | G | ⬜ |
| 4 | Comunicación Go→Scala | HTTP/JSON: Go hace `POST /analyze` a Scala con el lote de una partida (`contracts/batch.schema.json`) | A | ⬜ |
| 5 | Cuándo envía Go una partida | Al recibir `MATCH_FINISHED`, con los eventos ordenados por `timestamp` | S | ⬜ |
| 6 | Casos límite en Go | Ver "Casos límite" abajo y `contracts/rejections.md` | S | ⬜ |
| 7 | Formato y endpoints de resultados | `GET /results/matches/{id}` y `GET /results/players/{id}`; forma exacta en `contracts/results.md` | G | ⬜ |
| 8 | Puertos | Go `8080`, Scala `8081` | S | ⬜ |
| 9 | Versiones y bibliotecas | Go 1.24, solo biblioteca estándar. Scala 3.3.8 (LTS), sbt 1.13.0, MUnit 1.3.6, Java 21. HTTP y JSON de Scala **sin decidir**: candidatos y propuesta en `analytics-scala/README.md` | A | ⬜ |
| 10 | Repo, CI y deployment | GitHub, GitHub Actions, GHCR; entorno de deployment por elegir | S | ⬜ |
| 11 | Reparto y ritmo | Tabla de "Reparto de trabajo" y cronograma de 4 días | A | ⬜ |

Dos cosas quedaron decididas al armar el andamiaje y conviene dejarlas explícitas. Con el reparto nuevo las dos caen del lado de Gabriel:

- **El sobre del evento es cerrado** (`additionalProperties: false`, y `DisallowUnknownFields` en Go). Un campo de más se rechaza con `unknown_field` en vez de ignorarse. Afecta al simulador: no puede mandar campos extra "por si acaso".
- **`sbt-assembly`** está en `analytics-scala/project/plugins.sbt`. Es solo para producir el jar de la imagen de Docker; no condiciona la biblioteca de HTTP ni la de JSON.

### Definiciones precisas (ajustar y cerrar)

- **Remontada (racing):** jugador que en alguna vuelta estuvo en la última posición (`position == número de jugadores`) y terminó ganando.
- **Ganador (racing):** menor tiempo ajustado = suma de `lapTimeMs` + `penalty.seconds × 1000`.
- **Consistencia (racing):** desviación estándar **poblacional** (dividir entre `n`) de los `lapTimeMs` del jugador.
- **Adelantamiento (racing):** vuelta en la que `position` mejora respecto a la vuelta anterior del mismo jugador.
- **Racha (combat):** 3 `PLAYER_ELIMINATED` del mismo `playerId` en una ventana de ≤10 s, sin que ese jugador aparezca como `victimId` en medio.
- **K/D (combat):** eliminaciones / muertes; si las muertes son 0, devolver las eliminaciones y marcar el caso (`kdUndefined: true`).
- **Jugador más letal (combat):** el que tiene más eliminaciones; desempate por daño promedio.

Las convenciones numéricas (redondeo, promedios sin muestras, tiempos en milisegundos) están en `contracts/results.md`.

### Casos límite (propuesta)

- Evento con `matchId` sin `MATCH_STARTED` previo → rechazar con razón `match_not_started`.
- Evento posterior a `MATCH_FINISHED` de esa partida → rechazar con razón `match_already_finished`.
- `eventId` repetido → rechazar con razón `duplicate_event`.
- Campo fuera del contrato → rechazar con razón `unknown_field`.
- Eventos fuera de orden → aceptarlos y ordenar por `timestamp` antes de enviar a Scala.
- Partida que nunca recibe `MATCH_FINISHED` → fuera de alcance al principio; opcional: timeout.
- Respuesta de error de Scala → Go registra el error y reintenta una vez; no pierde la partida.

### Qué debe quedar en el repo al terminar la Fase 0

```
contracts/
  event.schema.json        # sobre común del evento
  batch.schema.json        # cuerpo del POST /analyze: {matchId, gameId, gameVersion, events:[...]}
  results.md               # forma exacta del JSON de resultados por jugador y por partida
  rejections.md            # catálogo de razones de rechazo
  validate.py              # comprueba que los ejemplos cumplen los esquemas (corre en CI)
  examples/
    racing-match-ok.json   # partida completa de carreras (~10 eventos mínimo)
    combat-match-ok.json   # partida completa de combate
    invalid-events.json    # al menos un caso por cada razón de rechazo
ingest-go/                 # proyecto Go vacío que compila y pasa `go test`
analytics-scala/           # proyecto sbt vacío que compila y pasa `sbt test`
simulator/                 # carpeta vacía con README
docker-compose.yml         # servicios declarados aunque sean mínimos
.github/workflows/ci.yml   # compila ambos módulos en cada PR
docs/enunciado.md
CLAUDE.md                  # este archivo con la tabla de decisiones en ✅
```

Además: `main` protegida (PR obligatorio, los cuatro checks del CI en verde, sin force push) y cada quien con acceso de escritura al repo.

### Definición de "Fase 0 terminada"

- Todas las decisiones de la tabla en ✅ y este archivo actualizado.
- Los ejemplos de `contracts/examples/` validan contra los esquemas.
- Ambos módulos vacíos compilan y el CI pasa en un PR de prueba.
- Cada uno puede arrancar su parte sin necesitar nada del otro hasta el Día 3.

## Flujo de git

- `main` está protegida: nada de push directo. Todo cambio entra por **PR** desde una rama (`feat/...`, `fix/...`, `docs/...`).
- Un PR = un cambio pequeño y revisable. Commits en español o inglés, claros y en imperativo.
- Antes de abrir el PR: `git fetch && git merge origin/main` en tu rama, luego `docker compose up --build` y la prueba de humo, para validar el resultado de la mezcla.
- El PR requiere los cuatro checks del CI en verde. **La aprobación del otro no es obligatoria para mergear**: la regla de rama no la exige, para que ninguno quede bloqueado esperando al otro. Pedir revisión sigue siendo lo normal, y es obligatoria de hecho para cambios en `contracts/`, porque ahí el acuerdo entre los dos es el punto.

## Docker y CI/CD

- Un Dockerfile por módulo, con **multi-stage build** (compilar en una imagen y copiar el resultado a una imagen mínima).
- `docker-compose.yml` en la raíz levanta Go + Scala (y el simulador como servicio opcional).
- Pipeline en GitHub Actions (`.github/workflows/`):
  - En cada PR: `go vet` + `go test` + `go build`; `sbt test` + `sbt assembly`; validación de los ejemplos contra los esquemas; `docker compose build` y prueba de humo de `/health`.
  - En merge a `main`: construir imágenes, publicar en GHCR y desplegar (entorno por decidir en Fase 3).

## Comandos

```bash
# Go
cd ingest-go && go vet ./... && go test ./... && go build ./...

# Scala
cd analytics-scala && sbt test

# Contrato (requiere: pip install jsonschema)
python contracts/validate.py

# Todo integrado
docker compose up --build
curl http://localhost:8080/health   # ingestión
curl http://localhost:8081/health   # análisis
```

## Cómo debe trabajar Claude Code aquí

- Lee `docs/enunciado.md` y este archivo antes de proponer cambios grandes.
- Si falta una decisión de diseño que afecta el contrato, **pregunta** en vez de inventarla.
- No cambies `contracts/` sin que se pida explícitamente.
- Escribe pruebas junto con el código (Go: `testing`; Scala: MUnit).
- Explica brevemente las decisiones importantes: los estudiantes deben poder **defender cada línea** en la demo.
- Identificadores de código en inglés; documentación y comentarios en español.
- No agregues dependencias sin justificar por qué la biblioteca estándar no alcanza.
- Nunca hagas push a `main` ni fuerces pushes.

## Entregables (checklist del enunciado)

- [ ] Código fuente Go y Scala
- [x] Especificación del formato de eventos — `contracts/`
- [ ] Descripción de los juegos y justificación de los eventos
- [ ] Descripción de estadísticas y reglas — definiciones en este archivo, falta el documento final
- [x] Dockerfiles y docker-compose
- [x] Configuración del pipeline CI/CD — falta la parte de deployment (Fase 3)
- [ ] Documentación de instalación, ejecución y deployment — `README.md` cubre instalación y ejecución
- [ ] Demo funcional
- [ ] Justificación de qué va en imperativo (Go) y qué en funcional (Scala), con ventajas y limitaciones de cada enfoque

## Pendiente administrativo

El enunciado §14 dice "Fecha de entrega: domingo 18 de Setiembre de 2026", una fecha que ya pasó.
Es un error de tipeo en el documento del curso. **Confirmar la fecha real con el profesor.**
