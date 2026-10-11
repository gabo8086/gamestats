package gamestats.analytics

/** Analizador de blackjack: el jugador contra el crupier.
  *
  * Lo propio de este juego es que **el valor de la mano no viaja en los eventos**. Los eventos
  * dicen que carta se repartio y a quien; el total hay que reconstruirlo. Es deliberado, y es lo
  * que el enunciado §2 pide cuando habla de reconstruir informacion en vez de leerla.
  *
  * Definiciones cerradas en docs/diseno.md:
  *
  *   - **valor de una mano**: cada as suma 11 y el resto su valor (figuras 10); mientras el total
  *     pase de 21 y queden ases contados como 11, se le restan 10.
  *   - **tilt**: tras tres rondas perdidas seguidas, la apuesta de la siguiente sube.
  *   - **% de pasarse de 21**: rondas en que la mano supera 21 sobre rondas jugadas.
  *   - **saldo neto**: suma de los `payout`. El del crupier es su negativo.
  */
object BlackjackAnalyzer extends GameAnalyzer:

  val gameId: String = "blackjack"

  private val BetPlaced   = "BET_PLACED"
  private val CardDealt   = "CARD_DEALT"
  private val RoundResult = "ROUND_RESULT"

  private val Victorias     = Set("WIN", "BLACKJACK")
  private val Derrotas      = Set("LOSE", "BUST")
  private val RondasDeRacha = 3

  /** Una ronda del jugador: lo que aposto, las cartas que le tocaron y como termino. */
  private final case class Ronda(apuesta: Long, cartas: List[String], outcome: String, payout: Long):
    val valor: Int      = valorDeMano(cartas)
    val sePaso: Boolean = valor > 21
    val gano: Boolean   = Victorias.contains(outcome)
    val perdio: Boolean = Derrotas.contains(outcome)

  /** El `foldLeft` del que habla el diseno: acumula el total y **cuantos ases** van contados como
    * 11, porque sin ese segundo dato no se puede decidir cuales degradar a 1 si la mano se pasa.
    */
  def valorDeMano(cartas: List[String]): Int =
    val (total, ases) = cartas.foldLeft((0, 0)) { case ((suma, ases), carta) =>
      carta.dropRight(1) match
        case "A"                    => (suma + 11, ases + 1)
        case "10" | "J" | "Q" | "K" => (suma + 10, ases)
        case numero                 => (suma + numero.toIntOption.getOrElse(0), ases)
    }
    degradarAses(total, ases)

  /** Baja de 11 a 1 los ases que hagan falta, de a uno, hasta no pasarse o quedarse sin ases. */
  @annotation.tailrec
  private def degradarAses(total: Int, ases: Int): Int =
    if total <= 21 || ases == 0 then total else degradarAses(total - 10, ases - 1)

  /** Parte los eventos de un jugador en rondas.
    *
    * Una ronda abre con `BET_PLACED` y cierra con `ROUND_RESULT`; las cartas de en medio son suyas
    * solo si `data.to` lo nombra, porque las del crupier viajan con el mismo `playerId` del sobre.
    * Las cartas repartidas antes de la primera apuesta se descartan: no pertenecen a ninguna ronda.
    */
  private def rondas(lote: Batch, jugador: String): List[Ronda] =
    val suyos = lote.events.filter(_.playerId.contains(jugador))

    val (cerradas, _, _) =
      suyos.foldLeft((List.empty[Ronda], Option.empty[Long], List.empty[String])) {
        case ((acumuladas, apuesta, cartas), e) =>
          e.eventType match
            case BetPlaced =>
              (acumuladas, e.long("amount"), Nil)

            case CardDealt if e.str("to").contains(jugador) =>
              (acumuladas, apuesta, cartas :+ e.str("card").getOrElse(""))

            case RoundResult =>
              val ronda = Ronda(
                apuesta = apuesta.getOrElse(0L),
                cartas = cartas,
                outcome = e.str("outcome").getOrElse(""),
                payout = e.long("payout").getOrElse(0L)
              )
              (acumuladas :+ ronda, None, Nil)

            case _ => (acumuladas, apuesta, cartas)
      }

    cerradas

  /** Solo los que apostaron. El crupier se une a la partida como jugador, pero no juega rondas
    * propias y aparecer con todo en cero o en null confundiria mas de lo que informa.
    */
  private def apostadores(lote: Batch): List[(String, List[Ronda])] =
    lote.jugadores
      .map(j => j -> rondas(lote, j))
      .filter((_, suyas) => suyas.nonEmpty)
      .sortBy((_, suyas) => -suyas.map(_.payout).sum)

  /** Gana quien termina en verde. Si nadie quedo con saldo positivo, la casa se quedo con todo y
    * no hay ganador: devolver al "menos perdedor" seria inventar una victoria.
    */
  protected def ganador(lote: Batch): Option[String] =
    apostadores(lote).headOption
      .filter((_, suyas) => suyas.map(_.payout).sum > 0)
      .map((jugador, _) => jugador)

  protected def resumenDelJuego(lote: Batch): ujson.Obj =
    val todas = apostadores(lote).flatMap((_, suyas) => suyas)
    ujson.Obj(
      "rounds" -> todas.size,
      // Lo que gana el jugador lo pierde la casa, de ahi el signo invertido.
      "dealerBalance" -> -todas.map(_.payout).sum.toDouble
    )

  protected def jugadores(lote: Batch): List[PlayerResult] =
    apostadores(lote).map { (jugador, suyas) =>
      val total = suyas.size
      PlayerResult(
        playerId = jugador,
        gameSpecific = ujson.Obj(
          "roundsPlayed" -> total,
          "winRate"      -> porcentaje(suyas.count(_.gano), total),
          "netBalance"   -> suyas.map(_.payout).sum.toDouble,
          "avgBet"       -> Js.num(Numeros.promedio(suyas.map(_.apuesta.toDouble)).map(Numeros.dosDecimales)),
          // Se cuenta con la mano reconstruida, no leyendo el outcome: pasarse de 21 es un hecho
          // de las cartas, y derivarlo es lo que demuestra que el fold hace su trabajo.
          "bustRate" -> porcentaje(suyas.count(_.sePaso), total)
        )
      )
    }

  private def porcentaje(cuantos: Int, total: Int): Double =
    if total == 0 then 0.0 else Numeros.dosDecimales(100.0 * cuantos / total)

  /** Tilt: tras tres rondas perdidas seguidas, la apuesta de la siguiente sube.
    *
    * Se reporta solo el primer episodio de cada jugador: en una mala racha larga habria una ventana
    * por cada ronda y todas describirian lo mismo.
    */
  protected def patrones(lote: Batch): List[Pattern] =
    apostadores(lote).flatMap { (jugador, suyas) =>
      suyas
        .sliding(RondasDeRacha + 1)
        .zipWithIndex
        .collectFirst {
          case (tramo, desde)
              if tramo.sizeIs == RondasDeRacha + 1
                && tramo.init.forall(_.perdio)
                && tramo.last.apuesta > tramo.init.last.apuesta =>
            Pattern(
              rule = "tilt",
              playerId = jugador,
              detail = ujson.Obj(
                "lossStreak" -> RondasDeRacha,
                "betBefore"  -> tramo.init.last.apuesta.toDouble,
                "betAfter"   -> tramo.last.apuesta.toDouble,
                // Numero de ronda, empezando en 1, en que subio la apuesta.
                "atRound" -> (desde + RondasDeRacha + 1)
              )
            )
        }
    }

  /** Entre partidas: las rondas y el saldo se suman; los porcentajes y la apuesta promedio se
    * ponderan por rondas jugadas, porque promediar los promedios daria mas peso a las partidas
    * cortas.
    */
  def agregarJugador(porPartida: List[ujson.Obj]): ujson.Obj =
    val rondasTotales = porPartida.flatMap(Js.leerNum(_, "roundsPlayed")).sum

    def ponderado(clave: String): Option[Double] =
      Option.when(rondasTotales > 0)(
        porPartida.flatMap { o =>
          for
            valor  <- Js.leerNum(o, clave)
            rondas <- Js.leerNum(o, "roundsPlayed")
          yield valor * rondas
        }.sum / rondasTotales
      )

    ujson.Obj(
      "roundsPlayed" -> rondasTotales,
      "netBalance"   -> porPartida.flatMap(Js.leerNum(_, "netBalance")).sum,
      "winRate"      -> Js.num(ponderado("winRate").map(Numeros.dosDecimales)),
      "avgBet"       -> Js.num(ponderado("avgBet").map(Numeros.dosDecimales)),
      "bustRate"     -> Js.num(ponderado("bustRate").map(Numeros.dosDecimales))
    )
