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

| Pieza | Estado |
|---|---|
| Ingestión (Go) | completa: `POST /events`, validación, una goroutine por partida, envío a Scala, `GET /metrics` |
| Análisis (Scala) | completo: los cuatro analizadores, `GenericAnalyzer`, perfil global, ranking, desconexiones |
| Docker | una sola imagen para todo; `docker compose` la usa para cada servicio |
| CI | contrato, Go, Scala, imagen y prueba de integración de punta a punta |
| Simulador | en Python, **solo `racing` y `combat`**; el de Go con los cuatro juegos (decisión 17) está pendiente |
| Deployment | la imagen se publica en GitHub Container Registry (amd64 y arm64) y el CI prueba la publicada |

Mientras el simulador no cubra los cuatro juegos, `blackjack` y `battleship` se prueban con las
partidas de `contracts/examples/` (modo `ejemplos`, abajo).

El 9 de octubre de 2026 el requisito pasó de dos juegos a **cuatro**, con dos preparados para
Jugador vs Máquina. Es una indicación **verbal** del profesor: `docs/enunciado.md` §5 no fija un
mínimo.

Las decisiones de diseño están en la tabla de [`diseno.md`](docs/diseno.md) y **casi ninguna está
aprobada todavía**: Samuel tiene que revisarlas.

## Estructura

| Carpeta | Qué es | Responsable |
|---|---|---|
| [`contracts/`](contracts/) | formato de eventos, lote y resultados: la fuente de verdad | ambos |
| [`ingest-go/`](ingest-go/) | módulo de ingestión (Go) | Samuel |
| [`analytics-scala/`](analytics-scala/) | módulo de análisis (Scala) | Gabriel |
| [`simulator/`](simulator/) | fuente de eventos simulada | Gabriel |
| [`docker/`](docker/) | punto de entrada, healthcheck y envío de ejemplos de la imagen | Samuel |
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

Para correr solo con Docker basta Docker: las versiones de Go, JDK, sbt y Python están fijadas en
el `Dockerfile`, y el simulador corre dentro de la imagen.

## Ejecución

### Docker: una sola imagen

El [`Dockerfile`](Dockerfile) de la raíz produce **una imagen** con los dos módulos y el
simulador. Es multi-stage: una etapa compila Go, otra compila Scala, y la final solo trae un JRE,
el binario de Go, el jar de Scala y Python para el simulador (~500 MB).

Una imagen no es un proceso: el primer argumento elige qué levantar
([`docker/entrypoint.sh`](docker/entrypoint.sh)).

| Modo | Qué levanta |
|---|---|
| `all` (por defecto) | Go y Scala en el mismo contenedor, en dos procesos que se hablan por HTTP |
| `analytics` / `ingest` | uno solo de los dos servicios |
| `simulator` | el simulador; los argumentos los lista con `-h` |
| `ejemplos` | manda a Go las partidas de los cuatro juegos de `contracts/examples/` y un caso por cada razón de rechazo, y compara cada rechazo con el esperado |

#### Con `docker compose` (lo normal)

Compose corre la misma imagen tres veces, cada módulo en su contenedor:

```bash
docker compose up --build -d --wait            # Go (8080) + Scala (8081), espera a que estén sanos
docker compose run --rm ejemplos               # los cuatro juegos + los rechazos
docker compose run --rm simulator --no-files --delay-ms 0   # flujo del simulador (racing y combat)
docker compose logs -f ingest                  # ver cómo Go manda cada lote a Scala
docker compose down                            # bajar todo
```

#### Con un solo contenedor

```bash
docker build -t gamestats .
docker run -d --name gamestats -p 8080:8080 -p 8081:8081 gamestats
docker exec gamestats gamestats ejemplos
docker exec gamestats gamestats simulator --no-files --delay-ms 0
docker rm -f gamestats
```

Si uno de los dos procesos se cae, el contenedor termina con error en vez de quedar con medio
sistema vivo. El healthcheck de la imagen revisa los servicios del modo con que arrancó.

#### Qué consultar

```bash
curl http://localhost:8080/health                               # ingestión (Go)
curl http://localhost:8081/health                               # análisis (Scala)
curl http://localhost:8080/metrics                              # eventos recibidos, rechazos por razón
curl http://localhost:8081/results/matches/m-racing-001         # regla comeback
curl http://localhost:8081/results/matches/m-combat-001         # reglas streak y revenge
curl http://localhost:8081/results/matches/m-blackjack-001      # regla tilt
curl http://localhost:8081/results/matches/m-battleship-001     # regla hitStreak
curl http://localhost:8081/results/players/p1                   # perfil global del jugador
curl http://localhost:8081/results/games/combat/ranking         # ranking por juego
curl http://localhost:8081/results/matches/m-sim-racing-comeback   # una del simulador
```

En PowerShell, `curl` es un alias de otra cosa: usar `curl.exe` o `Invoke-RestMethod`.

`ejemplos` está pensado para un sistema recién levantado: si se corre dos veces, la segunda los
eventos ya existen y Go los rechaza con `duplicate_event`, que es lo correcto.

Dentro de Compose los servicios se llaman por nombre (`http://analytics:8081`), nunca `localhost`:
cada contenedor tiene su propio `localhost`. En el modo `all` sí es `127.0.0.1`, porque los dos
procesos comparten contenedor.

#### Por qué una imagen y no tres

Un solo `Dockerfile` es un solo lugar donde fijar versiones, y una sola imagen que publicar y
desplegar. Lo que el enunciado §7 pide —que Go y Scala sean **componentes independientes** que se
comunican por un mecanismo definido— se mantiene: siguen siendo dos programas, en dos procesos, que
solo se conocen por HTTP. Compose además los pone en contenedores separados.

El costo es que cambiar una línea de Go reconstruye la imagen entera, aunque la caché de Docker
reutiliza la etapa de Scala si su código no cambió.

## Deployment: la imagen publicada

Cada push a `main` publica la imagen en **GitHub Container Registry** como
`ghcr.io/gabo8086/gamestats`, para `linux/amd64` y `linux/arm64` (las Mac con chip M). Cualquier
persona con Docker la puede usar **sin el código y sin compilar nada**.

| Tag | Qué es |
|---|---|
| `latest` | lo último que entró a `main` |
| `sha-<commit>` | la imagen de un commit concreto; nunca cambia |
| `v1.0` | la versión de la entrega; sale al empujar el tag git `v1.0` |

### Para quien la quiera probar

Con un solo contenedor, basta un comando:

```bash
docker run -d --name gamestats -p 8080:8080 -p 8081:8081 ghcr.io/gabo8086/gamestats:latest
docker exec gamestats gamestats ejemplos
curl http://localhost:8081/results/matches/m-blackjack-001
docker rm -f gamestats
```

O con compose, cada módulo en su contenedor. Solo hace falta el archivo
[`docker-compose.deploy.yml`](docker-compose.deploy.yml), que descarga la imagen en vez de
construirla:

```bash
docker compose -f docker-compose.deploy.yml up -d --wait
docker compose -f docker-compose.deploy.yml run --rm ejemplos
docker compose -f docker-compose.deploy.yml run --rm simulator --no-files --delay-ms 0
docker compose -f docker-compose.deploy.yml down -v
```

`GAMESTATS_TAG` elige la versión (por defecto `latest`). En PowerShell:
`$env:GAMESTATS_TAG = "v1.0"` antes de los comandos.

### Cómo se publica

Los jobs `publish` y `deploy` del CI corren solo en push a `main` o en un tag `v*`, y solo si los
cuatro jobs de pruebas pasaron. `publish` construye para las dos arquitecturas: la compilación de
Go y de Scala corre nativa y solo la etapa final es por arquitectura, porque Go compila cruzado y
el jar sirve en cualquiera. `deploy` levanta `docker-compose.deploy.yml` con la imagen recién
publicada, **sin `docker login`**, manda los ejemplos y comprueba las cuatro reglas. Es
exactamente lo que va a hacer el profe, así que si eso pasa, a él le funciona.

Para la entrega:

```bash
git switch main && git pull
git tag v1.0
git push origin v1.0
```

El entorno de deployment que definimos es **Docker en la máquina de quien evalúa**, con la imagen
publicada en GHCR. No hay un servidor siempre encendido.

**La primera vez hay que hacer público el paquete.** GHCR crea los paquetes privados, y en ese
caso nadie puede bajar la imagen sin login, y el job `deploy` falla a propósito. Lo cambia el dueño
del repo (`gabo8086`): en GitHub, *Packages → gamestats → Package settings → Change visibility →
Public*. Se hace una sola vez.

### Por módulo

```bash
cd ingest-go       && go vet ./... && go test -race ./... && go build ./...
cd analytics-scala && sbt test
cd simulator       && go vet ./... && go test ./...
```

### Validar el contrato

```bash
pip install jsonschema
python contracts/validate.py
```

Comprueba que los ejemplos cumplen los esquemas y las reglas que JSON Schema no puede expresar
(`eventId` único, eventos ordenados por `timestamp`, un caso por cada razón de rechazo).

## Juegos

Cuatro juegos de forma deliberadamente distinta, para demostrar que el núcleo es genérico: agregar un
juego significa agregar un analizador en Scala, sin tocar Go ni el sobre del evento.

| Juego | Modo | Eventos propios | Regla de secuencia |
|---|---|---|---|
| **Carreras** (`racing`) | PVP | vueltas y penalizaciones | **remontada** — estuvo último y ganó |
| **Combate** (`combat`) | PVP o PVE | eliminaciones con arma y daño | **racha** — 3 eliminaciones en ≤10 s sin morir |
| **Blackjack** (`blackjack`) | PVE | apuestas, cartas y resultado de ronda | **tilt** — sube la apuesta tras 3 derrotas |
| **Hundir la flota** (`battleship`) | PVE | disparos y barcos hundidos | **racha de aciertos** — 3 seguidos |

En los modos PVE la máquina es un jugador más, con `playerType: "BOT"`. El sistema no implementa la
IA; el contrato queda listo para acoplarla.

Además hay estadísticas que **no dependen de ningún juego** y se calculan igual para los cuatro:
distribución de eventos, actividad por minuto, perfil global del jugador, ranking por juego,
desconexiones y win rate contra bots frente a contra humanos.

El catálogo de eventos y su justificación está en [`contracts/events.md`](contracts/events.md), el
detalle de cada estadística en [`diseno.md`](docs/diseno.md), y la forma exacta de la salida en
[`contracts/results.md`](contracts/results.md).

## Documentación

| Archivo | Qué contiene |
|---|---|
| [`diseno.md`](docs/diseno.md) | decisiones de diseño, definiciones precisas, reparto de trabajo |
| [`contracts/event.schema.json`](contracts/event.schema.json) | el sobre común del evento |
| [`contracts/events.md`](contracts/events.md) | catálogo de tipos de evento por juego y su justificación |
| [`ParaSamuel.md`](ParaSamuel.md) | resumen de todas las decisiones tomadas, con el porqué |
| [`contracts/batch.schema.json`](contracts/batch.schema.json) | el lote que Go envía a Scala |
| [`contracts/results.md`](contracts/results.md) | forma del JSON de resultados |
| [`contracts/rejections.md`](contracts/rejections.md) | razones de rechazo de eventos |
| [`docs/paradigmas.md`](docs/paradigmas.md) | qué va en imperativo y qué en funcional, con ventajas y limitaciones |
| [`docs/enunciado.md`](docs/enunciado.md) | enunciado del curso (el `.docx` original está al lado) |

## Entrega

El enunciado §14 dice *"domingo 18 de Setiembre de 2026"*, fecha que ya pasó: es un error de tipeo
en el documento. **Confirmar la fecha real con el profesor.**
