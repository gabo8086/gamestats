package gamestats.analytics

import upickle.default.ReadWriter
import upickle.implicits.key

/** Sobre de resultados, espejo de `contracts/results.md`.
  *
  * El sobre es igual para todos los juegos y lo propio de cada uno va dentro de `gameSpecific`,
  * que es `ujson.Obj` o sea JSON armado por el analizador. Es la misma idea que `data` en el
  * evento, en el sentido contrario: agregar un juego es agregar un `GameAnalyzer` que llene
  * `gameSpecific`, sin tocar ni el sobre ni los endpoints.
  *
  * `matchSummary` se llama asi y no `match` porque `match` es palabra reservada en Scala; igual que
  * con `type` en el evento, `@key` lo mapea al nombre del contrato.
  */
final case class MatchResult(
    matchId: String,
    gameId: String,
    gameVersion: String,
    analyzedAt: String,
    eventCount: Int,
    @key("match") matchSummary: MatchSummary,
    players: List[PlayerResult],
    patterns: List[Pattern]
) derives ReadWriter

final case class MatchSummary(
    startedAt: Option[String],
    finishedAt: Option[String],
    durationMs: Option[Long],
    playerCount: Int,
    winnerId: Option[String],
    gameSpecific: ujson.Obj
) derives ReadWriter

final case class PlayerResult(
    playerId: String,
    gameSpecific: ujson.Obj
) derives ReadWriter

/** Un patron detectado. Es una lista y no un booleano porque un mismo jugador puede disparar varias
  * reglas y porque agregar una regla no cambia la forma del JSON.
  */
final case class Pattern(
    rule: String,
    playerId: String,
    detail: ujson.Obj
) derives ReadWriter

/** Ayudas para armar los `gameSpecific`.
  *
  * `ujson.Obj` acepta `Double`, `Int` o `String` directamente, pero no `Option`. El contrato pide
  * `null` cuando no hay muestras (`avgDamage` de quien no elimino a nadie), asi que estas tres
  * convierten `None` en `ujson.Null` en vez de inventar un cero.
  */
object Js:
  def num(valor: Option[Double]): ujson.Value  = valor.fold[ujson.Value](ujson.Null)(ujson.Num(_))
  def entero(valor: Option[Long]): ujson.Value = valor.fold[ujson.Value](ujson.Null)(v => ujson.Num(v.toDouble))
  def texto(valor: Option[String]): ujson.Value = valor.fold[ujson.Value](ujson.Null)(ujson.Str(_))

  /** Lee un numero de un `gameSpecific` ya calculado, para poder agregarlo entre partidas.
    * Devuelve `None` si la clave no esta o si trae `null`.
    */
  def leerNum(obj: ujson.Obj, clave: String): Option[Double] =
    obj.value.get(clave).flatMap(_.numOpt)

/** Agregado de un jugador a traves de las partidas ya analizadas (`GET /results/players/{id}`).
  *
  * `byGame` existe porque no tiene sentido sumar vueltas de carreras con eliminaciones de combate:
  * lo agregado solo se puede comparar dentro del mismo juego.
  */
final case class PlayerSummary(
    playerId: String,
    matchesPlayed: Int,
    wins: Int,
    byGame: List[GameSummary],
    matches: List[MatchRef]
) derives ReadWriter

final case class GameSummary(
    gameId: String,
    matchesPlayed: Int,
    gameSpecific: ujson.Obj
) derives ReadWriter

final case class MatchRef(matchId: String, gameId: String, winner: Boolean) derives ReadWriter

/** Cuerpo de error de los endpoints. */
final case class ErrorBody(error: String) derives ReadWriter
