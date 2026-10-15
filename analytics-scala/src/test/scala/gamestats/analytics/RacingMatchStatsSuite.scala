package gamestats.analytics

import java.nio.charset.StandardCharsets
import java.nio.file.{Files, Paths}

/** Las estadisticas de carrera que faltaban: vuelta rapida, adelantamientos totales y
  * penalizaciones por jugador. Estaban listadas en docs/diseno.md pero no implementadas.
  */
class RacingMatchStatsSuite extends munit.FunSuite:

  private val Reloj = () => "2026-10-14T00:00:00.000Z"

  private lazy val resultado: MatchResult =
    val ruta  = Paths.get("..", "contracts", "examples", "racing-match-ok.json")
    val texto = new String(Files.readAllBytes(ruta), StandardCharsets.UTF_8)
    val lote  = Batch.parse(texto).fold(e => fail(s"no parsea: $e"), identity)
    RacingAnalyzer.analizar(lote, Reloj)

  private def de(jugador: String): ujson.Obj =
    resultado.players.find(_.playerId == jugador).fold(fail(s"falta $jugador"))(_.gameSpecific)

  test("la vuelta rapida de la carrera es la mejor de todas, con su autor"):
    val g = resultado.matchSummary.gameSpecific
    assertEquals(g("fastestLapMs").num, 76000.0)
    assertEquals(g("fastestLapBy").str, "p3")

  test("la vuelta rapida no tiene por que ser del ganador por tiempo ajustado"):
    // Aqui coinciden, pero son cosas distintas: una es una vuelta, la otra la carrera entera.
    val g = resultado.matchSummary.gameSpecific
    assertEquals(de("p3")("bestLapMs").num, g("fastestLapMs").num)

  test("los adelantamientos totales suman los de todos"):
    // Solo p3 adelanta, una vez: sus posiciones van 3, 1, 1.
    assertEquals(resultado.matchSummary.gameSpecific("totalOvertakes").num, 1.0)
    assertEquals(resultado.players.map(_.gameSpecific("overtakes").num).sum, 1.0)

  test("las penalizaciones acumuladas aparecen en la ficha del jugador"):
    assertEquals(de("p2")("penaltyMs").num, 6000.0)
    assertEquals(de("p1")("penaltyMs").num, 0.0)
    assertEquals(de("p3")("penaltyMs").num, 0.0)

  test("la penalizacion del jugador coincide con la del ranking"):
    val ranking = resultado.matchSummary.gameSpecific("ranking").arr.toList
    val enRanking = ranking.map(r => r("playerId").str -> r("penaltyMs").num).toMap
    for p <- List("p1", "p2", "p3") do
      assertEquals(de(p)("penaltyMs").num, enRanking(p), s"no coinciden para $p")

  test("una carrera sin vueltas completadas no inventa vuelta rapida"):
    val vacia = Batch(
      matchId = "m-sin-vueltas",
      gameId = "racing",
      gameVersion = "1.0",
      events = List(
        Event("e1", "2026-10-14T10:00:00.000Z", "racing", "1.0", "m-sin-vueltas",
              eventType = EventType.MatchStarted,
              data = ujson.Obj("matchMode" -> "PVP", "laps" -> 3, "track" -> "T")),
        Event("e2", "2026-10-14T10:00:01.000Z", "racing", "1.0", "m-sin-vueltas",
              playerId = Some("p1"), eventType = EventType.PlayerJoined,
              data = ujson.Obj("playerType" -> "HUMAN")),
        Event("e3", "2026-10-14T10:01:00.000Z", "racing", "1.0", "m-sin-vueltas",
              eventType = EventType.MatchFinished)
      )
    )
    val r = RacingAnalyzer.analizar(vacia, Reloj)
    assertEquals(r.matchSummary.gameSpecific("fastestLapMs"), ujson.Null)
    assertEquals(r.matchSummary.gameSpecific("fastestLapBy"), ujson.Null)
    assertEquals(r.matchSummary.gameSpecific("totalOvertakes").num, 0.0)

  test("el agregado entre partidas suma las penalizaciones"):
    val una  = ujson.Obj("bestLapMs" -> 80000, "avgLapMs" -> 82000.0, "overtakes" -> 1, "penaltyMs" -> 5000)
    val otra = ujson.Obj("bestLapMs" -> 76000, "avgLapMs" -> 79000.0, "overtakes" -> 2, "penaltyMs" -> 0)
    val total = RacingAnalyzer.agregarJugador(List(una, otra))

    assertEquals(total("bestLapMs").num, 76000.0) // la mejor de las dos
    assertEquals(total("overtakes").num, 3.0)
    assertEquals(total("penaltyMs").num, 5000.0)
