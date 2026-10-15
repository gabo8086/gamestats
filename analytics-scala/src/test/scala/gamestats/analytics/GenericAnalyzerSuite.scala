package gamestats.analytics

import java.nio.charset.StandardCharsets
import java.nio.file.{Files, Paths}

/** Pruebas del analizador que no conoce ningun juego.
  *
  * La prueba que mas importa no es ninguna en particular, sino que **todas pasan para los cuatro
  * juegos con el mismo codigo**: si hiciera falta un caso especial por juego, el GenericAnalyzer no
  * estaria cumpliendo su proposito.
  */
class GenericAnalyzerSuite extends munit.FunSuite:

  private val Reloj = () => "2026-10-14T00:00:00.000Z"

  private def lote(nombre: String): Batch =
    val ruta  = Paths.get("..", "contracts", "examples", nombre)
    val texto = new String(Files.readAllBytes(ruta), StandardCharsets.UTF_8)
    Batch.parse(texto).fold(e => fail(s"$nombre: $e"), identity)

  private def generico(nombre: String): ujson.Obj =
    val b = lote(nombre)
    GameAnalyzer.para(b.gameId).fold(fail(s"sin analizador para ${b.gameId}"))(_.analizar(b, Reloj)).generic

  private val ejemplos = List(
    "racing-match-ok.json",
    "combat-match-ok.json",
    "combat-vs-bot-match-ok.json",
    "blackjack-match-ok.json",
    "battleship-match-ok.json"
  )

  // ------------------------------------------------- vale para los cuatro juegos

  test("todos los juegos traen el bloque generic, sin codigo propio de ninguno"):
    for nombre <- ejemplos do
      val g = generico(nombre)
      assert(g.value.contains("eventsByType"), s"$nombre sin eventsByType")
      assert(g.value.contains("activityPerMinute"), s"$nombre sin activityPerMinute")
      assert(g.value.contains("playersSeen"), s"$nombre sin playersSeen")

  test("la suma de eventsByType es la cantidad total de eventos"):
    for nombre <- ejemplos do
      val b = lote(nombre)
      val g = generico(nombre)
      val suma = g("eventsByType").obj.values.map(_.num).sum
      assertEquals(suma, b.events.size.toDouble, s"no cuadra en $nombre")

  // ------------------------------------------------- distribucion por tipo

  test("cuenta los eventos por tipo, en orden de aparicion"):
    val porTipo = generico("combat-match-ok.json")("eventsByType").obj
    assertEquals(porTipo.keys.toList,
                 List("MATCH_STARTED", "PLAYER_JOINED", "PLAYER_ELIMINATED", "MATCH_FINISHED"))
    assertEquals(porTipo("MATCH_STARTED").num, 1.0)
    assertEquals(porTipo("PLAYER_JOINED").num, 4.0)
    assertEquals(porTipo("PLAYER_ELIMINATED").num, 6.0)
    assertEquals(porTipo("MATCH_FINISHED").num, 1.0)

  test("conoce los tipos de un juego que no sabe que existe"):
    // Nadie le enseno que es BET_PLACED ni SHOT_FIRED: los cuenta porque estan en el sobre.
    assert(generico("blackjack-match-ok.json")("eventsByType").obj.contains("BET_PLACED"))
    assert(generico("battleship-match-ok.json")("eventsByType").obj.contains("SHOT_FIRED"))

  // ------------------------------------------------- actividad por minuto

  test("agrupa los eventos por minuto desde el arranque"):
    val serie = generico("combat-match-ok.json")("activityPerMinute").arr.toList
    assertEquals(serie.map(m => (m("minute").num.toInt, m("events").num.toInt)),
                 List((0, 6), (1, 4), (2, 2)))

  test("la serie no tiene huecos: arranca en 0 y los minutos son consecutivos"):
    for nombre <- ejemplos do
      val minutos = generico(nombre)("activityPerMinute").arr.toList.map(_("minute").num.toInt)
      assertEquals(minutos, minutos.indices.toList, s"serie con huecos en $nombre")

  test("un minuto sin actividad sale en cero, no se omite"):
    // Dos eventos separados por mas de dos minutos: el minuto del medio tiene que aparecer.
    val conHueco = Batch(
      matchId = "m-hueco", gameId = "racing", gameVersion = "1.0",
      events = List(
        Event("e1", "2026-10-14T10:00:00.000Z", "racing", "1.0", "m-hueco",
              eventType = EventType.MatchStarted,
              data = ujson.Obj("matchMode" -> "PVP", "laps" -> 1, "track" -> "T")),
        Event("e2", "2026-10-14T10:02:30.000Z", "racing", "1.0", "m-hueco",
              eventType = EventType.MatchFinished)
      )
    )
    val serie = RacingAnalyzer.analizar(conHueco, Reloj).generic("activityPerMinute").arr.toList
    assertEquals(serie.map(m => (m("minute").num.toInt, m("events").num.toInt)),
                 List((0, 1), (1, 0), (2, 1)))

  // ------------------------------------------------- jugadores vistos

  test("cuenta los jugadores distintos que aparecen en algun evento"):
    assertEquals(generico("combat-match-ok.json")("playersSeen").num, 4.0)
    assertEquals(generico("battleship-match-ok.json")("playersSeen").num, 2.0)

  test("playersSeen puede diferir de playerCount, y esa diferencia es la senal util"):
    // En blackjack los dos coinciden: el crupier se une y tambien recibe cartas.
    val b = lote("blackjack-match-ok.json")
    val r = BlackjackAnalyzer.analizar(b, Reloj)
    assertEquals(r.generic("playersSeen").num, 2.0)
    assertEquals(r.matchSummary.playerCount, 2)

  test("un lote de un solo evento no rompe nada"):
    val minimo = Batch(
      matchId = "m-uno", gameId = "racing", gameVersion = "1.0",
      events = List(
        Event("e1", "2026-10-14T10:00:00.000Z", "racing", "1.0", "m-uno",
              eventType = EventType.MatchStarted,
              data = ujson.Obj("matchMode" -> "PVP", "laps" -> 1, "track" -> "T"))
      )
    )
    val g = RacingAnalyzer.analizar(minimo, Reloj).generic
    assertEquals(g("playersSeen").num, 0.0)
    assertEquals(g("activityPerMinute").arr.toList.map(_("events").num), List(1.0))
