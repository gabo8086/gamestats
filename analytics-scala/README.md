# analytics-scala — módulo de análisis

> **Responsable: Gabriel.** Al terminar la Fase 0 esto es un esqueleto: compila, pasa `sbt test` y
> responde `/health`. Las estadísticas y las reglas llegan el Día 2.

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
| `src/main/scala/gamestats/analytics/Config.scala` | configuración leída del entorno, con `fromEnv` pura |
| `src/main/scala/gamestats/analytics/Main.scala` | servidor cask con `/health` y la case class `Health` |
| `src/test/scala/gamestats/analytics/ConfigSuite.scala` | pruebas de `Config` |
| `src/test/scala/gamestats/analytics/HealthSuite.scala` | pruebas del codec derivado por upickle |

`Config.fromEnv` recibe la función de lectura como parámetro en vez de llamar a `sys.env` por
dentro. Es el patrón de todo el módulo: el cálculo es puro y el IO (HTTP, JSON, entorno) queda en los
bordes, de modo que las pruebas no necesitan montar nada. `HealthSuite` es el mismo patrón aplicado
al JSON: prueba el codec como función pura, sin levantar el servidor.

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
