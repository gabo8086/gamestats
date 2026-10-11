package gamestats.analytics

/** Analizador de hundir la flota: el jugador contra la maquina.
  *
  * Lo propio de este juego es que `SHIP_SUNK` lleva en el `playerId` al **dueno** del barco
  * hundido, no a quien disparo. Es la misma convencion que la victima en `combat`: los barcos que
  * hundio un jugador salen de los eventos del rival, no de los suyos.
  *
  * Definiciones cerradas en docs/diseno.md:
  *
  *   - **precision**: disparos con `hit: true` sobre el total de disparos del jugador.
  *   - **barcos hundidos por un jugador**: los `SHIP_SUNK` cuyo `playerId` es otro jugador.
  *   - **racha de aciertos**: tres o mas disparos consecutivos acertados, sin un fallo en medio.
  */
object BattleshipAnalyzer extends GameAnalyzer:

  val gameId: String = "battleship"

  private val ShotFired    = "SHOT_FIRED"
  private val ShipSunk     = "SHIP_SUNK"
  private val RachaMinima  = 3

  private final case class Flota(disparos: List[Event], hundidosAlRival: Int):
    val aciertos: List[Event]     = disparos.filter(_.field("hit").flatMap(_.boolOpt).contains(true))
    val precision: Option[Double] = Option.when(disparos.nonEmpty)(100.0 * aciertos.size / disparos.size)

    /** La racha mas larga de aciertos seguidos, con el primero y el ultimo disparo.
      *
      * Se devuelve la mas larga y no la primera porque en este juego la racha **es** el episodio:
      * cinco aciertos seguidos y los tres primeros de esos cinco describen lo mismo, y quedarse con
      * tres contaria de menos.
      */
    val rachaMasLarga: List[Event] =
      disparos
        .foldLeft((List.empty[Event], List.empty[Event])) { case ((mejor, actual), disparo) =>
          val sigue = if disparo.field("hit").flatMap(_.boolOpt).contains(true) then actual :+ disparo else Nil
          (if sigue.sizeIs > mejor.size then sigue else mejor, sigue)
        }
        ._1

  /** `SHIP_SUNK` dice que barco se hundio y de quien era, pero **no quien disparo**. Con dos bandos
    * se deduce: todo barco que no es mio lo hundi yo. Si alguna vez hubiera tres o mas jugadores
    * habria que agregar el autor al evento; para este juego, que es 1 contra 1, alcanza.
    */
  private def flotas(lote: Batch): List[(String, Flota)] =
    val disparosPorJugador = lote.deTipo(ShotFired).flatMap(e => e.playerId.map(_ -> e)).groupMap(_._1)(_._2)
    val hundidos           = lote.deTipo(ShipSunk)

    lote.jugadores
      .map { jugador =>
        jugador -> Flota(
          disparos = disparosPorJugador.getOrElse(jugador, Nil),
          hundidosAlRival = hundidos.count(!_.playerId.contains(jugador))
        )
      }
      .sortBy((_, f) => (-f.hundidosAlRival, -f.precision.getOrElse(0.0)))

  /** Cuantos barcos tiene cada bando, segun la flota declarada en MATCH_STARTED. */
  private def barcosPorBando(lote: Batch): Option[Int] =
    lote
      .deTipo(EventType.MatchStarted)
      .headOption
      .flatMap(_.field("fleet"))
      .flatMap(_.arrOpt)
      .map(_.size)

  /** Gana quien hunde la flota rival completa. Si nadie la termino, la partida no se decidio y no
    * hay ganador: coronar al que iba ganando seria inventar un resultado.
    */
  protected def ganador(lote: Batch): Option[String] =
    for
      total            <- barcosPorBando(lote)
      (jugador, flota) <- flotas(lote).headOption
      if flota.hundidosAlRival >= total
    yield jugador

  protected def resumenDelJuego(lote: Batch): ujson.Obj =
    val arranque = lote.deTipo(EventType.MatchStarted).headOption
    val campeon  = ganador(lote).flatMap(id => flotas(lote).find((j, _) => j == id)).map((_, f) => f)

    ujson.Obj(
      "boardSize" -> Js.entero(arranque.flatMap(_.long("boardSize"))),
      "shotsToWin" -> Js.entero(campeon.map(_.disparos.size.toLong)),
      "avgShotsPerShipSunk" -> Js.num(
        campeon
          .filter(_.hundidosAlRival > 0)
          .map(f => Numeros.dosDecimales(f.disparos.size.toDouble / f.hundidosAlRival))
      )
    )

  protected def jugadores(lote: Batch): List[PlayerResult] =
    flotas(lote).map { (jugador, f) =>
      PlayerResult(
        playerId = jugador,
        gameSpecific = ujson.Obj(
          "shots"     -> f.disparos.size,
          "hits"      -> f.aciertos.size,
          "accuracy"  -> Js.num(f.precision.map(Numeros.dosDecimales)),
          "shipsSunk" -> f.hundidosAlRival
        )
      )
    }

  protected def patrones(lote: Batch): List[Pattern] =
    flotas(lote).flatMap { (jugador, f) =>
      Option
        .when(f.rachaMasLarga.sizeIs >= RachaMinima)(f.rachaMasLarga)
        .map { racha =>
          Pattern(
            rule = "hitStreak",
            playerId = jugador,
            detail = ujson.Obj(
              "hits"      -> racha.size,
              "fromCoord" -> Js.texto(racha.head.str("coord")),
              "toCoord"   -> Js.texto(racha.last.str("coord"))
            )
          )
        }
    }

  /** Entre partidas: los disparos, aciertos y barcos se suman, y la precision se **recalcula** con
    * los totales. Promediar las precisiones de cada partida daria otro numero y seria el
    * equivocado, por la misma razon que el K/D en combate.
    */
  def agregarJugador(porPartida: List[ujson.Obj]): ujson.Obj =
    val disparos = porPartida.flatMap(Js.leerNum(_, "shots")).sum
    val aciertos = porPartida.flatMap(Js.leerNum(_, "hits")).sum

    ujson.Obj(
      "shots"     -> disparos,
      "hits"      -> aciertos,
      "accuracy"  -> Js.num(Option.when(disparos > 0)(Numeros.dosDecimales(100.0 * aciertos / disparos))),
      "shipsSunk" -> porPartida.flatMap(Js.leerNum(_, "shipsSunk")).sum
    )
