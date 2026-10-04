# analytics-scala — módulo de análisis

> **Responsable: Samuel.** Al terminar la Fase 0 esto es un esqueleto: compila, pasa `sbt test` y
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
| `src/main/scala/gamestats/analytics/Main.scala` | servidor mínimo con `/health` |
| `src/test/scala/gamestats/analytics/ConfigSuite.scala` | pruebas de `Config` |

`Config.fromEnv` recibe la función de lectura como parámetro en vez de llamar a `sys.env` por
dentro. Es el patrón de todo el módulo: el cálculo es puro y el IO (HTTP, JSON, entorno) queda en los
bordes, de modo que las pruebas no necesitan montar nada.

## Reglas de estilo

Del `CLAUDE.md` de la raíz: sin `var`, sin colecciones mutables, sin `null`, `sealed trait` y case
classes para el modelo, `Option` / `Either` en vez de excepciones, y el análisis en funciones puras.

## Versiones

Scala **3.3.8** (última de la línea LTS), sbt **1.13.0**, MUnit **1.3.6**, Java **21**. El tag de la
imagen de Docker fija las mismas tres primeras, para que lo que compila en la máquina sea lo que
compila en el contenedor.

## Decisión pendiente: bibliotecas de HTTP y JSON

Es la **decisión 9 de la Fase 0 y es de Samuel**. Hoy `build.sbt` solo depende de MUnit (pruebas) y
`Main` usa `com.sun.net.httpserver`, que viene en el JDK, para no fijar nada todavía. Ese servidor es
provisional: se reemplaza sin tocar el resto del módulo, porque el análisis no sabe de HTTP.

Candidatos, de más simple a más completo:

| Opción | A favor | En contra |
|---|---|---|
| `com.sun.net.httpserver` + JSON a mano | cero dependencias | serializar JSON a mano se vuelve tedioso y propenso a errores |
| **cask + upickle** | API mínima, poco código, fácil de defender línea por línea | menos usada en la industria |
| http4s + circe | la combinación estándar de Scala funcional | mucho concepto nuevo (`IO`, efectos) para el tiempo que hay |
| tapir | describe los endpoints como datos | otra capa de abstracción encima |

**Propuesta: cask + upickle**, por la prioridad de simplicidad del proyecto. http4s + circe es la
opción más funcional de verdad y queda mejor en la defensa, pero arrastra la mónada `IO` a todo el
borde de HTTP y hay 4 días. La decisión es de Samuel.

Lo que se decida va documentado con su justificación: el enunciado §12 pide que las decisiones de
arquitectura estén justificadas técnicamente.
