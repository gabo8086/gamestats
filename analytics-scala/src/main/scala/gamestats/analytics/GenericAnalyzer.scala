package gamestats.analytics

/** Estadisticas que no dependen de ningun juego.
  *
  * Este objeto **no conoce** racing, combat, blackjack ni battleship: usa solo el sobre comun del
  * evento. Nunca abre el campo `data`. Que funcione sin saber de que juego se trata es la
  * demostracion concreta de que el sobre comun alcanza, y significa que un juego nuevo trae estas
  * metricas gratis, sin escribir una linea.
  *
  * Su salida va en el bloque `generic` de todo resultado de partida, sea del juego que sea.
  */
object GenericAnalyzer:

  private val MsPorMinuto = 60_000L

  def analizar(lote: Batch): ujson.Obj =
    ujson.Obj(
      "eventsByType"      -> porTipo(lote),
      "activityPerMinute" -> actividadPorMinuto(lote),
      "playersSeen"       -> jugadoresVistos(lote)
    )

  /** Cuantos eventos de cada tipo, en el orden en que cada tipo aparece por primera vez. Ese orden
    * se lee como la linea de tiempo de la partida y es determinista, que es lo que necesitan las
    * pruebas; un `Map` corriente no garantiza ninguno de los dos.
    */
  private def porTipo(lote: Batch): ujson.Obj =
    val cuentas = lote.events.groupMapReduce(_.eventType)(_ => 1)(_ + _)
    val orden   = lote.events.map(_.eventType).distinct
    ujson.Obj.from(orden.map(tipo => tipo -> ujson.Num(cuentas.getOrElse(tipo, 0).toDouble)))

  /** Eventos por minuto transcurrido desde el `MATCH_STARTED`.
    *
    * Los minutos sin actividad salen con `events: 0` en vez de faltar: una serie con huecos no se
    * puede graficar directo, y "no hubo eventos" es informacion, no ausencia de informacion.
    */
  private def actividadPorMinuto(lote: Batch): ujson.Arr =
    val inicio = lote
      .deTipo(EventType.MatchStarted)
      .headOption
      .orElse(lote.events.headOption)
      .map(_.timestamp)

    val minutos = for
      arranque <- inicio.toList
      evento   <- lote.events
      ms       <- Tiempo.duracionMs(arranque, evento.timestamp)
    yield math.max(0L, ms / MsPorMinuto).toInt

    if minutos.isEmpty then ujson.Arr()
    else
      val cuentas = minutos.groupMapReduce(identity)(_ => 1)(_ + _)
      ujson.Arr.from(
        (0 to minutos.max).map(m => ujson.Obj("minute" -> m, "events" -> cuentas.getOrElse(m, 0)))
      )

  /** Jugadores distintos que aparecen en **algun** evento.
    *
    * No es lo mismo que `playerCount` del sobre, que cuenta los `PLAYER_JOINED`: si un lote trae
    * eventos de alguien que nunca se unio formalmente, aqui se ve y alli no. Que los dos numeros
    * no coincidan es justamente la senal de que algo falta.
    */
  private def jugadoresVistos(lote: Batch): Int =
    lote.events.flatMap(_.playerId).distinct.size
