# Para Samuel — todas las decisiones, en un solo lugar

Samuel: este documento es el resumen de todo lo que decidí mientras vos no estabas mirando, con el
porqué de cada cosa. La idea es que no tengas que reconstruirlo leyendo commits.

Está ordenado de lo más importante a lo más detallado. **Si solo vas a leer una sección, que sea
"Lo que necesito de vos"**, al final.

Todo lo que digo acá está también en el `CLAUDE.md` y en los README de cada módulo; esto es el mapa,
no la fuente de verdad.

---

## 1. Intercambiamos roles

Lo pediste vos y ya está aplicado en el `CLAUDE.md`:

| | Antes | Ahora |
|---|---|---|
| **Gabriel** | `ingest-go/`, Docker, CI | `analytics-scala/`, `simulator/`, pruebas |
| **Samuel** | `analytics-scala/`, `simulator/` | `ingest-go/`, `docker-compose.yml`, `.github/` |

Una consecuencia que conviene tener presente: **el andamiaje de `ingest-go/` lo escribí yo**, antes
del intercambio. Es el punto de partida de tu módulo, no una especificación. Si algo de cómo está
armado no te gusta, cambialo sin consultarme.

En la tabla de decisiones del `CLAUDE.md` agregué una letra nueva en la columna "Aprueba":

- **A** = ambos, porque toca el contrato compartido o el reparto
- **G** = la decido yo (Scala, análisis, simulador, pruebas)
- **S** = la decidís vos (Go, Docker, CI, deployment)

Antes esa columna estaba escrita con el reparto viejo, así que decisiones del contrato compartido
figuraban como si las aprobaras solo vos. Eso estaba mal en las dos direcciones.

---

## 2. Dónde está el proyecto hoy

| Pieza | Estado |
|---|---|
| `contracts/` | completo: esquemas, ejemplos, catálogo de rechazos y validador en CI |
| `analytics-scala/` | **completo de punta a punta**: modelo, analizadores, reglas y endpoints. 47 pruebas |
| `ingest-go/` | esqueleto: valida el sobre del evento y responde `/health`. **Tuyo** |
| `simulator/` | solo el README con lo que tiene que generar. Mío |
| Docker + compose + CI | funcionando, los cuatro checks en verde. **Tuyos** |

Lo que falta para la demo: tu módulo de Go completo (ingestión, estado por partida, goroutines, el
`POST /analyze` hacia mí) y mi simulador.

---

## 3. Decisiones del contrato (las que nos atan a los dos)

Estas son las que más caro cuesta cambiar después, porque rompen los dos módulos a la vez.

### El sobre del evento es cerrado

`additionalProperties: false` en el esquema y `DisallowUnknownFields` en Go. Un campo que no esté en
el contrato **se rechaza** con `unknown_field`, no se ignora.

El motivo: un campo ignorado en silencio es un error que aparece tres días después, cuando alguien
se pregunta por qué una estadística da cero. Rechazarlo lo convierte en un error inmediato y con
nombre.

Esto me afecta a mí en el simulador: no puedo mandar campos extra "por si acaso".

### `data` es JSON opaco en los dos lados

En Go es `json.RawMessage`; en Scala es `ujson.Value`. Ninguno de los dos núcleos conoce los campos
de ningún juego en particular. Solo el analizador del juego correspondiente abre ese campo.

Es lo que hace que agregar un juego sea agregar un analizador y nada más, que es lo que el enunciado
§12 pide cuando dice que un conjunto nuevo de eventos debe poder incorporarse sin reescribir el
sistema. Si Go conociera `lapTimeMs`, agregar ajedrez te obligaría a tocar tu módulo.

### El orden de los eventos es parte del contrato

El lote llega ordenado por `timestamp` ascendente y **Scala no reordena**. No es un detalle: la
racha de combate y la serie de posiciones de la remontada dependen del orden. Si te llegan eventos
desordenados, ordenarlos antes de mandármelos es tu lado.

### Lo que verifico de tu lado igual

`Batch.parse` comprueba dos cosas que el JSON Schema no puede expresar:

- que el lote traiga **al menos un evento**;
- que **ningún evento sea de otra partida** distinta al `matchId` del lote.

Sé que tu lado ya garantiza las dos. No es desconfianza: si alguna vez se cuela, el análisis
fallaría más adelante con un error mucho más confuso que un 400 que diga cuál evento es el intruso.
Son cuatro líneas.

---

## 4. Decisiones del módulo Scala

### cask + upickle, y no http4s + circe

Era la parte de la decisión 9 que había quedado abierta a propósito en la Fase 0. La cerré así:

| Opción | A favor | En contra |
|---|---|---|
| JDK + JSON a mano | cero dependencias | serializar a mano es tedioso y propenso a errores |
| **cask + upickle** | API mínima, poco código, defendible línea por línea | menos usada en la industria |
| http4s + circe | la combinación estándar del Scala funcional | arrastra la mónada `IO` a todo el borde |
| tapir | endpoints descritos como datos | otra capa de abstracción encima |

**El argumento: el paradigma funcional que el curso evalúa vive en el análisis, no en el
transporte.** Las transformaciones sobre colecciones inmutables son idénticas se reciban los bytes
como se reciban.

**La limitación, que hay que decir en la defensa:** http4s + circe es la opción más funcional de
verdad y la que mejor se vería como demostración del paradigma. cask es un framework de anotaciones,
más cercano a lo imperativo, y está confinado a `Main.scala`. Elegí simplicidad sobre pureza en el
borde, conscientemente, a cambio de tiempo para el análisis, que es donde está la nota. Si te
preguntan "¿por qué no http4s?", esa es la respuesta honesta, no "no lo conocíamos".

De upickle lo que me convenció: deriva los codecs **en tiempo de compilación**. Si alguien renombra
un campo de una case class, el código deja de compilar en vez de producir un JSON equivocado en
ejecución.

### Dos nombres que no coinciden con el contrato, y por qué

`type` y `match` son palabras reservadas en Scala. En el código se llaman `eventType` y
`matchSummary`, y `@upickle.implicits.key(...)` los mapea al nombre del contrato. **Lo que viaja por
el cable no cambia**: vos seguís mandando `"type"` y seguís recibiendo `"match"`.

### `eventType` es `String`, no un `enum`

Un `enum` cerrado obligaría a tocar el núcleo cada vez que un juego nuevo trae un tipo nuevo, que es
justo lo que queremos evitar. Los tipos generales (`MATCH_STARTED`, `PLAYER_JOINED`,
`MATCH_FINISHED`) están en `EventType`; los de cada juego viven en su analizador.

### El estado, sin romper las reglas de estilo

El módulo tiene que recordar lo que analizó para responder los `GET /results/...`. O sea que hay
estado, y el `CLAUDE.md` dice "sin `var`, sin colecciones mutables".

Lo resolví con un `AtomicReference` que apunta a un `Map` **inmutable**: lo único mutable es la
referencia, que se cambia de golpe con `updateAndGet`. No hay `var`, no hay colección mutable, y dos
peticiones simultáneas no pueden dejar el mapa a medias. Creo que es un punto bueno para la defensa,
porque es exactamente el contraste con tu lado: vos manejás estado mutable con goroutines y
channels, yo con una referencia atómica sobre datos inmutables. Mismo problema, dos paradigmas.

Es **en memoria a propósito**: al reiniciar se pierde. Persistir queda fuera del alcance.

### El reloj entra por parámetro

`analizar` recibe una función `ahora: () => String` en vez de llamar a `Instant.now()` por dentro.
Es el mismo patrón que `Config.fromEnv`. Así los analizadores son funciones puras, el resultado es
reproducible y las pruebas no dependen de la hora.

### Las convenciones numéricas viven en un solo archivo

Redondeo a 2 decimales, desviación **poblacional** (divide entre `n`), promedio sin muestras =
`null`. Todo en `Numeros.scala`. Están juntas para que sea imposible redondear distinto en dos
lugares, que es lo que hace fallar las pruebas de integración por un decimal.

---

## 5. Las estadísticas y las reglas que implementé

Las definiciones son las que ya estaban cerradas en el `CLAUDE.md`. Las implementé tal cual y
**verifiqué los resultados contra los ejemplos del contrato calculándolos por fuera**, en Python,
antes de escribir las aserciones — para no estar comprobando mi implementación contra sí misma.
Salen idénticos a los de `contracts/results.md`.

### Carreras

- **Tiempo ajustado** = suma de `lapTimeMs` + `seconds` de las penalizaciones × 1000. Es la
  estadística que cruza dos tipos de evento distintos y la que define al ganador. En el ejemplo, p2
  corre 246 000 ms pero los 6 s de penalización lo mandan al último puesto.
- **Consistencia** = desviación estándar poblacional de los `lapTimeMs`.
- **Adelantamiento** = vuelta en la que la posición mejora respecto a la anterior.
- **Remontada** = el ganador estuvo en la última posición (`position == número de jugadores`) en
  alguna vuelta.

### Combate

- **Las muertes salen del `victimId` de los eventos ajenos.** En `PLAYER_ELIMINATED` el `playerId`
  del sobre es quien elimina; la víctima va en `data`. Es el caso típico de estadística que obliga a
  cruzar el sobre con el `data`.
- **K/D** con 0 muertes devuelve las eliminaciones y marca `kdUndefined: true`, para que quien
  consuma el JSON no confunda "3 eliminaciones sin morir" con "K/D de 3.0".
- **`avgDamage` de quien no eliminó a nadie es `null`, no `0.0`.** No hay muestras, que es distinto
  de un promedio cero.
- **Racha** = 3 eliminaciones en ≤ 10 s sin aparecer como víctima en medio. Reporto solo la primera
  racha de cada jugador: con cuatro eliminaciones rápidas habría dos ventanas solapadas describiendo
  el mismo episodio.
- **Venganza** = A elimina a B y después B elimina a A. Una sola vez por par ordenado.

### Agregado entre partidas

En `GET /results/players/{id}`, `byGame` separa los juegos porque no tiene sentido sumar vueltas de
carreras con eliminaciones de combate.

Una decisión fina: **el K/D agregado se recalcula con los totales**, no se promedian los K/D de cada
partida. Promediarlos daría otro número y sería el equivocado. El daño promedio se pondera por
eliminaciones por la misma razón. Esa lógica vive en cada analizador, no en el almacén, porque solo
el juego sabe qué significa agregar lo suyo.

---

## 6. Decisiones de infraestructura

Estas son tuyas ahora; las listo para que sepas qué heredaste.

- **Puertos:** Go `8080`, Scala `8081`.
- **Dentro de compose los servicios se llaman por nombre** (`http://analytics:8081`), nunca
  `localhost`. Relacionado: `Main.scala` sobrescribe el host a `0.0.0.0`, porque cask escucha en
  `localhost` por defecto y eso compila, pasa todas las pruebas y falla solo al levantar compose,
  con un *connection refused* bastante molesto de diagnosticar. Está comentado en el código para que
  nadie lo quite sin querer.
- **Imágenes multi-stage**, una por módulo. La de Scala pesa 432 MB.
- **CI:** cuatro jobs — `Contrato`, `Go (ingest)`, `Scala (analytics)`, `Imagenes`.
- **`main` protegida** con un ruleset: PR obligatorio, los cuatro checks en verde, sin force push,
  sin borrar la rama, y la *bypass list* vacía para que nos aplique también a nosotros.

### Una decisión que cambié sobre la marcha

El `CLAUDE.md` decía originalmente que mergear requeriría **una aprobación**. Lo saqué: con dos
personas, exigirla deja a cualquiera bloqueado esperando al otro a horas en las que el otro no está.
El ruleset pide PR y CI en verde, pero `Required approvals: 0`.

Pedir revisión sigue siendo lo normal, y para cambios en `contracts/` el acuerdo de los dos sigue
siendo obligatorio — ahí el punto *es* ponerse de acuerdo. Si preferís volver a exigirla, decímelo y
lo cambio.

---

## 7. Cosas que sé que están desactualizadas o pendientes

Te las digo yo para que no las descubras vos:

1. **`contracts/results.md` dice "Estado: propuesta — pendiente de revisión de Samuel"** en el
   encabezado. Eso es de cuando Scala era tuyo. Ahora el módulo es mío y ese documento describe lo
   que ya está implementado. No lo toqué porque el `CLAUDE.md` dice que `contracts/` no se cambia
   sin pedirlo explícitamente. **¿Lo corrijo?**

2. **La fecha de entrega.** El enunciado §14 dice "domingo 18 de Setiembre de 2026", una fecha que
   ya pasó. Sigue sin confirmarse con el profesor.

3. **La lista de entregables del `CLAUDE.md`** no está actualizada con lo que ya se hizo estos días.

4. **Un detalle cosmético:** upickle escribe los `Double` enteros sin el `.0`, así que donde
   `results.md` muestra `"avgLapMs": 82000.0` la salida real dice `82000`. Es el mismo número en
   JSON y ningún parser lo nota. Lo digo por si lo ves y te parece una diferencia.

---

## 8. Lo que necesito de vos

### Decisiones que faltan aprobar

De la tabla del `CLAUDE.md`, lo que sigue en ⬜:

| # | Decisión | Quién |
|---|---|---|
| 1 | Los dos juegos: carreras + combate | ambos |
| 2 | Tipos de evento y campos de `data` por juego | ambos |
| 4 | Go→Scala por HTTP/JSON con `POST /analyze` | ambos |
| 5 | Cuándo envía Go una partida | vos |
| 6 | Casos límite en Go | vos |
| 8 | Puertos | vos |
| 9a | Go 1.24 con solo biblioteca estándar | vos |
| 10 | Repo, CI y deployment | vos |
| 11 | Reparto y ritmo | ambos |

Las 3, 7 y 9b ya están cerradas: son las mías y están implementadas.

La **4** es la que más me urge, porque es nuestra frontera: vos hacés `POST /analyze` con el cuerpo
de `batch.schema.json` y yo te devuelvo el resultado o un 400. Si eso te sirve como está, decilo y
lo marcamos.

### Lo concreto que podés probar ya

Mi módulo funciona solo, sin el tuyo:

```bash
cd analytics-scala
sbt assembly
java -jar target/scala-3.3.8/analytics.jar &
curl -X POST --data-binary @../contracts/examples/combat-match-ok.json http://localhost:8081/analyze
curl http://localhost:8081/results/players/p2
```

Si tu Go manda exactamente eso, ya está integrado.

### Lo que sigue de mi lado

El simulador. Después de eso, la demo depende de que tu módulo esté.
