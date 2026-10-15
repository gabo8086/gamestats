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
  * Se guarda **el resultado y tambien el lote**. Las estadisticas globales necesitan cosas que el
  * resultado no lleva —quien era bot, quien se desconecto— y la alternativa seria inflar el
  * contrato de salida con campos que solo existen para uso interno.
  *
  * Es intencionalmente en memoria: al reiniciar se pierde. Persistir queda fuera del alcance y Go
  * reenvia la partida si hace falta.
  */
final class ResultStore:

  /** Ventana para considerar que una desconexion fue una interrupcion y no un abandono. */
  private val VentanaReconexionMs = 60_000L

  private final case class Analizada(resultado: MatchResult, lote: Batch)

  private val partidas = new AtomicReference(Map.empty[String, Analizada])

  def guardar(resultado: MatchResult, lote: Batch): Unit =
    partidas.updateAndGet(actuales => actuales + (resultado.matchId -> Analizada(resultado, lote)))
    ()

  def partida(matchId: String): Option[MatchResult] =
    partidas.get.get(matchId).map(_.resultado)

  /** Agregado de un jugador a traves de todas las partidas donde aparece.
    *
    * `None` cuando no aparece en ninguna, que es el 404 del contrato. Lo propio de cada juego se
    * lo pide al analizador correspondiente: el almacen no sabe sumar eliminaciones ni vueltas.
    */
  def jugador(playerId: String): Option[PlayerSummary] =
    val suyas = partidas.get.values.toList
      .filter(_.resultado.players.exists(_.playerId == playerId))
      .sortBy(_.resultado.matchId)

    Option.when(suyas.nonEmpty):
      val ganadas = suyas.count(_.resultado.matchSummary.winnerId.contains(playerId))

      val (conBots, entreHumanos) = suyas.partition(a => a.lote.tiposDeJugador.values.exists(_ == EventType.Bot))

      val porJuego = suyas
        .groupBy(_.resultado.gameId)
        .toList
        .sortBy((gameId, _) => gameId)
        .flatMap { (gameId, analizadas) =>
          GameAnalyzer.para(gameId).map { analizador =>
            val especificos =
              analizadas.flatMap(_.resultado.players.find(_.playerId == playerId)).map(_.gameSpecific)
            GameSummary(gameId, analizadas.size, analizador.agregarJugador(especificos))
          }
        }

      PlayerSummary(
        playerId = playerId,
        matchesPlayed = suyas.size,
        wins = ganadas,
        winRate = porcentaje(ganadas, suyas.size),
        vsBots = split(conBots, playerId),
        vsHumans = split(entreHumanos, playerId),
        disconnections = desconexiones(suyas, playerId),
        byGame = porJuego,
        matches = suyas.map(a =>
          MatchRef(a.resultado.matchId, a.resultado.gameId,
                   a.resultado.matchSummary.winnerId.contains(playerId))
        )
      )

  /** Los N mejores de un juego. `None` si ese juego no tiene ninguna partida analizada.
    *
    * Se ordena por victorias y se desempata por porcentaje, tal como fija el contrato. El nombre
    * del jugador entra como ultimo desempate para que el orden sea estable entre llamadas.
    */
  def ranking(gameId: String, top: Int): Option[GameRanking] =
    val delJuego = partidas.get.values.toList.filter(_.resultado.gameId == gameId)

    Option.when(delJuego.nonEmpty):
      val jugadores = delJuego.flatMap(_.resultado.players.map(_.playerId)).distinct

      val filas = jugadores
        .map { id =>
          val suyas   = delJuego.filter(_.resultado.players.exists(_.playerId == id))
          val ganadas = suyas.count(_.resultado.matchSummary.winnerId.contains(id))
          (id, suyas.size, ganadas, porcentaje(ganadas, suyas.size))
        }
        .sortBy((id, _, ganadas, tasa) => (-ganadas, -tasa, id))
        .take(math.max(0, top))

      GameRanking(
        gameId = gameId,
        top = top,
        ranking = filas.zipWithIndex.map { case ((id, jugadas, ganadas, tasa), i) =>
          RankingEntry(i + 1, id, jugadas, ganadas, tasa)
        }
      )

  private def split(analizadas: List[Analizada], playerId: String): SplitPorRival =
    val ganadas = analizadas.count(_.resultado.matchSummary.winnerId.contains(playerId))
    SplitPorRival(analizadas.size, ganadas, porcentaje(ganadas, analizadas.size))

  /** Empareja cada `PLAYER_DISCONNECTED` con el siguiente `PLAYER_RECONNECTED` del mismo jugador.
    *
    * Vuelve dentro de la ventana: reconexion. No vuelve, o vuelve tarde: abandono. Lo segundo es
    * una decision; el diseno solo define "no vuelve", pero alguien que reaparece diez minutos
    * despues abandono la partida igual, y contarlo como interrupcion seria mas enganoso.
    */
  private def desconexiones(analizadas: List[Analizada], playerId: String): Desconexiones =
    val resultados = for
      analizada <- analizadas
      corte     <- analizada.lote.deTipo(EventType.PlayerDisconnected).filter(_.playerId.contains(playerId))
      regreso = analizada.lote
        .deTipo(EventType.PlayerReconnected)
        .filter(_.playerId.contains(playerId))
        .find(r => Tiempo.duracionMs(corte.timestamp, r.timestamp).exists(_ >= 0))
    yield regreso
      .flatMap(r => Tiempo.duracionMs(corte.timestamp, r.timestamp))
      .exists(_ <= VentanaReconexionMs)

    Desconexiones(reconnections = resultados.count(identity), abandons = resultados.count(!_))

  private def porcentaje(cuantos: Int, total: Int): Double =
    if total == 0 then 0.0 else Numeros.dosDecimales(100.0 * cuantos / total)
