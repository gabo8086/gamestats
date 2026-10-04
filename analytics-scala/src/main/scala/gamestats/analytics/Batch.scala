package gamestats.analytics

import upickle.default.ReadWriter

/** Lote de una partida terminada. Espejo de `contracts/batch.schema.json`, el cuerpo del
  * `POST /analyze` que manda Go.
  *
  * Los eventos vienen ordenados por `timestamp` ascendente: el contrato dice que ordenarlos es
  * responsabilidad de Go y que Scala no reordena. Varias reglas dependen de ese orden (la racha de
  * combate, la serie de posiciones de la remontada), asi que es una precondicion, no un detalle.
  */
final case class Batch(
    matchId: String,
    gameId: String,
    gameVersion: String,
    events: List[Event]
) derives ReadWriter:

  /** Eventos agrupados por jugador. Los `MATCH_*` no tienen jugador y quedan fuera.
    *
    * `flatMap` sobre la `Option` filtra y desenvuelve en un solo paso: los eventos sin `playerId`
    * desaparecen solos, sin un `.get` que pueda explotar. `groupMap` agrupa y proyecta a la vez,
    * en una sola pasada.
    */
  def porJugador: Map[String, List[Event]] =
    events.flatMap(e => e.playerId.map(_ -> e)).groupMap(_._1)(_._2)

  /** Eventos de un tipo, en el orden en que llegaron. */
  def deTipo(tipo: String): List[Event] = events.filter(_.eventType == tipo)

  /** Jugadores de la partida, segun los `PLAYER_JOINED`. */
  def jugadores: List[String] =
    deTipo(EventType.PlayerJoined).flatMap(_.playerId).distinct

object Batch:

  /** Decodifica el cuerpo del `POST /analyze` y comprueba lo que el esquema no puede garantizar.
    *
    * Go ya valida cada evento, pero dos cosas no son expresables en JSON Schema y romperian el
    * analisis mas adelante de forma confusa: un lote vacio y un evento de otra partida. Verificarlas
    * en el borde cuesta cuatro lineas y convierte un error de datos en un 400 con explicacion.
    */
  def parse(texto: String): Either[String, Batch] =
    Json.decode[Batch](texto).flatMap(coherente)

  private def coherente(lote: Batch): Either[String, Batch] =
    if lote.events.isEmpty then Left("el lote no trae eventos")
    else
      lote.events.find(_.matchId != lote.matchId) match
        case Some(ajeno) =>
          Left(s"el evento ${ajeno.eventId} es de la partida ${ajeno.matchId}, no de ${lote.matchId}")
        case None => Right(lote)
