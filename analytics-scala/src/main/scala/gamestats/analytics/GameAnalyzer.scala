package gamestats.analytics

/** Un analizador por juego.
  *
  * El sobre del resultado se arma una sola vez, aqui, en `analizar`. Cada juego solo aporta lo
  * suyo: quien gano, que va en el `gameSpecific` de la partida, que va en el de cada jugador y que
  * patrones disparo. Agregar un juego nuevo es escribir un objeto que extienda este trait y
  * registrarlo abajo: no se toca ni el sobre, ni los endpoints, ni los otros juegos. Es lo que el
  * enunciado §12 pide cuando dice que un conjunto nuevo de eventos debe poder incorporarse sin
  * reescribir el sistema.
  *
  * Todos los metodos son funciones puras del lote. `analizar` recibe el reloj como parametro en vez
  * de llamar a `Instant.now()` por dentro, el mismo patron que `Config.fromEnv`: asi el resultado
  * es reproducible y las pruebas no dependen de la hora.
  */
trait GameAnalyzer:

  def gameId: String

  protected def ganador(lote: Batch): Option[String]

  protected def resumenDelJuego(lote: Batch): ujson.Obj

  protected def jugadores(lote: Batch): List[PlayerResult]

  protected def patrones(lote: Batch): List[Pattern]

  /** Combina los `gameSpecific` que un mismo jugador obtuvo en varias partidas de este juego.
    *
    * Vive aqui y no en el almacen porque solo el juego sabe que significa agregar lo suyo: en
    * carreras la mejor vuelta es un minimo y los adelantamientos una suma; en combate el K/D hay
    * que recalcularlo con los totales, no promediar los K/D de cada partida.
    */
  def agregarJugador(porPartida: List[ujson.Obj]): ujson.Obj

  final def analizar(lote: Batch, ahora: () => String): MatchResult =
    val inicio = lote.deTipo(EventType.MatchStarted).headOption.map(_.timestamp)
    val fin    = lote.deTipo(EventType.MatchFinished).lastOption.map(_.timestamp)

    MatchResult(
      matchId = lote.matchId,
      gameId = lote.gameId,
      gameVersion = lote.gameVersion,
      analyzedAt = ahora(),
      eventCount = lote.events.size,
      matchSummary = MatchSummary(
        startedAt = inicio,
        finishedAt = fin,
        durationMs = for
          a <- inicio
          b <- fin
          d <- Tiempo.duracionMs(a, b)
        yield d,
        playerCount = lote.jugadores.size,
        winnerId = ganador(lote),
        gameSpecific = resumenDelJuego(lote)
      ),
      players = jugadores(lote),
      patterns = patrones(lote)
    )

object GameAnalyzer:

  /** El registro. Agregar un juego es agregar una linea aqui. */
  private val registrados: Map[String, GameAnalyzer] =
    List(RacingAnalyzer, CombatAnalyzer).map(a => a.gameId -> a).toMap

  def para(gameId: String): Option[GameAnalyzer] = registrados.get(gameId)

  def conocidos: List[String] = registrados.keys.toList.sorted
