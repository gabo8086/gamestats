# Guion de la demo

Ocho pasos, unos **12 minutos**. Todos los comandos y los números de este documento salieron de un
ensayo real contra la imagen publicada, bajada sin credenciales.

Lo único que hace falta es **Docker abierto**. Nada de Go, Scala ni Python instalados.

## Antes de empezar

Bajá la imagen **antes** de la presentación: la primera vez tarda unos 30 s y no hay por qué gastarlos
en vivo.

```bash
docker pull ghcr.io/gabo8086/gamestats:v1.3
```

Tené dos ventanas listas: una terminal y un navegador. **Usá Firefox**, que formatea el JSON solo;
si vas a mostrar resultados en la terminal, mirá antes la sección
"[Ver los resultados de forma legible](#ver-los-resultados-de-forma-legible)".

---

## 1. Un comando levanta todo el sistema

```bash
docker run -d --name gamestats -p 8080:8080 -p 8081:8081 ghcr.io/gabo8086/gamestats:v1.3
curl http://localhost:8080/health
curl http://localhost:8081/health
```

**Qué decir:** son dos servicios independientes —ingestión en Go y análisis en Scala— empaquetados en
una imagen para que evaluarlo sea un comando. Con los dos `/health` respondiendo ya está arriba.

Con la imagen ya bajada, responden casi de inmediato.

## 2. La ingestión valida, y rechaza con motivo

```bash
docker exec gamestats gamestats ejemplos
```

**Qué mostrar:** manda las cinco partidas de ejemplo del contrato y después **un evento inválido por
cada razón de rechazo**. Las diez líneas del final muestran que cada uno devuelve exactamente el
código que el contrato promete.

**Qué decir:** Go nunca descarta un evento en silencio. Cada rechazo lleva su razón, y el catálogo
está en `contracts/rejections.md`. Esto es el §6.1 del enunciado: validar y tratar adecuadamente lo
inválido.

## 3. Varias partidas a la vez

```bash
docker exec gamestats gamestats simulator -no-files
```

**Qué mostrar:** tarda unos 12 segundos y manda 234 eventos de 12 partidas de los cuatro juegos, con
los `matchId` **intercalados**.

**Qué decir:** esto es lo que le da trabajo concurrente a Go. Cada partida vive en su propia
goroutine y un despachador enruta por `matchId`. El simulador también genera cada partida en una
goroutine, así que la concurrencia aparece en los dos lados.

## 4. Las métricas de la ingestión

```bash
curl http://localhost:8080/metrics
```

**Qué mostrar:** `eventsReceived`, `eventsValid`, `eventsInvalid`, el desglose `rejectionsByReason`
con los nueve códigos, y `activeMatches`.

**Si preguntan por `activeMatches: 2`:** son las partidas que abrieron los casos inválidos y que
nunca reciben `MATCH_FINISHED`. No es un error: es el estado por partida que Go mantiene, visible.

**Qué decir:** son métricas de la **ingestión**, no de los juegos. Go sigue sin saber qué es una
vuelta ni una eliminación.

## 5. Una regla de secuencia por juego

```bash
curl http://localhost:8081/results/matches/m-sim-racing-comeback
curl http://localhost:8081/results/matches/m-sim-combat-streak
curl http://localhost:8081/results/matches/m-sim-blackjack-tilt
curl http://localhost:8081/results/matches/m-sim-battleship-streak
```

| Partida | Gana | Regla |
|---|---|---|
| `m-sim-racing-comeback` | p3 | `comeback` |
| `m-sim-combat-streak` | p3 | `streak` |
| `m-sim-blackjack-tilt` | p1 | `tilt` |
| `m-sim-battleship-streak` | p1 | `hitStreak` |

**Qué decir:** ninguna se calcula mirando un evento aislado; todas necesitan una secuencia. Es el
punto 9 del §9 del enunciado.

**El mejor momento para detenerse es blackjack:** el valor de cada mano **no viaja en los eventos**.
Scala lo reconstruye con un `foldLeft` que acumula el total y la cantidad de ases, porque el as vale
11 salvo que la mano se pase. Si el valor viniera dado, no habría análisis que defender.

Si queda tiempo, `m-sim-racing-penalty` muestra que el ganador combina dos tipos de evento: p1 es el
más rápido pero los 5 s de penalización le dan la victoria a p2.

## 6. Lo que funciona sin conocer el juego

```bash
curl http://localhost:8081/results/matches/m-sim-racing-comeback   # mirar el bloque "generic"
curl http://localhost:8081/results/matches/m-sim-blackjack-tilt    # el mismo bloque
```

**Qué mostrar:** las dos partidas traen `eventsByType`, `activityPerMinute` y `playersSeen`, y en
blackjack aparecen tipos como `BET_PLACED` que el `GenericAnalyzer` nunca conoció.

**Qué decir:** este analizador usa **solo el sobre común** y jamás abre el campo `data`. Por eso un
juego nuevo trae estas métricas gratis. Es la prueba concreta de que el núcleo es genérico, que es lo
que pide el §12.

## 7. Estadísticas que cruzan partidas y juegos

```bash
curl http://localhost:8081/results/players/p1
curl http://localhost:8081/results/players/p4
curl http://localhost:8081/results/players/p5
```

**Qué mostrar en p1:** juega los **cuatro** juegos, y el perfil separa el win rate contra bots del de
contra humanos.

**Qué decir:** un bot es un jugador con un atributo, `playerType`, no un caso especial del modelo.
Por eso ese corte sale de particionar, sin lógica aparte. Es la preparación para Jugador vs Máquina:
el contrato ya la soporta aunque la IA no esté implementada.

**p4 y p5** muestran los dos lados de una desconexión: p4 vuelve dentro de los 60 s y cuenta como
reconexión; p5 no vuelve y cuenta como abandono.

```bash
curl "http://localhost:8081/results/games/combat/ranking?top=3"
```

## 8. Los dos paradigmas, que es lo que se evalúa

No hace falta mostrar código corriendo; alcanza con abrir dos archivos.

**Go — estado mutable, hecho explícito.** En `ingest-go/internal/ingest/dispatcher.go`:

```go
m = &match{in: make(chan request), done: make(chan struct{})}
go d.run(ev.MatchID, m)
```

Una goroutine por partida con su propio channel. El estado lo toca una sola goroutine, así que no
hace falta candado: nadie más lo alcanza.

**Scala — el dato no cambia.** En `analytics-scala/.../ResultStore.scala`:

```scala
private val partidas = new AtomicReference(Map.empty[String, MatchResult])
```

Lo único mutable es la referencia; lo que guarda es un `Map` inmutable que se reemplaza entero.

**El remate:** los dos módulos tienen estado mutable y lo resuelven distinto. Go lo aísla dándoselo a
un dueño único; Scala lo aísla haciendo que el dato no cambie. Dos respuestas correctas a la misma
pregunta, cada una idiomática de su paradigma. El desarrollo está en
[`paradigmas.md`](paradigmas.md), con las limitaciones de cada enfoque.

---

## Ver los resultados de forma legible

El contrato es JSON, y el enunciado §6.2 lo nombra como el formato esperado: es lo que permite que
Go y Scala se hablen y que las pruebas comparen valores exactos. Pero un JSON de cuarenta líneas en
pantalla no se lee bien en vivo, así que para la presentación conviene tener esto a mano.

**En el navegador**, usá **Firefox**: formatea el JSON solo, con secciones plegables. Chrome lo
muestra como un bloque de texto a menos que tengas una extensión.

**En la terminal**, los comandos de abajo corren **dentro del contenedor**, así que no hace falta
instalar nada: solo Docker. Están verificados.

Formatear cualquier respuesta:

```bash
docker exec gamestats sh -c "curl -s localhost:8081/results/matches/m-racing-001 | python3 -m json.tool"
```

Una partida como tabla, con sus reglas:

```bash
docker exec gamestats python3 -c "
import json, urllib.request
d = json.load(urllib.request.urlopen('http://127.0.0.1:8081/results/matches/m-racing-001'))
print(d['gameId'], d['matchId'], '| gana:', d['match']['winnerId'])
for p in d['players']:
    print(' ', p['playerId'], p['gameSpecific'])
for r in d['patterns']:
    print('  regla:', r['rule'], '->', r['playerId'])
"
```

Las cuatro reglas de un vistazo, un juego por línea:

```bash
docker exec gamestats python3 -c "
import json, urllib.request
for m in ['m-racing-001','m-combat-001','m-blackjack-001','m-battleship-001']:
    d = json.load(urllib.request.urlopen('http://127.0.0.1:8081/results/matches/' + m))
    reglas = ', '.join(p['rule'] + ' (' + p['playerId'] + ')' for p in d['patterns']) or '-'
    print('%-12s %-20s gana %-5s  %s' % (d['gameId'], d['matchId'], d['match']['winnerId'], reglas))
"
```

Las métricas de la ingestión, con el desglose de rechazos:

```bash
docker exec gamestats python3 -c "
import json, urllib.request
d = json.load(urllib.request.urlopen('http://127.0.0.1:8080/metrics'))
print('recibidos %d | validos %d | invalidos %d | lotes %d | activas %d' % (
    d['eventsReceived'], d['eventsValid'], d['eventsInvalid'], d['batchesSent'], d['activeMatches']))
for razon, n in sorted(d['rejectionsByReason'].items()):
    if n: print('  %-30s %d' % (razon, n))
"
```

El perfil de un jugador, con el corte contra bots y las desconexiones:

```bash
docker exec gamestats python3 -c "
import json, urllib.request
d = json.load(urllib.request.urlopen('http://127.0.0.1:8081/results/players/p1'))
print('%s: %d partidas, %d victorias (%s%%)' % (d['playerId'], d['matchesPlayed'], d['wins'], d['winRate']))
print('  contra bots    %d/%d' % (d['vsBots']['wins'], d['vsBots']['matchesPlayed']))
print('  contra humanos %d/%d' % (d['vsHumans']['wins'], d['vsHumans']['matchesPlayed']))
print('  desconexiones  %d reconexiones, %d abandonos' % (d['disconnections']['reconnections'], d['disconnections']['abandons']))
for g in d['byGame']:
    print('  %-12s %d partidas  %s' % (g['gameId'], g['matchesPlayed'], g['gameSpecific']))
"
```

El ranking de un juego:

```bash
docker exec gamestats python3 -c "
import json, urllib.request
d = json.load(urllib.request.urlopen('http://127.0.0.1:8081/results/games/combat/ranking?top=5'))
print('ranking de', d['gameId'])
for r in d['ranking']:
    print('  %d. %-6s %d de %d  (%s%%)' % (r['rank'], r['playerId'], r['wins'], r['matchesPlayed'], r['winRate']))
"
```

**Si preguntan por qué no hay una interfaz gráfica:** el §6.2 pide un formato estructurado y nombra
JSON; "mostrados en una interfaz" es una de tres alternativas de uso posterior, unidas por "o", y el
§9.10 pide "consultar **o** presentar". Los endpoints son el "consultar". Una interfaz sería trabajo
sobre el entregable, no parte de él.

## Cerrar

```bash
docker rm -f gamestats
```

## Si algo falla en vivo

| Síntoma | Qué hacer |
|---|---|
| `/health` no responde | Scala tarda más en arrancar que Go. Esperar unos segundos y reintentar |
| El puerto está ocupado | `docker rm -f gamestats` y volver a levantar |
| No baja la imagen | Es pública y no necesita login; comprobar la red. Plan B: `docker compose up --build` desde el repo |
| Un resultado sale distinto | El simulador es determinista con la misma semilla; si se corrió dos veces, los acumulados por jugador crecen. Reiniciar el contenedor deja todo limpio |

**Importante:** los números por jugador y el ranking dependen de cuántas veces se haya corrido el
simulador, porque el almacén acumula. Las partidas individuales y sus reglas no cambian. Si querés
números idénticos a los de este guion, reiniciá el contenedor antes de empezar.

## Preguntas probables

**¿Por qué dos lenguajes?** Porque las dos mitades tienen naturaleza distinta: recibir eventos es
estado que cambia en el tiempo, analizarlos es transformar una colección que ya no cambia. Está
desarrollado en [`paradigmas.md`](paradigmas.md).

**¿Cómo se agrega un juego?** Un archivo nuevo en `analytics-scala` y una línea en el registro de
`GameAnalyzer`. Pasamos de dos juegos a cuatro sin tocar el contrato, ni Go, ni los analizadores
existentes.

**¿Y si el evento viene mal?** Lo rechaza con su razón y queda contado en `/metrics`. Se mostró en el
paso 2.

**¿Dónde están las estadísticas definidas?** En [`diseno.md`](diseno.md), con la definición exacta de
cada una, y la forma de la salida en [`../contracts/results.md`](../contracts/results.md).
