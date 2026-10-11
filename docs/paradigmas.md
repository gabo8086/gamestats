# Imperativo y funcional: qué va en cada lado y por qué

Entregable del §13 del enunciado. La pregunta no es cuál paradigma es mejor, sino qué parte del
problema le queda bien a cada uno, y qué se pierde al elegirlo.

## La tesis

GameStats tiene dos mitades con naturalezas opuestas.

**Recibir eventos es un problema de estado que cambia con el tiempo.** Llega un evento suelto, hay
que saber si su partida ya empezó, si ese `eventId` ya se vio, si la partida terminó. La respuesta
depende de todo lo que pasó antes. Eso es estado mutable, y además concurrente: varias partidas
avanzan a la vez. Es el terreno del paradigma imperativo, y por eso va en **Go**.

**Analizar una partida es un problema de transformación de datos.** Cuando el lote llega, ya está
completo y no va a cambiar: es una colección inmutable que hay que convertir en otra cosa —filtrar
los eventos de un tipo, agrupar por jugador, reducir a un número—. No hay estado que evolucione,
hay una tubería de transformaciones. Es el terreno del paradigma funcional, y por eso va en
**Scala**.

## Go: el estado mutable, hecho explícito

El módulo de ingestión mantiene, por cada partida, qué jugadores entraron, qué eventos lleva y si
ya terminó. La decisión de diseño que más dice sobre el paradigma es cómo se protege ese estado:

```go
m = &match{in: make(chan request), done: make(chan struct{})}
d.matches[ev.MatchID] = m
go d.run(ev.MatchID, m)
```

**Una goroutine por partida, con su propio channel.** El estado de una partida lo toca una sola
goroutine, y los demás le hablan mandándole mensajes. No hay candado alrededor del estado de la
partida porque no hace falta: nadie más lo alcanza.

Eso resuelve la concurrencia sin la parte frágil de la concurrencia. El `sync.Mutex` que sí existe
protege únicamente el mapa de `eventId` ya vistos, que es global por definición.

**Ventajas del enfoque imperativo aquí.** El código se lee igual que el enunciado: «si la partida no
empezó, rechazar; si el evento está repetido, rechazar; cuando llegue `MATCH_FINISHED`, enviar».
Modelar eso como transformación pura obligaría a arrastrar el estado como parámetro en cada paso, o
a meter una mónada de estado, y el resultado sería más difícil de leer, no más fácil.

**Limitaciones, y son reales.** Nada en el tipo de `matchState` impide que mañana alguien lo lea
desde otra goroutine; el compilador no lo va a avisar. La disciplina de «solo su goroutine lo toca»
es una convención, no una garantía. Por eso las pruebas corren con `go test -race`, que es
precisamente una herramienta para un problema que el paradigma no previene. Un lenguaje con tipos de
propiedad lo rechazaría en compilación.

## Scala: la inmutabilidad y las transformaciones

El análisis no tiene un solo `var` ni una colección mutable. Las estadísticas se calculan
componiendo operaciones sobre la lista de eventos:

```scala
def porJugador: Map[String, List[Event]] =
  events.flatMap(e => e.playerId.map(_ -> e)).groupMap(_._1)(_._2)
```

`flatMap` sobre la `Option` filtra y desenvuelve en un paso: los eventos sin jugador desaparecen
solos, sin un `.get` que pueda explotar. Donde el código imperativo tendría un bucle, un `if` y un
mapa que se va llenando, acá hay una expresión que se lee como su definición.

El caso más claro es el valor de una mano de blackjack, que **no viaja en los eventos** y hay que
reconstruirlo: un `foldLeft` que acumula el total y la cantidad de ases, porque el as vale 11 salvo
que pase de 21. El enunciado §2 pide justamente eso, reconstruir información en vez de leerla.

**Ventajas.** Cada analizador es una función pura del lote: misma entrada, misma salida, sin orden
de ejecución que recordar. Eso hace las pruebas triviales —no hay nada que montar— y por eso el
módulo tiene 48 de ellas. Y permitió verificar los resultados contra cálculos hechos por fuera, en
Python, en vez de contra la propia implementación.

**Limitaciones, también reales.** La inmutabilidad cuesta memoria: cada `groupBy` construye
colecciones nuevas. Con lotes de decenas o cientos de eventos es irrelevante, pero no lo sería con
millones, y ahí habría que medir antes de defender la elección. Y el estilo tiene un costo de
lectura: un `foldLeft` con acumulador de dos campos es más compacto que un bucle, no más obvio para
quien llega de nuevo.

## El mismo problema, resuelto dos veces

El contraste más útil para la defensa es que **los dos módulos tienen estado mutable** y lo
resuelven distinto.

Scala también necesita recordar lo que analizó, para responder los `GET /results/...`:

```scala
private val partidas = new AtomicReference(Map.empty[String, MatchResult])
def guardar(resultado: MatchResult): Unit =
  partidas.updateAndGet(actuales => actuales + (resultado.matchId -> resultado))
```

Lo único mutable es la **referencia**; lo que guarda es un `Map` inmutable que se reemplaza entero
de golpe. No hay `var`, no hay colección mutable, y dos peticiones simultáneas no pueden dejar el
mapa a medias.

Go aísla el estado mutable **dándoselo a un dueño único**; Scala lo aísla **haciendo que el dato no
cambie** y moviendo la referencia. Las dos son respuestas correctas a «cómo comparto estado sin
corromperlo», y cada una es idiomática de su paradigma.

## Dónde el paradigma cede

Ningún módulo es puro de punta a punta, y conviene decirlo antes de que lo pregunten.

En Scala, el borde HTTP usa **cask**, que es un framework de anotaciones y por tanto más cercano a lo
imperativo que a lo funcional. La alternativa purista era http4s con la mónada `IO`, que habría
llevado los efectos a todo el borde. Se eligió simplicidad sobre pureza **a conciencia**: el
paradigma que el curso evalúa vive en el análisis, no en el transporte, y `cask` está confinado a un
solo archivo. Del mismo modo, upickle lanza excepciones; se atrapan **una sola vez**, en
`Json.decode`, y se convierten en `Either` para que el resto del módulo no las vea.

En Go, el `data` de cada evento se trata como `json.RawMessage` —JSON sin interpretar—, igual que en
Scala es `ujson.Value`. Esa simetría no es casualidad: es lo que permite que agregar un juego sea
agregar un analizador, sin tocar la ingestión.

## Si se hubiera hecho al revés

**El análisis en Go** sería bucles sobre slices y mapas que se van llenando. Funciona, pero cada
estadística quedaría como un procedimiento con su acumulador, en vez de una composición de
operaciones reutilizables, y las pruebas tendrían que preparar y limpiar estado.

**La ingestión en Scala funcional** obligaría a modelar como efectos algo que es inherentemente
secuencial, con estado y concurrente. Se puede, con `IO` y `Ref`, pero se estaría peleando contra el
paradigma para expresar lo que las goroutines y los channels dicen en tres líneas.

La conclusión es la del §13: no hay un paradigma mejor, hay un ajuste entre la forma del problema y
la forma de la herramienta. Lo que este proyecto muestra es que **dentro de un mismo sistema conviven
los dos**, y que la frontera no es arbitraria sino que cae justo donde el problema cambia de
naturaleza: cuando los eventos dejan de llegar y pasan a ser un dato completo que hay que analizar.
