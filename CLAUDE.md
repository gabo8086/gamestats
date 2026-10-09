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
| **Gabriel** | Módulo Scala: los cuatro analizadores, el `GenericAnalyzer` y las estadísticas globales. Simulador (en Go), datos de prueba y pruebas | `analytics-scala/`, `simulator/` |
| **Samuel** | Módulo Go: ingestión, estado por partida, concurrencia y `GET /metrics`. Docker, docker-compose, CI/CD, deployment | `ingest-go/`, `docker-compose.yml`, `.github/` |
| **Ambos** | Contrato, documentación, integración y demo | `contracts/`, `docs/` |

Los roles se intercambiaron el 3 de octubre de 2026, a pedido de Samuel: antes Gabriel llevaba Go y Samuel Scala. El andamiaje de `ingest-go/` que entra en el PR de la Fase 0 lo escribió Gabriel **antes** del intercambio; a partir del merge ese módulo es de Samuel.

El 9 de octubre de 2026 el requisito pasó de dos juegos a **cuatro**, con dos de ellos preparados para Jugador vs Máquina. El reparto no cambió de dueños: Gabriel suma los dos analizadores nuevos, el `GenericAnalyzer` y las estadísticas globales; Samuel suma `GET /metrics`. Lo único que cambia de forma es el **simulador, que se reescribe en Go** sin dejar de ser de Gabriel: el lenguaje no sigue al dueño. Hasta que ese reemplazo exista, el simulador en Python que está en `simulator/` sigue siendo el que funciona.

Cada quien trabaja en sus carpetas para evitar conflictos de merge. Cambios en `contracts/` requieren aprobación de ambos.
Claude Code: si te piden algo fuera de las carpetas de quien te habla, avisa antes de tocarlo.

## Arquitectura

```
simulador ──JSON──> [ Go: ingestión ] ──HTTP/JSON──> [ Scala: análisis ] ──> resultados JSON
 (4 juegos,         valida, agrupa por partida,       4 analizadores +         GET /results/...
  en paralelo)      concurrencia, estado              GenericAnalyzer          puerto 8081
                    GET /metrics
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
6. Exponer `GET /metrics`: eventos recibidos, válidos, inválidos **desglosados por razón de rechazo** (los códigos de `contracts/rejections.md`) y partidas activas.

NO hace: estadísticas ni reglas de juego. Eso es de Scala. `GET /metrics` son métricas de la **ingestión**, no del juego: cuántos eventos entraron y cuántos se rechazaron y por qué. Go sigue sin saber qué es una vuelta ni una eliminación.
Preferir la biblioteca estándar (`net/http`, `encoding/json`, `sync`). Tratar `data` como JSON opaco (`json.RawMessage`): Go no debe conocer los detalles de cada juego.

### Scala (`analytics-scala/`) — qué hace
1. Recibir el lote de una partida y convertirlo a case classes inmutables.
2. Elegir un analizador según `gameId` (`trait GameAnalyzer`, un objeto por juego: `RacingAnalyzer`, `CombatAnalyzer`, `BlackjackAnalyzer`, `BattleshipAnalyzer`). Agregar un juego nuevo = agregar un analizador y una línea en el registro, sin tocar el resto.
3. Calcular estadísticas y reglas con operaciones funcionales.
4. Calcular con el `GenericAnalyzer` lo que **no depende de ningún juego**, usando solo el sobre común: distribución por tipo de evento y actividad por minuto. Un juego nuevo trae esas métricas gratis.
5. Calcular las estadísticas globales que cruzan partidas y juegos: perfil del jugador, ranking por juego, desconexiones y win rate contra bots frente a contra humanos.
6. Producir resultados JSON y exponerlos (`GET /results/matches/{matchId}`, `GET /results/players/{playerId}`, `GET /results/games/{gameId}/ranking`). Forma exacta en `contracts/results.md`.

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

Eventos generales (todos los juegos): `MATCH_STARTED`, `PLAYER_JOINED`, `PLAYER_DISCONNECTED`, `PLAYER_RECONNECTED`, `MATCH_FINISHED`.

**El catálogo completo de tipos y la forma de `data` por juego está en `contracts/events.md`**, con la justificación de cada evento. Dos campos de ahí los usa el núcleo y conviene tenerlos presentes:

- `MATCH_STARTED.data.matchMode`: `"PVP"` o `"PVE"`.
- `PLAYER_JOINED.data.playerType`: `"HUMAN"` o `"BOT"`, con `botLevel` opcional (`"easy"` / `"hard"`).

Van en `data` y no en el sobre porque el sobre es común a los diecisiete tipos de evento y estos dos solo aplican a uno cada uno. Son la preparación para **Jugador vs Máquina**: un bot es un jugador con un atributo, no un caso especial del modelo, y eso es lo que permite calcular «win rate contra bots» sin lógica aparte. Este proyecto no implementa la IA; solo deja el contrato listo para acoplarla.

## Juegos elegidos (propuesta; confirmar en Fase 0)

Cuatro juegos de forma deliberadamente distinta, para demostrar que el núcleo es genérico. Dos de ellos (`blackjack` y `battleship`) son Jugador vs Máquina, y `combat` puede serlo. El detalle de los eventos y el porqué de cada uno está en `contracts/events.md`.

Cada juego tiene **al menos tres estadísticas y una regla de secuencia**, que es lo que piden los puntos 6 a 9 del enunciado §9.

### Carreras (`gameId: "racing"`) — PVP
- Eventos propios: `LAP_COMPLETED` (`{lap, lapTimeMs, position}`), `PENALTY` (`{seconds, reason}`).
- Por jugador: mejor vuelta, tiempo promedio, consistencia (desviación poblacional), penalizaciones acumuladas.
- Por partida: ganador, vuelta rápida de la carrera, total de adelantamientos.
- Combinando tipos de evento: **tiempo ajustado** = `LAP_COMPLETED` + `PENALTY`; define el ganador.
- Regla de secuencia: **remontada**.

### Combate (`gameId: "combat"`) — PVP o PVE
- Eventos propios: `PLAYER_ELIMINATED` (`playerId` = quien elimina; `data`: `{victimId, weapon, damage}`).
- Por jugador: eliminaciones, muertes, K/D, daño promedio y **K/D contra bots**.
- Por partida: jugador más letal y **primera sangre** (el primer `PLAYER_ELIMINATED`).
- Combinando eventos: eliminaciones por arma / daño promedio por arma.
- Reglas de secuencia: **racha** y **venganza**.

Con el requisito nuevo los bots participan como jugadores: una eliminación puede tener como autor o como víctima a un bot. Eso no cambia el evento, solo el `playerType` de quien se unió.

### Blackjack (`gameId: "blackjack"`) — PVE, contra el crupier
- Eventos propios: `BET_PLACED` (`{amount}`), `CARD_DEALT` (`{card, to}`), `PLAYER_HIT`, `PLAYER_STAND`, `ROUND_RESULT` (`{outcome, payout}`).
- Por jugador: % de victorias, saldo neto, apuesta promedio, % de rondas en que se pasa de 21.
- Por partida: rondas jugadas y saldo del crupier.
- Regla de secuencia: **tilt**.

**El valor de la mano no viaja en los eventos.** Scala lo reconstruye con un `foldLeft` sobre las cartas de la ronda. Es deliberado: si viniera calculado, el análisis sería leer un número en vez de derivarlo, que es justo lo que el enunciado §2 pide evitar.

### Hundir la flota (`gameId: "battleship"`) — PVE, contra la máquina
- Eventos propios: `SHOT_FIRED` (`{coord, hit}`), `SHIP_SUNK` (`playerId` = **dueño** del barco hundido; `data`: `{ship, size}`).
- Por jugador: precisión y barcos hundidos.
- Por partida: tiros hasta ganar y promedio de tiros por barco hundido.
- Regla de secuencia: **racha de aciertos**.

El `playerId` de `SHIP_SUNK` es el dueño del barco, no quien disparó, igual que en combate la víctima no es el autor. Misma convención en los dos juegos para no tener que recordar una excepción.

## Estadísticas generales de plataforma

No pertenecen a ningún juego y por eso se calculan una sola vez para los cuatro.

- **`GenericAnalyzer`** — usa **solo el sobre común**, sin una línea de código de ningún juego: duración de la partida, cantidad de eventos, distribución por tipo de evento y actividad por minuto. Que esto funcione sin saber de qué juego se trata es la demostración concreta de que el sobre alcanza.
- **Perfil global del jugador** — partidas por juego y % de victorias total, para el mismo `playerId` en varios juegos.
- **Ranking por juego** — los N mejores.
- **Desconexiones** — `PLAYER_DISCONNECTED` seguido de `PLAYER_RECONNECTED` en **≤ 60 s** cuenta como reconexión; si nunca vuelve, es abandono.
- **Win rate contra bots frente a contra humanos** — particionando por `playerType`.

## Simulador y datos de prueba (`simulator/`)

Simula al sistema de juegos externo. **Se escribe en Go** (decisión 17): un simulador concurrente en Go refuerza el paradigma imperativo del proyecto y evita sumar un tercer lenguaje al despliegue. Sigue siendo de Gabriel; el lenguaje no sigue al dueño.

Debe generar: partidas de los **cuatro** juegos **en paralelo**, varios jugadores, humanos y bots, casos normales y casos armados a mano que disparen **cada regla** (una remontada, una racha de combate, un tilt, una racha de aciertos), eventos fuera de orden y al menos un evento inválido por cada razón de `contracts/rejections.md`.

Salida: archivos JSON y/o envío a `POST /events`. Los datos de racing deben ser coherentes (posiciones derivadas de tiempos acumulados) y los de blackjack también (el `outcome` tiene que concordar con el valor de las manos).

**Estado:** hoy `simulator/` tiene un simulador en **Python** que cubre dos juegos y funciona. Se mantiene hasta que exista el reemplazo en Go, para no dejar el repo sin forma de probar el análisis; el borrado va en el PR que traiga el de Go.

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
| 3 | Definición exacta de cada estadística y regla | Ver "Definiciones precisas" abajo | G | ✅ |
| 4 | Comunicación Go→Scala | HTTP/JSON: Go hace `POST /analyze` a Scala con el lote de una partida (`contracts/batch.schema.json`) | A | ⬜ |
| 5 | Cuándo envía Go una partida | Al recibir `MATCH_FINISHED`, con los eventos ordenados por `timestamp` | S | ⬜ |
| 6 | Casos límite en Go | Ver "Casos límite" abajo y `contracts/rejections.md` | S | ⬜ |
| 7 | Formato y endpoints de resultados | `GET /results/matches/{id}` y `GET /results/players/{id}`; forma exacta en `contracts/results.md` | G | ✅ |
| 8 | Puertos | Go `8080`, Scala `8081` | S | ⬜ |
| 9a | Versiones y bibliotecas de Go | Go 1.24, solo biblioteca estándar | S | ⬜ |
| 9b | Versiones y bibliotecas de Scala | Scala 3.3.8 (LTS), sbt 1.13.0, Java 21, MUnit 1.3.6, **cask 0.11.3** (HTTP) y **upickle 4.4.3** (JSON). Justificación y alternativas descartadas en `analytics-scala/README.md` | G | ✅ |
| 10 | Repo, CI y deployment | GitHub, GitHub Actions, GHCR; entorno de deployment por elegir | S | ⬜ |
| 11 | Reparto y ritmo | Tabla de "Reparto de trabajo" y cronograma de 4 días | A | ⬜ |
| 12 | **Cuatro juegos** | `racing`, `combat`, `blackjack`, `battleship`; dos de ellos preparados para Jugador vs Máquina | A | ⬜ |
| 13 | **`matchMode` y `playerType`** | `MATCH_STARTED.data.matchMode` (`PVP`/`PVE`) y `PLAYER_JOINED.data.playerType` (`HUMAN`/`BOT`, con `botLevel` opcional). Justificación en `contracts/events.md` | A | ⬜ |
| 14 | **Formatos de los juegos nuevos** | Carta `"10H"`, `to` = `playerId` del receptor, `outcome` ∈ {`WIN`,`LOSE`,`PUSH`,`BLACKJACK`,`BUST`}, `payout` con signo; `coord` `"A1"`–`"J10"`, tablero 10×10, flota estándar de 5 barcos | A | ⬜ |
| 15 | **Desconexiones** | `PLAYER_DISCONNECTED` / `PLAYER_RECONNECTED` como eventos generales; reconexión si vuelve en **≤ 60 s**, abandono si no. Razón de rechazo `reconnect_without_disconnect`, opcional | A | ⬜ |
| 16 | **`GET /metrics` en Go** | Eventos recibidos, válidos, inválidos por razón de rechazo y partidas activas | S | ⬜ |
| 17 | **Simulador en Go** | Se reescribe en Go el simulador que hoy está en Python; sigue siendo de Gabriel | G | ✅ |

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
- **Primera sangre (combat):** el autor del primer `PLAYER_ELIMINATED` de la partida por `timestamp`.
- **K/D contra bots (combat):** eliminaciones cuya víctima tiene `playerType: "BOT"` dividido entre las muertes cuyo autor es un bot. Mismo tratamiento del caso de cero muertes que el K/D normal.
- **Vuelta rápida de la carrera (racing):** el menor `lapTimeMs` de toda la partida, con el jugador que lo hizo.
- **Valor de una mano (blackjack):** `foldLeft` sobre las cartas. Cada as suma 11 y el resto su valor (figuras 10); mientras el total pase de 21 y queden ases contados como 11, se le restan 10. Por eso el acumulador lleva el total **y** la cantidad de ases.
- **Tilt (blackjack):** tras tres `ROUND_RESULT` consecutivos del mismo jugador con `outcome` en {`LOSE`, `BUST`}, el `BET_PLACED` de la ronda siguiente es **mayor** que el de la última ronda perdida.
- **% de pasarse de 21 (blackjack):** rondas con `outcome: "BUST"` sobre rondas jugadas.
- **Saldo neto (blackjack):** suma de los `payout`. El saldo del crupier es su negativo.
- **Precisión (battleship):** `SHOT_FIRED` con `hit: true` sobre el total de disparos del jugador.
- **Barcos hundidos por un jugador (battleship):** los `SHIP_SUNK` cuyo `playerId` es **otro** jugador, porque ese campo lleva al dueño del barco.
- **Racha de aciertos (battleship):** tres o más `SHOT_FIRED` consecutivos del mismo jugador con `hit: true`, sin un fallo en medio.
- **Reconexión / abandono:** un `PLAYER_DISCONNECTED` seguido de `PLAYER_RECONNECTED` del mismo jugador en la misma partida dentro de **60 s** es una reconexión; si no aparece el reconnect, es abandono.

Las convenciones numéricas (redondeo, promedios sin muestras, tiempos en milisegundos) están en `contracts/results.md`.

### Casos límite (propuesta)

- Evento con `matchId` sin `MATCH_STARTED` previo → rechazar con razón `match_not_started`.
- Evento posterior a `MATCH_FINISHED` de esa partida → rechazar con razón `match_already_finished`.
- `eventId` repetido → rechazar con razón `duplicate_event`.
- Campo fuera del contrato → rechazar con razón `unknown_field`.
- `PLAYER_RECONNECTED` de un jugador que no estaba desconectado → rechazar con razón `reconnect_without_disconnect` (opcional; ver `contracts/rejections.md`).
- Eventos fuera de orden → aceptarlos y ordenar por `timestamp` antes de enviar a Scala.
- Partida que nunca recibe `MATCH_FINISHED` → fuera de alcance al principio; opcional: timeout.
- Respuesta de error de Scala → Go registra el error y reintenta una vez; no pierde la partida.

### Qué debe quedar en el repo al terminar la Fase 0

```
contracts/
  event.schema.json        # sobre común del evento
  batch.schema.json        # cuerpo del POST /analyze: {matchId, gameId, gameVersion, events:[...]}
  events.md                # catálogo de tipos de evento y forma de data por juego
  results.md               # forma exacta del JSON de resultados por jugador y por partida
  rejections.md            # catálogo de razones de rechazo
  validate.py              # comprueba que los ejemplos cumplen los esquemas (corre en CI)
  examples/
    racing-match-ok.json        # carreras
    combat-match-ok.json        # combate PVP
    combat-vs-bot-match-ok.json # combate PVE, con bots y una desconexión
    blackjack-match-ok.json     # blackjack contra el crupier
    battleship-match-ok.json    # hundir la flota contra la máquina
    invalid-events.json         # al menos un caso por cada razón de rechazo
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

## Cronograma (4 días tras la Fase 0)

Adaptado al estado real del repo: lo tachado ya está en `main`.

| Día | Gabriel (Scala + simulador) | Samuel (Go + infra) |
|---|---|---|
| **1 — Base** | ~~modelo inmutable~~, ~~`GameAnalyzer`~~; **`GenericAnalyzer`** | recibir y validar eventos |
| **2 — Núcleo** | ~~`RacingAnalyzer`~~, ~~`CombatAnalyzer`~~; **K/D contra bots y primera sangre**; **simulador en Go: racing y combat** | estado por partida, goroutines, envío a Scala |
| **3 — Juegos nuevos** | **`BlackjackAnalyzer` y `BattleshipAnalyzer`**; **estadísticas globales y ranking**; **simulador: blackjack y battleship** | **`GET /metrics`** |
| **4 — Integración** | ensayo de la demo | docker-compose, CI/CD, deployment |

La documentación se escribe durante los cuatro días, no al final.

Lo que ya está hecho de la columna de Gabriel: el modelo, el `GameAnalyzer`, los analizadores de racing y combat, el almacén y los endpoints. Lo que falta de su columna es lo que aparece en **negrita**. La columna de Samuel está entera por hacer: `ingest-go/` sigue como quedó en la Fase 0.

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

# Simulador (hoy en Python; pasará a Go, decisión 17)
cd simulator && python -m unittest && python simulate.py

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

- [ ] Código fuente Go y Scala — Scala hecho para 2 de los 4 juegos; Go en esqueleto
- [x] Especificación del formato de eventos — `contracts/event.schema.json` y `contracts/events.md`
- [x] Descripción de los juegos y justificación de los eventos — `contracts/events.md`
- [ ] Descripción de estadísticas y reglas — definiciones en este archivo y formas de salida en `contracts/results.md`; falta el documento final
- [x] Dockerfiles y docker-compose
- [x] Configuración del pipeline CI/CD — falta la parte de deployment (Fase 3)
- [ ] Documentación de instalación, ejecución y deployment — `README.md` cubre instalación y ejecución
- [ ] Demo funcional
- [ ] Justificación de qué va en imperativo (Go) y qué en funcional (Scala), con ventajas y limitaciones de cada enfoque

## Pendiente administrativo

El enunciado §14 dice "Fecha de entrega: domingo 18 de Setiembre de 2026", una fecha que ya pasó.
Es un error de tipeo en el documento del curso. **Confirmar la fecha real con el profesor.**
