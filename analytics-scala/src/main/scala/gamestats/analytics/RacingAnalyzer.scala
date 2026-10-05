package gamestats.analytics

/** Analizador de carreras.
  *
  * Las definiciones son las cerradas en el CLAUDE.md:
  *
  *   - **tiempo ajustado** = suma de `lapTimeMs` + `seconds` de las penalizaciones x 1000. Es la
  *     estadistica que combina dos tipos de evento distintos, y es la que define al ganador.
  *   - **consistencia** = desviacion estandar poblacional de los `lapTimeMs` del jugador.
  *   - **adelantamiento** = vuelta en la que la posicion mejora respecto a la vuelta anterior.
  *   - **remontada** = el ganador estuvo en la ultima posicion (`position == numero de jugadores`)
  *     en alguna vuelta.
  */
object RacingAnalyzer extends GameAnalyzer:

  val gameId: String = "racing"

  private val LapCompleted = "LAP_COMPLETED"
  private val Penalty      = "PENALTY"

  /** Lo que se calcula una sola vez por jugador y se reusa en el resumen, en el ranking y en los
    * patrones. Se construye del lote, no se muta.
    */
  private final case class Corredor(playerId: String, vueltas: List[Event], penaltyMs: Long):
    val tiemposMs: List[Long]  = vueltas.flatMap(_.long("lapTimeMs"))
    val posiciones: List[Int]  = vueltas.flatMap(_.int("position"))
    val adjustedTimeMs: Long   = tiemposMs.sum + penaltyMs
    val mejorVueltaMs: Option[Long] = tiemposMs.minOption

    private val tiemposDouble: List[Double] = tiemposMs.map(_.toDouble)
    val promedioVueltaMs: Option[Double]    = Numeros.promedio(tiemposDouble)
    val consistenciaMs: Option[Double]      = Numeros.desviacionPoblacional(tiemposDouble)

    /** Una vuelta cuenta como adelantamiento cuando la posicion mejora respecto a la anterior.
      * `sliding(2)` recorre la serie de posiciones por pares consecutivos.
      */
    val adelantamientos: Int =
      posiciones.sliding(2).count {
        case List(previa, actual) => actual < previa
        case _                    => false
      }

    val posicionFinal: Option[Int] = posiciones.lastOption

    /** La peor posicion y la vuelta en que ocurrio, para el detalle de la remontada. */
    val peorVuelta: Option[Event] =
      vueltas.filter(_.int("position").isDefined).maxByOption(_.int("position").getOrElse(0))

  private def corredores(lote: Batch): List[Corredor] =
    val penalizaciones: Map[String, Long] =
      lote
        .deTipo(Penalty)
        .flatMap(e => e.playerId.map(_ -> e.long("seconds").getOrElse(0L) * 1000))
        .groupMapReduce(_._1)(_._2)(_ + _)

    val vueltasPorJugador: Map[String, List[Event]] =
      lote.deTipo(LapCompleted).flatMap(e => e.playerId.map(_ -> e)).groupMap(_._1)(_._2)

    lote.jugadores
      .map(id => Corredor(id, vueltasPorJugador.getOrElse(id, Nil), penalizaciones.getOrElse(id, 0L)))
      .sortBy(_.adjustedTimeMs)

  /** El ganador es el del menor tiempo ajustado. Quien no completo ninguna vuelta tiene tiempo
    * ajustado 0, que lo dejaria primero por accidente, asi que no entra en la disputa.
    */
  protected def ganador(lote: Batch): Option[String] =
    corredores(lote).filter(_.tiemposMs.nonEmpty).minByOption(_.adjustedTimeMs).map(_.playerId)

  protected def resumenDelJuego(lote: Batch): ujson.Obj =
    val arranque = lote.deTipo(EventType.MatchStarted).headOption

    ujson.Obj(
      "track" -> Js.texto(arranque.flatMap(_.str("track"))),
      "laps"  -> Js.entero(arranque.flatMap(_.long("laps"))),
      "ranking" -> ujson.Arr.from(
        corredores(lote).filter(_.tiemposMs.nonEmpty).map { c =>
          ujson.Obj(
            "playerId"       -> c.playerId,
            "adjustedTimeMs" -> c.adjustedTimeMs.toDouble,
            "penaltyMs"      -> c.penaltyMs.toDouble
          )
        }
      )
    )

  /** Los jugadores salen en el orden del ranking, que es como se leen unos resultados de carrera. */
  protected def jugadores(lote: Batch): List[PlayerResult] =
    corredores(lote).map { c =>
      PlayerResult(
        playerId = c.playerId,
        gameSpecific = ujson.Obj(
          "bestLapMs"      -> Js.entero(c.mejorVueltaMs),
          "avgLapMs"       -> Js.num(c.promedioVueltaMs.map(Numeros.dosDecimales)),
          "consistencyMs"  -> Js.num(c.consistenciaMs.map(Numeros.dosDecimales)),
          "overtakes"      -> c.adelantamientos,
          "positions"      -> ujson.Arr.from(c.posiciones),
          "adjustedTimeMs" -> c.adjustedTimeMs.toDouble
        )
      )
    }

  /** Entre partidas: la mejor vuelta es la mejor de todas, el promedio es el de los promedios y
    * los adelantamientos se suman. El tiempo ajustado no se agrega: sumar carreras distintas no
    * significa nada.
    */
  def agregarJugador(porPartida: List[ujson.Obj]): ujson.Obj =
    ujson.Obj(
      "bestLapMs"  -> Js.num(porPartida.flatMap(Js.leerNum(_, "bestLapMs")).minOption),
      "avgLapMs"   -> Js.num(Numeros.promedio(porPartida.flatMap(Js.leerNum(_, "avgLapMs"))).map(Numeros.dosDecimales)),
      "overtakes"  -> porPartida.flatMap(Js.leerNum(_, "overtakes")).sum
    )

  /** Remontada: el ganador estuvo en la ultima posicion en alguna vuelta.
    *
    * La ultima posicion es `position == numero de jugadores`. Se mira solo al ganador porque la
    * definicion del CLAUDE.md exige las dos cosas a la vez: haber estado ultimo y haber ganado.
    */
  protected def patrones(lote: Batch): List[Pattern] =
    val total = lote.jugadores.size
    val todos = corredores(lote)

    for
      campeonId <- ganador(lote).toList
      campeon   <- todos.filter(_.playerId == campeonId)
      if campeon.posiciones.contains(total)
      peor <- campeon.peorVuelta.toList
    yield Pattern(
      rule = "comeback",
      playerId = campeonId,
      detail = ujson.Obj(
        "worstPosition"    -> Js.entero(peor.long("position")),
        "worstPositionLap" -> Js.entero(peor.long("lap")),
        "finalPosition"    -> Js.entero(campeon.posicionFinal.map(_.toLong))
      )
    )
