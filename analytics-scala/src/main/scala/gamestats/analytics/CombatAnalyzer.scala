package gamestats.analytics

/** Analizador de combate.
  *
  * En `PLAYER_ELIMINATED` el `playerId` del sobre es **quien elimina** y la victima va en
  * `data.victimId`. De ahi que las muertes de un jugador no se cuenten con sus propios eventos sino
  * apareciendo como victima en los de otros: es el caso tipico de estadistica que obliga a cruzar
  * el sobre con el `data`.
  *
  * Definiciones cerradas en el CLAUDE.md:
  *
  *   - **K/D** = eliminaciones / muertes. Con 0 muertes devuelve las eliminaciones y marca
  *     `kdUndefined: true`, para no confundir "3 sin morir" con "K/D de 3.0".
  *   - **mas letal** = mas eliminaciones; desempate por dano promedio.
  *   - **racha** = 3 eliminaciones del mismo jugador en <= 10 s sin que aparezca como victima en medio.
  *   - **venganza** = A elimina a B y despues B elimina a A.
  */
object CombatAnalyzer extends GameAnalyzer:

  val gameId: String = "combat"

  private val PlayerEliminated = "PLAYER_ELIMINATED"
  private val VentanaRachaMs   = 10_000L
  private val EliminacionesRacha = 3

  private def ms(evento: Event): Option[Long] =
    Tiempo.instante(evento.timestamp).map(_.toEpochMilli)

  private final case class Luchador(playerId: String, eliminaciones: List[Event], muertes: List[Event]):
    val danos: List[Double]        = eliminaciones.flatMap(_.num("damage"))
    val danoPromedio: Option[Double] = Numeros.promedio(danos)
    val kdIndefinido: Boolean      = muertes.isEmpty

    /** Con 0 muertes el cociente seria infinito; el contrato pide devolver las eliminaciones y
      * marcarlo aparte, que es informacion y no un numero inventado.
      */
    val kd: Double =
      if kdIndefinido then eliminaciones.size.toDouble
      else eliminaciones.size.toDouble / muertes.size

  private def luchadores(lote: Batch): List[Luchador] =
    val bajas = lote.deTipo(PlayerEliminated)

    val porAutor: Map[String, List[Event]] =
      bajas.flatMap(e => e.playerId.map(_ -> e)).groupMap(_._1)(_._2)

    val porVictima: Map[String, List[Event]] =
      bajas.flatMap(e => e.str("victimId").map(_ -> e)).groupMap(_._1)(_._2)

    lote.jugadores
      .map(id => Luchador(id, porAutor.getOrElse(id, Nil), porVictima.getOrElse(id, Nil)))
      .sortBy(l => (-l.eliminaciones.size, -l.danoPromedio.getOrElse(0.0)))

  /** Gana el de mas eliminaciones; el desempate por dano promedio ya viene del orden de
    * `luchadores`. Quien no elimino a nadie no gana aunque sea el unico en la lista.
    */
  protected def ganador(lote: Batch): Option[String] =
    luchadores(lote).find(_.eliminaciones.nonEmpty).map(_.playerId)

  protected def resumenDelJuego(lote: Batch): ujson.Obj =
    val arranque = lote.deTipo(EventType.MatchStarted).headOption

    val porArma = lote
      .deTipo(PlayerEliminated)
      .flatMap(e => e.str("weapon").map(_ -> e))
      .groupMap(_._1)(_._2)
      .toList
      .map { (arma, eventos) =>
        (arma, eventos.size, Numeros.promedio(eventos.flatMap(_.num("damage"))))
      }
      .sortBy((arma, cuantas, _) => (-cuantas, arma))

    ujson.Obj(
      "mode"              -> Js.texto(arranque.flatMap(_.str("mode"))),
      "deadliestPlayerId" -> Js.texto(ganador(lote)),
      "byWeapon" -> ujson.Arr.from(
        porArma.map { (arma, cuantas, promedio) =>
          ujson.Obj(
            "weapon"       -> arma,
            "eliminations" -> cuantas,
            "avgDamage"    -> Js.num(promedio.map(Numeros.dosDecimales))
          )
        }
      )
    )

  /** Los jugadores salen ordenados por eliminaciones, que es como se lee una tabla de combate. */
  protected def jugadores(lote: Batch): List[PlayerResult] =
    luchadores(lote).map { l =>
      PlayerResult(
        playerId = l.playerId,
        gameSpecific = ujson.Obj(
          "eliminations" -> l.eliminaciones.size,
          "deaths"       -> l.muertes.size,
          "kd"           -> Numeros.dosDecimales(l.kd),
          "kdUndefined"  -> l.kdIndefinido,
          "avgDamage"    -> Js.num(l.danoPromedio.map(Numeros.dosDecimales))
        )
      )
    }

  /** Entre partidas se suman eliminaciones y muertes y el K/D se **recalcula** con esos totales.
    * Promediar los K/D de cada partida daria otro numero y seria el equivocado. El dano promedio se
    * pondera por eliminaciones, por la misma razon.
    */
  def agregarJugador(porPartida: List[ujson.Obj]): ujson.Obj =
    val eliminaciones = porPartida.flatMap(Js.leerNum(_, "eliminations")).sum
    val muertes       = porPartida.flatMap(Js.leerNum(_, "deaths")).sum

    val danoTotal = porPartida.flatMap { o =>
      for
        promedio <- Js.leerNum(o, "avgDamage")
        cuantas  <- Js.leerNum(o, "eliminations")
      yield promedio * cuantas
    }.sum

    ujson.Obj(
      "eliminations" -> eliminaciones,
      "deaths"       -> muertes,
      "kd"           -> Numeros.dosDecimales(if muertes == 0 then eliminaciones else eliminaciones / muertes),
      "kdUndefined"  -> (muertes == 0),
      "avgDamage"    -> Js.num(Option.when(eliminaciones > 0)(Numeros.dosDecimales(danoTotal / eliminaciones)))
    )

  protected def patrones(lote: Batch): List[Pattern] =
    val todos = luchadores(lote)
    todos.flatMap(racha) ++ venganzas(lote)

  /** Racha: tres eliminaciones seguidas dentro de la ventana, sin morir en medio.
    *
    * `sliding(3)` recorre las eliminaciones del jugador de tres en tres consecutivas. Se reporta
    * solo la primera racha de cada jugador: con cuatro eliminaciones rapidas habria dos ventanas
    * solapadas describiendo el mismo episodio.
    *
    * El `sizeIs == EliminacionesRacha` no sobra: `sliding` **no exige que la ventana este completa**,
    * asi que `List(a).sliding(3)` devuelve igual un grupo de un elemento. Sin esa guarda, cualquiera
    * con una o dos eliminaciones producia una "racha" de una eliminacion y ventana de 0 ms.
    */
  private def racha(luchador: Luchador): Option[Pattern] =
    val muertesMs = luchador.muertes.flatMap(ms)

    luchador.eliminaciones
      .sliding(EliminacionesRacha)
      .collectFirst {
        case tramo @ primera :: _
            if tramo.sizeIs == EliminacionesRacha && dentroDeVentana(tramo) && !murioEn(tramo, muertesMs) =>
          val desde = ms(primera).getOrElse(0L)
          val hasta = ms(tramo.last).getOrElse(0L)
          Pattern(
            rule = "streak",
            playerId = luchador.playerId,
            detail = ujson.Obj(
              "eliminations" -> tramo.size,
              "windowMs"     -> (hasta - desde).toDouble,
              "eventIds"     -> ujson.Arr.from(tramo.map(_.eventId))
            )
          )
      }

  private def dentroDeVentana(tramo: List[Event]): Boolean =
    (for
      desde <- ms(tramo.head)
      hasta <- ms(tramo.last)
    yield hasta - desde <= VentanaRachaMs).getOrElse(false)

  /** El jugador no debe haber muerto entre la primera y la ultima eliminacion del tramo. */
  private def murioEn(tramo: List[Event], muertesMs: List[Long]): Boolean =
    (for
      desde <- ms(tramo.head)
      hasta <- ms(tramo.last)
    yield muertesMs.exists(m => m >= desde && m <= hasta)).getOrElse(false)

  /** Venganza: A elimina a B y mas tarde B elimina a A.
    *
    * Se reporta una sola vez por par ordenado (vengador, vengado): si se matan cinco veces entre
    * ellos lo interesante es que la venganza ocurrio, no cada repeticion.
    */
  private def venganzas(lote: Batch): List[Pattern] =
    val bajas = lote.deTipo(PlayerEliminated)

    val pares =
      for
        (primera, i) <- bajas.zipWithIndex
        autor        <- primera.playerId.toList
        victima      <- primera.str("victimId").toList
        respuesta <- bajas
          .drop(i + 1)
          .find(e => e.playerId.contains(victima) && e.str("victimId").contains(autor))
          .toList
      yield (victima, autor) -> Pattern(
        rule = "revenge",
        playerId = victima,
        detail = ujson.Obj(
          "againstPlayerId"     -> autor,
          "eliminatedByEventId" -> primera.eventId,
          "revengeEventId"      -> respuesta.eventId
        )
      )

    pares.distinctBy(_._1).map(_._2)
