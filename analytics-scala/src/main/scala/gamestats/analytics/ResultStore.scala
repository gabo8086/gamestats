package gamestats.analytics

import java.util.concurrent.atomic.AtomicReference

/** Resultados de las partidas ya analizadas.
  *
  * El modulo tiene que recordar lo que analizo para poder responder los `GET /results/...`, o sea
  * que hay estado. La forma de tenerlo sin romper las reglas de estilo es esta: el dato guardado es
  * un `Map` **inmutable** y lo unico mutable es la referencia que lo apunta, que se cambia de golpe
  * con `updateAndGet`. No hay `var`, no hay coleccion mutable, y dos peticiones simultaneas no
  * pueden dejar el mapa a medias.
  *
  * Es intencionalmente en memoria: al reiniciar se pierde. Persistir queda fuera del alcance y Go
  * reenvia la partida si hace falta.
  */
final class ResultStore:

  private val partidas = new AtomicReference(Map.empty[String, MatchResult])

  def guardar(resultado: MatchResult): Unit =
    partidas.updateAndGet(actuales => actuales + (resultado.matchId -> resultado))
    ()

  def partida(matchId: String): Option[MatchResult] =
    partidas.get.get(matchId)

  /** Agregado de un jugador a traves de todas las partidas donde aparece.
    *
    * `None` cuando no aparece en ninguna, que es el 404 del contrato. Lo propio de cada juego se
    * lo pide al analizador correspondiente: el almacen no sabe sumar eliminaciones ni vueltas.
    */
  def jugador(playerId: String): Option[PlayerSummary] =
    val suyas = partidas.get.values.toList
      .filter(_.players.exists(_.playerId == playerId))
      .sortBy(_.matchId)

    Option.when(suyas.nonEmpty):
      val porJuego = suyas
        .groupBy(_.gameId)
        .toList
        .sortBy((gameId, _) => gameId)
        .flatMap { (gameId, resultados) =>
          GameAnalyzer.para(gameId).map { analizador =>
            val especificos = resultados.flatMap(_.players.find(_.playerId == playerId)).map(_.gameSpecific)
            GameSummary(gameId, resultados.size, analizador.agregarJugador(especificos))
          }
        }

      PlayerSummary(
        playerId = playerId,
        matchesPlayed = suyas.size,
        wins = suyas.count(_.matchSummary.winnerId.contains(playerId)),
        byGame = porJuego,
        matches = suyas.map(r => MatchRef(r.matchId, r.gameId, r.matchSummary.winnerId.contains(playerId)))
      )
