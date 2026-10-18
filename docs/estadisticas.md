# Estadísticas y reglas

Qué calcula GameStats, cómo lo calcula y por qué. Es el entregable del §11 del enunciado
("descripción de las estadísticas y reglas implementadas").

Todo lo que sigue lo produce el módulo de análisis en Scala. El de ingestión en Go no calcula
ninguna estadística de juego: solo valida, agrupa por partida y entrega el lote.

La forma exacta del JSON de salida está en [`../contracts/results.md`](../contracts/results.md); el
catálogo de eventos de los que sale todo, en [`../contracts/events.md`](../contracts/events.md).

## Dónde cae cada requisito del §9

| Punto del enunciado | Dónde se cumple |
|---|---|
| 6. Al menos una estadística agregada **por jugador** | las hay en los cuatro juegos; p. ej. consistencia en carreras |
| 7. Al menos una estadística agregada **por partida** | ganador y duración en los cuatro; p. ej. jugador más letal |
| 8. Al menos una que **combine tipos de evento distintos** | tiempo ajustado en carreras: `LAP_COMPLETED` + `PENALTY` |
| 9. Al menos una **regla de secuencia** | hay una por juego: remontada, racha, tilt y racha de aciertos |

## Convenciones numéricas

Valen para todas las estadísticas y están implementadas en un solo archivo, `Numeros.scala`, para
que sea imposible redondear distinto en dos lugares.

- Los tiempos son **enteros en milisegundos** y los campos terminan en `Ms`.
- Los promedios, desviaciones y razones se **redondean a dos decimales** solo al salir. El cálculo
  interno usa `Double` sin redondear.
- Un promedio **sin muestras es `null`, no `0.0`**. No es lo mismo "no hay datos" que "el promedio
  es cero", y confundirlos haría que un jugador que no disparó nunca pareciera tener puntería nula.
- Las desviaciones son **poblacionales** (se divide entre `n`, no entre `n−1`), porque las vueltas
  de un jugador son toda su población, no una muestra de ella.

---

## Carreras (`racing`)

### Por jugador

| Estadística | Cómo se calcula |
|---|---|
| Mejor vuelta | el menor `lapTimeMs` del jugador |
| Tiempo promedio | media de sus `lapTimeMs` |
| **Consistencia** | desviación estándar poblacional de sus `lapTimeMs` |
| Adelantamientos | vueltas en las que su `position` mejora respecto a la anterior |
| Penalizaciones | suma de `seconds × 1000` de sus `PENALTY` |
| **Tiempo ajustado** | suma de `lapTimeMs` **más** las penalizaciones |

La **consistencia** es la estadística más interesante del juego: un piloto puede tener la vuelta más
rápida y aun así ser poco fiable. Una desviación baja dice que repite el mismo tiempo vuelta a
vuelta, que es lo que gana carreras largas.

### Por partida

Ganador, duración, **vuelta rápida de la carrera** (el menor `lapTimeMs` de todos, con su autor) y
**total de adelantamientos**. El ranking lista a cada jugador con su tiempo ajustado y su
penalización, para que se vea de dónde sale el orden.

### La que combina tipos de evento

**El ganador no es quien suma menos tiempo de vuelta, sino el de menor tiempo ajustado**, y eso
obliga a cruzar `LAP_COMPLETED` con `PENALTY`. En la partida de ejemplo `m-sim-racing-penalty`, p1
es el más rápido en pista pero los 5 s de penalización le dan la victoria a p2. Es el punto 8 del
§9, y se ve en los datos.

### Regla de secuencia: remontada

Un jugador que **estuvo en la última posición** en alguna vuelta —`position` igual al número de
jugadores— **y terminó ganando** por tiempo ajustado.

No se puede detectar mirando un evento: hace falta la serie completa de posiciones del jugador y el
resultado final. El detalle que se emite incluye la peor posición, en qué vuelta ocurrió y la
posición final.

La `position` de cada vuelta no viene inventada: el simulador la **deriva del tiempo acumulado**, de
modo que si un jugador va con menos tiempo que otro, va delante. Sin esa coherencia la remontada no
se podría justificar.

---

## Combate (`combat`)

### Por jugador

| Estadística | Cómo se calcula |
|---|---|
| Eliminaciones | sus `PLAYER_ELIMINATED` |
| Muertes | los `PLAYER_ELIMINATED` **de otros** donde él es `victimId` |
| **K/D** | eliminaciones / muertes |
| Daño promedio | media del `damage` de sus eliminaciones |
| **K/D contra bots** | eliminaciones a bots / muertes a manos de bots |

**Las muertes salen de los eventos ajenos.** En `PLAYER_ELIMINATED` el `playerId` del sobre es quien
elimina y la víctima va en `data.victimId`, así que las muertes de un jugador hay que buscarlas en
los eventos de los demás. Es el caso típico que obliga a cruzar el sobre con el `data`.

**El K/D con cero muertes** no se reporta como infinito ni se inventa: devuelve el número de
eliminaciones y marca `kdUndefined: true`. Así quien consume el JSON no confunde "tres eliminaciones
sin morir" con "un K/D de 3.0", que son cosas distintas.

El **K/D contra bots** no necesitó lógica aparte: se obtiene particionando por el `playerType` que
declara cada `PLAYER_JOINED`. Es para lo que ese campo existe.

### Por partida

Jugador más letal (más eliminaciones, desempate por daño promedio), **primera sangre** (el autor de
la primera eliminación) y eliminaciones y daño promedio **por arma**.

### Reglas de secuencia

**Racha** — tres eliminaciones del mismo jugador en una ventana de **≤ 10 s**, sin que ese jugador
aparezca como víctima en medio. Se reporta solo la primera racha de cada jugador: con cuatro
eliminaciones rápidas habría dos ventanas solapadas describiendo el mismo episodio.

**Venganza** — A elimina a B y más tarde B elimina a A. Se reporta una sola vez por par: si se matan
cinco veces entre ellos, lo interesante es que la venganza ocurrió, no cada repetición.

---

## Blackjack (`blackjack`)

### Por jugador

| Estadística | Cómo se calcula |
|---|---|
| % de victorias | rondas con `outcome` `WIN` o `BLACKJACK` sobre rondas jugadas |
| Saldo neto | suma de los `payout` |
| Apuesta promedio | media de los `amount` |
| % de pasarse de 21 | rondas cuya **mano reconstruida** supera 21 |

### El valor de la mano no viaja en los eventos

Es la decisión de diseño más importante del juego. Los eventos dicen **qué carta se repartió y a
quién**; el total hay que reconstruirlo.

Se calcula con un `foldLeft` cuyo acumulador lleva **dos cosas**: el total y cuántos ases van
contados como 11. Cada as suma 11 de entrada; mientras el total pase de 21 y queden ases sin
degradar, se le restan 10 de a uno. Sin el segundo dato en el acumulador no se podría decidir
cuántos ases bajar.

Si el valor viniera calculado en el evento, el análisis sería leer un número en vez de derivarlo,
que es justo lo que el §2 del enunciado pide evitar.

El **% de pasarse de 21 se deriva de esa mano reconstruida**, no del campo `outcome`. Hay una prueba
que comprueba que las dos vías coinciden en el ejemplo del contrato: es lo que demuestra que el fold
funciona y no está de adorno.

### Por partida

Rondas jugadas y **saldo del crupier**, que es el negativo de lo que ganan los jugadores. El crupier
no aparece en la lista de jugadores porque no apuesta rondas propias.

### Regla de secuencia: tilt

Tras **tres rondas perdidas seguidas** —`LOSE` o `BUST`— la apuesta de la siguiente **sube**. Es un
patrón de comportamiento, no de resultado: describe al jugador que intenta recuperar lo perdido
subiendo la apuesta. Se reporta el primer episodio de cada jugador.

---

## Hundir la flota (`battleship`)

### Por jugador

| Estadística | Cómo se calcula |
|---|---|
| Precisión | disparos con `hit: true` sobre el total de sus disparos |
| Barcos hundidos | los `SHIP_SUNK` cuyo `playerId` es **otro** jugador |

**El `playerId` de `SHIP_SUNK` es el dueño del barco hundido, no quien disparó.** Es la misma
convención que la víctima en combate, elegida así a propósito para no tener dos reglas distintas que
recordar. Con dos bandos se deduce sin ambigüedad quién lo hundió.

### Por partida

Tiros hasta ganar y **promedio de tiros por barco hundido**, los dos del ganador. Gana quien hunde
la flota rival completa; si nadie la termina, no hay ganador, porque coronar al que iba ganando
sería inventar un resultado.

### Regla de secuencia: racha de aciertos

Tres o más disparos consecutivos acertados, sin un fallo en medio. **Se reporta la más larga, no la
primera.** En este juego la racha *es* el episodio: cinco aciertos seguidos y los tres primeros de
esos cinco describen lo mismo, y cortar en tres contaría de menos.

Es la diferencia con la racha de combate, donde la ventana de 10 s define el episodio y por eso allí
sí se reporta la primera.

---

## Estadísticas de plataforma

No pertenecen a ningún juego, así que se calculan **una sola vez** para los cuatro.

### Las que salen solo del sobre

El `GenericAnalyzer` **no conoce ningún juego** y nunca abre el campo `data`. Con eso produce, en
cada partida:

- **Distribución por tipo de evento**, en orden de aparición, que se lee como la línea de tiempo.
- **Actividad por minuto** desde el arranque. Un minuto sin eventos sale con `0`, no se omite: una
  serie con huecos no se puede graficar directo.
- **Jugadores vistos**, los que aparecen en algún evento.

Que esto funcione sin saber de qué juego se trata es la demostración concreta de que el sobre común
alcanza: sabe contar `BET_PLACED` y `SHOT_FIRED` sin que nadie le haya enseñado qué son, y un juego
nuevo trae estas métricas sin escribir una línea.

### Perfil global del jugador

Partidas jugadas, victorias y porcentaje **a través de todos los juegos**, más:

- **Contra bots frente a contra humanos.** Una partida cuenta como "contra bots" si tuvo al menos un
  participante con `playerType: "BOT"`. Los dos cortes siempre suman el total.
- **Desconexiones.** Un `PLAYER_DISCONNECTED` seguido de `PLAYER_RECONNECTED` del mismo jugador
  dentro de **60 s** es una reconexión; si no vuelve, o vuelve más tarde, es un abandono. Que volver
  tarde cuente como abandono es una decisión nuestra: alguien que reaparece diez minutos después
  abandonó la partida igual.

### Agregado por juego

Lo específico de cada juego se agrega **dentro del mismo juego**, nunca entre juegos: no tiene
sentido sumar vueltas de carreras con eliminaciones de combate.

Cada analizador decide cómo se agrega lo suyo, y ahí hay un detalle que importa: **los cocientes se
recalculan con los totales, no se promedian**. El K/D de alguien con 4/1 en una partida y 1/3 en
otra es 5/4 = 1.25, no el promedio de 4.0 y 0.33. Lo mismo con la precisión en battleship y con los
porcentajes de blackjack, que se ponderan por rondas jugadas.

### Ranking por juego

Los mejores de un juego, ordenados por victorias y desempatados por porcentaje.

---

## Cómo comprobarlo

Todo lo anterior está cubierto por **116 pruebas** en el módulo de análisis. Los valores esperados
de las partidas de ejemplo se calcularon **por fuera del código**, en Python, antes de escribir las
aserciones, para no comprobar la implementación contra sí misma.

Para verlo corriendo, [`demo.md`](demo.md) tiene el guion paso a paso con comandos ya verificados.
