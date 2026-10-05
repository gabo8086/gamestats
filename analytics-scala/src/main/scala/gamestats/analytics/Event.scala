package gamestats.analytics

import upickle.default.ReadWriter
import upickle.implicits.key

/** Sobre comun de un evento. Es el espejo en Scala de `contracts/event.schema.json`.
  *
  * Tres decisiones que conviene poder defender:
  *
  *   - `eventType` se llama asi y no `type` porque `type` es palabra reservada en Scala.
  *     `@key("type")` lo mapea al nombre del contrato, que es el que viaja por el cable.
  *   - `data` queda como `ujson.Value`, o sea JSON sin interpretar, igual que el `json.RawMessage`
  *     del lado de Go. El nucleo no conoce los campos de cada juego; solo el analizador
  *     correspondiente los abre. Es lo que permite agregar un juego sin tocar este archivo.
  *   - `eventType` es `String` y no un `enum`. Un `enum` cerrado obligaria a modificar el nucleo
  *     cada vez que un juego nuevo trae un tipo nuevo, que es justo lo que el enunciado pide evitar.
  *     Los tipos generales, los que si comparten todos los juegos, estan en `EventType`.
  */
final case class Event(
    eventId: String,
    timestamp: String,
    gameId: String,
    gameVersion: String,
    matchId: String,
    playerId: Option[String] = None,
    @key("type") eventType: String,
    action: Option[String] = None,
    data: ujson.Value = ujson.Null
) derives ReadWriter:

  /** Un campo de `data`, si `data` es un objeto y si el campo esta.
    *
    * Acceder a `data("lap")` directamente lanza cuando falta la clave. Estos cuatro accesores
    * devuelven `Option` para que los analizadores compongan con `flatMap` y `for` en vez de
    * atrapar excepciones.
    */
  def field(nombre: String): Option[ujson.Value] =
    data.objOpt.flatMap(_.get(nombre))

  def str(nombre: String): Option[String] = field(nombre).flatMap(_.strOpt)

  def num(nombre: String): Option[Double] = field(nombre).flatMap(_.numOpt)

  def int(nombre: String): Option[Int] = num(nombre).map(_.toInt)

  def long(nombre: String): Option[Long] = num(nombre).map(_.toLong)

/** Tipos de evento comunes a todos los juegos, los unicos que el nucleo necesita conocer.
  *
  * Los especificos de cada juego (`LAP_COMPLETED`, `PENALTY`, `PLAYER_ELIMINATED`) viven en el
  * analizador de su juego, no aqui.
  */
object EventType:
  val MatchStarted: String  = "MATCH_STARTED"
  val PlayerJoined: String  = "PLAYER_JOINED"
  val MatchFinished: String = "MATCH_FINISHED"

  /** Los eventos `MATCH_*` son los que pueden venir sin `playerId`. */
  def esDePartida(tipo: String): Boolean = tipo.startsWith("MATCH_")
