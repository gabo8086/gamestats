# Catálogo de eventos

> **Estado: propuesta.** Los cuatro juegos y los campos `matchMode` / `playerType` son las
> decisiones 12 a 15 de la tabla del `CLAUDE.md` y están **pendientes de revisión de Samuel**.

El sobre común está en [`event.schema.json`](event.schema.json) y es **cerrado**: un campo que no
esté en él se rechaza con `unknown_field`. Lo específico de cada juego viaja dentro de `data`, que
el núcleo trata como JSON opaco: Go nunca lo interpreta y en Scala solo lo abre el analizador del
juego correspondiente.

Por eso **agregar un juego es agregar un analizador**, sin tocar el sobre, ni el módulo de
ingestión, ni los otros juegos. Es la respuesta concreta a lo que pide el enunciado §12.

## Eventos generales

Los comparten los cuatro juegos. Son los únicos que el núcleo necesita conocer.

| Tipo | `playerId` | `data` |
|---|---|---|
| `MATCH_STARTED` | ausente o `null` | `{ matchMode, ...propio del juego }` |
| `PLAYER_JOINED` | obligatorio | `{ playerType, botLevel? }` |
| `PLAYER_DISCONNECTED` | obligatorio | `null` |
| `PLAYER_RECONNECTED` | obligatorio | `null` |
| `MATCH_FINISHED` | ausente o `null` | `null` |

### `matchMode` — preparación para Jugador vs Máquina

`MATCH_STARTED.data.matchMode` es `"PVP"` o `"PVE"`.

- `PVP` — todos los participantes son humanos.
- `PVE` — hay al menos un participante con `playerType: "BOT"`.

Hoy `racing` es siempre `PVP`; `combat` puede ser cualquiera de los dos; `blackjack` y
`battleship` son siempre `PVE`, porque el crupier y la flota enemiga son la máquina.

El campo existe para que el modo Jugador vs Máquina **no requiera cambiar el contrato más
adelante**: un juego que hoy es PVP puede pasar a PVE sin tocar el sobre ni los analizadores de los
demás. Este proyecto no implementa la IA; solo deja el diseño listo.

### `playerType` — quién es humano y quién es máquina

`PLAYER_JOINED.data.playerType` es `"HUMAN"` o `"BOT"`. Cuando es `"BOT"` puede traer además
`botLevel`, `"easy"` o `"hard"`.

Un bot **es un jugador como cualquier otro**: tiene `playerId`, aparece en las estadísticas y puede
eliminar o ser eliminado. No es un caso especial en el modelo; es un atributo. Eso es lo que permite
calcular «win rate contra bots» y «K/D contra bots» sin lógica aparte: basta con particionar por
`playerType`.

### Por qué `matchMode` y `playerType` van en `data` y no en el sobre

El sobre es común a **todo** evento y es cerrado. Estos dos campos solo tienen sentido en
`MATCH_STARTED` y en `PLAYER_JOINED` respectivamente; ponerlos en el sobre obligaría a que los otros
quince tipos de evento los llevaran en `null`. Van en `data` aunque los consuma el núcleo de Scala y
no un juego concreto.

### Desconexiones

`PLAYER_DISCONNECTED` y `PLAYER_RECONNECTED` son generales porque cualquier juego puede perder a un
jugador. Scala los usa para distinguir dos situaciones distintas:

- **Reconexión** — vuelve en **≤ 60 s**. Se cuenta como interrupción, no como abandono.
- **Abandono** — nunca aparece el `PLAYER_RECONNECTED` correspondiente.

Los 60 s son la decisión 15 y están propuestos, no cerrados.

---

## `racing` — carreras

`matchMode`: `PVP`. Sin cambios respecto a la Fase 0.

| Tipo | `playerId` | `data` |
|---|---|---|
| `MATCH_STARTED` | — | `{ matchMode, laps, track }` |
| `LAP_COMPLETED` | obligatorio | `{ lap, lapTimeMs, position }` |
| `PENALTY` | obligatorio | `{ seconds, reason }` |

`position` **se deriva** del tiempo acumulado, no se inventa: es el puesto del jugador por tiempo
total al cerrar esa vuelta.

**Por qué estos eventos:** la posición final sola no permite calcular nada interesante. Con la
vuelta, el tiempo y la posición se reconstruye toda la carrera; con `PENALTY` aparece una
estadística que **obliga a cruzar dos tipos de evento distintos** —el tiempo ajustado— y que puede
cambiar quién gana.

---

## `combat` — combate

`matchMode`: `PVP` o `PVE`. Con el requisito nuevo, los bots participan como jugadores: una
eliminación puede tener como autor o como víctima a un bot.

| Tipo | `playerId` | `data` |
|---|---|---|
| `MATCH_STARTED` | — | `{ matchMode, mode, arena }` |
| `PLAYER_ELIMINATED` | **quien elimina** | `{ victimId, weapon, damage }` |

**Ojo con `playerId`:** es quien elimina, y la víctima va en `data.victimId`. Las muertes de un
jugador no salen de sus propios eventos sino de aparecer como víctima en los de otros. Es el caso
típico de estadística que obliga a cruzar el sobre con el `data`.

**Por qué estos eventos:** un solo tipo de evento basta para derivar eliminaciones, muertes, K/D,
daño por arma y las reglas de secuencia, porque lleva a los dos participantes y el arma.

---

## `blackjack` — jugador contra el crupier

`matchMode`: siempre `PVE`. El crupier es un jugador con `playerType: "BOT"` y su propio `playerId`.

| Tipo | `playerId` | `data` |
|---|---|---|
| `MATCH_STARTED` | — | `{ matchMode, decks }` |
| `BET_PLACED` | obligatorio | `{ amount }` |
| `CARD_DEALT` | obligatorio | `{ card, to }` |
| `PLAYER_HIT` | obligatorio | `null` |
| `PLAYER_STAND` | obligatorio | `null` |
| `ROUND_RESULT` | obligatorio | `{ outcome, payout }` |

- `card` — `"AS"`, `"10H"`, `"KD"`. Rango `A 2 3 4 5 6 7 8 9 10 J Q K`, palo `H D C S`
  (corazones, diamantes, tréboles, picas). Patrón: `^(10|[2-9AJQK])[HDCS]$`.
- `to` — el `playerId` de quien **recibe** la carta. El `playerId` del sobre es el jugador cuya
  ronda es; `to` distingue si la carta va a su mano o a la del crupier.
- `outcome` — `WIN`, `LOSE`, `PUSH`, `BLACKJACK`, `BUST`.
- `payout` — entero con signo, el neto para el jugador en esa ronda. Negativo si perdió. El saldo
  neto del jugador es la suma de los `payout`.

**El valor de la mano no viaja en los eventos.** Scala lo reconstruye sumando las cartas con un
`foldLeft`, que es justo el tipo de reconstrucción que pide el enunciado §2. El as vale 11 salvo que
pase de 21, en cuyo caso vale 1 — por eso el fold lleva el total y la cantidad de ases, y no solo
un acumulador.

**Por qué estos eventos:** son los que un crupier real anunciaría. Que el valor de la mano **no**
esté entre ellos es deliberado: si viniera calculado, el análisis sería leer un número en vez de
derivarlo.

---

## `battleship` — hundir la flota, contra la máquina

`matchMode`: siempre `PVE`. La máquina es un jugador con `playerType: "BOT"`.

| Tipo | `playerId` | `data` |
|---|---|---|
| `MATCH_STARTED` | — | `{ matchMode, boardSize, fleet }` |
| `SHOT_FIRED` | **quien dispara** | `{ coord, hit }` |
| `SHIP_SUNK` | **dueño del barco hundido** | `{ ship, size }` |

- `boardSize` — entero, lado del tablero cuadrado. Propuesto: `10`.
- `fleet` — lista de `{ ship, size }` con la flota de cada lado. Propuesta estándar: `carrier` 5,
  `battleship` 4, `cruiser` 3, `submarine` 3, `destroyer` 2.
- `coord` — `"A1"` a `"J10"` en un tablero de 10. Patrón: `^[A-J](10|[1-9])$`.
- `hit` — booleano.

**Ojo con `playerId` en `SHIP_SUNK`:** es el **dueño** del barco que se hundió, no quien disparó.
Así «barcos hundidos por un jugador» se calcula mirando los `SHIP_SUNK` de su rival, igual que las
muertes en combate salen de los eventos ajenos. Mantener la misma convención en los dos juegos evita
una excepción que habría que recordar.

**Por qué estos eventos:** con `SHOT_FIRED` solo se puede calcular precisión; `SHIP_SUNK` agrega el
progreso real y permite cruzar ambos para los tiros por barco hundido.

---

## Resumen de tipos

| Tipo | Juegos |
|---|---|
| `MATCH_STARTED`, `PLAYER_JOINED`, `PLAYER_DISCONNECTED`, `PLAYER_RECONNECTED`, `MATCH_FINISHED` | todos |
| `LAP_COMPLETED`, `PENALTY` | `racing` |
| `PLAYER_ELIMINATED` | `combat` |
| `BET_PLACED`, `CARD_DEALT`, `PLAYER_HIT`, `PLAYER_STAND`, `ROUND_RESULT` | `blackjack` |
| `SHOT_FIRED`, `SHIP_SUNK` | `battleship` |

Diecisiete tipos, de los cuales el núcleo solo conoce cinco.
