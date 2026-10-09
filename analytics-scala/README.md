# analytics-scala — módulo de análisis

> **Responsable: Gabriel.** El módulo funciona de punta a punta para `racing` y `combat`: recibe el
> lote de una partida, la analiza y expone los resultados.
>
> **Pendiente tras el cambio de requisito del 9 de octubre de 2026:** los analizadores de
> `blackjack` y `battleship`, el `GenericAnalyzer` y las estadísticas globales (perfil del jugador,
> ranking por juego, desconexiones, win rate contra bots). El contrato de todo eso ya está escrito
> en `../contracts/events.md` y `../contracts/results.md`.

Convierte el lote de eventos de una partida en estadísticas y patrones. Es la parte **funcional** del
proyecto: datos inmutables, funciones puras y transformaciones de colecciones.

## Comandos

```bash
sbt test       # pruebas (MUnit)
sbt compile    # compilar
sbt assembly   # jar único, el que usa el Dockerfile
sbt run        # levantar local en el puerto 8081 (PORT lo cambia)
```

## Qué hay ahora

| Archivo | Qué es |
|---|---|
| `Config.scala` | configuración leída del entorno, con `fromEnv` pura |
| `Event.scala` | el sobre del evento y los tipos generales (`EventType`) |
| `Batch.scala` | el lote de una partida, con `parse` y las agrupaciones base |
| `Json.scala` | decodificación a `Either`; el único sitio que atrapa una excepción |
| `Numeros.scala` | redondeo, promedios, desviación poblacional y marcas RFC 3339 |
| `Results.scala` | el sobre de resultados y las ayudas para armar `gameSpecific` |
| `GameAnalyzer.scala` | el trait, el sobre común del resultado y el registro de juegos |
| `RacingAnalyzer.scala` | estadísticas y remontada de carreras |
| `CombatAnalyzer.scala` | estadísticas, racha y venganza de combate |
| *(pendiente)* `BlackjackAnalyzer.scala` | % de victorias, saldo, apuesta promedio, % de bust; regla `tilt` |
| *(pendiente)* `BattleshipAnalyzer.scala` | precisión y barcos hundidos; regla `hitStreak` |
| *(pendiente)* `GenericAnalyzer.scala` | lo que sale solo del sobre, sin conocer el juego |
| `ResultStore.scala` | lo analizado hasta ahora, en memoria |
| `Main.scala` | los endpoints: el borde HTTP |

Las pruebas van en `src/test/.../{Config,Health,Batch,Analyzer,ResultStore}Suite.scala`: **47 en total**.

## Endpoints

| Método y ruta | Qué hace |
|---|---|
| `POST /analyze` | recibe el lote de una partida, lo analiza, lo guarda y lo devuelve |
| `GET /results/matches/{matchId}` | el resultado de una partida analizada, 404 si no está |
| `GET /results/players/{playerId}` | el agregado del jugador, 404 si no aparece en ninguna |
| `GET /health` | para `docker compose` y el CI |

`POST /analyze` responde 400 en dos casos distintos: el lote no cumple el contrato, o el `gameId`
no tiene analizador. El segundo le dice a Go que mandó un juego que este módulo todavía no conoce,
que es un problema de despliegue y no de datos.

## El modelo (Día 1)

`Event` es el espejo de `event.schema.json` y `Batch` el de `batch.schema.json`. Tres cosas que vale
la pena saber antes de tocarlos:

- **`type` → `eventType`.** `type` es palabra reservada en Scala; `@upickle.implicits.key("type")`
  lo mapea al nombre del contrato, que es el que viaja por el cable. Lo mismo pasa con `match` en
  `MatchResult`, que en Scala se llama `matchSummary`.
- **`data` es `ujson.Value`**, JSON sin interpretar, igual que el `json.RawMessage` de Go. Los
  accesores `str` / `int` / `long` / `num` devuelven `Option`, así que un campo ausente da `None` en
  vez de lanzar. `data: null` tampoco rompe nada.
- **`eventType` es `String`, no un `enum`.** Un `enum` cerrado obligaría a tocar el núcleo cada vez
  que un juego nuevo trae un tipo nuevo, que es justo lo que el enunciado §12 pide evitar. Los tipos
  generales están en `EventType`; los de cada juego viven en su analizador.

`Batch.parse` devuelve `Either[String, Batch]` y comprueba dos cosas que el esquema no puede
expresar: que el lote traiga al menos un evento y que ninguno sea de otra partida.

`BatchSuite` lee los JSON de `contracts/examples/` en vez de copiarlos a `src/test/resources`: el
contrato tiene una sola fuente de verdad. Si alguien cambia el esquema sin actualizar el modelo, esas
pruebas se caen, que es lo que queremos que pase.

## Los analizadores (Día 2)

`GameAnalyzer` arma el sobre del resultado **una sola vez**; cada juego solo aporta quién ganó, qué
va en el `gameSpecific` de la partida, qué va en el de cada jugador y qué patrones disparó. Agregar
un juego es escribir un objeto que extienda el trait y añadir una línea al registro de
`object GameAnalyzer`: no se toca ni el sobre, ni los endpoints, ni los otros juegos.

Que pasar de dos juegos a cuatro no obligue a tocar nada de lo ya escrito es la prueba de que ese
diseño era el correcto. Los dos analizadores nuevos son archivos nuevos y dos líneas en el registro.

**Dos cosas que los analizadores nuevos traen y los viejos no:** en `blackjack` el valor de la mano
no viene en los eventos y hay que reconstruirlo con un `foldLeft` que lleve el total y la cantidad
de ases (el as vale 11 salvo que pase de 21); en `battleship` el `playerId` de `SHIP_SUNK` es el
**dueño** del barco hundido, no quien disparó, igual que la víctima en `combat`.

Todo el cálculo son funciones puras del lote. `analizar` recibe el reloj como parámetro en vez de
llamar a `Instant.now()` por dentro — el mismo patrón que `Config.fromEnv` — así el resultado es
reproducible y las pruebas no dependen de la hora.

Las convenciones numéricas (redondeo a 2 decimales, desviación **poblacional**, promedio sin
muestras = `null`) están todas en `Numeros.scala`, en un solo sitio, para que sea imposible
redondear distinto en dos lugares.

## El almacén (Día 3)

El módulo tiene que recordar lo que analizó para responder los `GET /results/...`, o sea que hay
estado. `ResultStore` lo resuelve con un `AtomicReference` que apunta a un `Map` **inmutable**: lo
único mutable es la referencia, que se cambia de golpe con `updateAndGet`. No hay `var`, no hay
colección mutable, y dos peticiones simultáneas no pueden dejar el mapa a medias.

Es intencionalmente en memoria: al reiniciar se pierde. Persistir queda fuera del alcance y Go
puede reenviar la partida.

El agregado por jugador (`byGame`) se lo pide al analizador de cada juego, porque solo el juego sabe
qué significa agregar lo suyo: en carreras la mejor vuelta es un mínimo y los adelantamientos una
suma; en combate el K/D hay que **recalcularlo** con los totales, no promediar los K/D de cada
partida.

## Reglas de estilo

Del `CLAUDE.md` de la raíz: sin `var`, sin colecciones mutables, sin `null`, `sealed trait` y case
classes para el modelo, `Option` / `Either` en vez de excepciones, y el análisis en funciones puras.

## Versiones

Scala **3.3.8** (última de la línea LTS), sbt **1.13.0**, MUnit **1.3.6**, Java **21**, cask
**0.11.3** y upickle **4.4.3**. El tag de la imagen de Docker fija las mismas versiones de Java, sbt
y Scala, para que lo que compila en la máquina sea lo que compila en el contenedor.

## Decisión 9: por qué cask + upickle

Decisión cerrada. El enunciado §12 pide que las decisiones de arquitectura estén justificadas
técnicamente, así que acá está el porqué, incluidas las limitaciones.

Las alternativas que se consideraron:

| Opción | A favor | En contra |
|---|---|---|
| `com.sun.net.httpserver` + JSON a mano | cero dependencias | serializar JSON a mano es tedioso y propenso a errores |
| **cask + upickle** ✅ | API mínima, poco código, defendible línea por línea | menos usada en la industria |
| http4s + circe | la combinación estándar del Scala funcional | arrastra la mónada `IO` a todo el borde de HTTP |
| tapir | describe los endpoints como datos | otra capa de abstracción encima |

**El argumento principal: el paradigma funcional de este proyecto vive en el análisis, no en el
transporte.** Lo que el curso evalúa son las transformaciones sobre colecciones inmutables —
`filter`, `map`, `groupBy`, `fold` — y esas son idénticas se reciban los bytes como se reciban. El
`CLAUDE.md` ya establece que el IO queda aislado en los bordes; http4s llevaría la mónada `IO` a ese
borde, que es varios días de conceptos nuevos sin mejorar la parte que sí se evalúa.

**Por qué no JSON a mano:** upickle deriva los codecs en tiempo de compilación con `derives
ReadWriter`. Si alguien renombra un campo de una case class, el código deja de compilar en vez de
producir un JSON equivocado en ejecución. Escribir la serialización a mano pierde esa garantía justo
en la parte más tediosa, la de los resultados.

**Limitación que asumimos, y hay que decirla en la defensa:** http4s + circe es la opción más
funcional de verdad y la que mejor se vería como demostración del paradigma. cask es un framework de
anotaciones, más cercano a lo imperativo, y está confinado a `Main.scala`. Se eligió simplicidad
sobre pureza en el borde, de forma consciente y a cambio de tiempo para el análisis, que es donde
está la nota.

### Dos detalles que esto deja fijados

- **El campo `data` del evento se modela como `ujson.Value`**, o sea JSON sin interpretar. Es el
  espejo exacto del `json.RawMessage` del lado de Go: el núcleo no conoce los detalles de cada juego
  y solo el analizador correspondiente abre ese campo. Es lo que permite que agregar un juego nuevo
  sea agregar un `GameAnalyzer` y nada más.
- **`Main` sobrescribe `host` a `0.0.0.0`.** cask escucha en `localhost` por defecto, y dentro de un
  contenedor eso solo acepta conexiones del propio contenedor: el servicio de Go no podría
  alcanzarnos y el healthcheck de `docker compose` fallaría.

### El campo `type` del contrato

`type` es palabra reservada en Scala. Cuando se escriban las case classes del evento, el campo se
llama `eventType` en el código y se mapea al nombre del contrato con `@upickle.implicits.key("type")`.
