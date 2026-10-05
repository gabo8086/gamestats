package gamestats.analytics

import java.nio.charset.StandardCharsets
import java.nio.file.{Files, Paths}

/** Pruebas de los analizadores contra los ejemplos reales del contrato.
  *
  * Los valores esperados son los de `contracts/results.md`, calculados a mano a partir de los
  * eventos. Si un analizador cambia un criterio sin querer, estas pruebas lo dicen con el numero
  * exacto que se movio.
  */
class AnalyzerSuite extends munit.FunSuite:

  private val Reloj = () => "2026-10-05T00:00:00.000Z"

  private def lote(nombre: String): Batch =
    val ruta  = Paths.get("..", "contracts", "examples", nombre)
    val texto = new String(Files.readAllBytes(ruta), StandardCharsets.UTF_8)
    Batch.parse(texto).fold(e => fail(s"$nombre: $e"), identity)

  private def analizar(nombre: String): MatchResult =
    val b = lote(nombre)
    GameAnalyzer.para(b.gameId).fold(fail(s"sin analizador para ${b.gameId}"))(_.analizar(b, Reloj))

  private def deJugador(r: MatchResult, playerId: String): ujson.Obj =
    r.players.find(_.playerId == playerId).fold(fail(s"falta el jugador $playerId"))(_.gameSpecific)

  // ---------------------------------------------------------------- registro

  test("el registro resuelve los dos juegos y nada mas"):
    assertEquals(GameAnalyzer.para("racing"), Some(RacingAnalyzer))
    assertEquals(GameAnalyzer.para("combat"), Some(CombatAnalyzer))
    assertEquals(GameAnalyzer.para("ajedrez"), None)
    assertEquals(GameAnalyzer.conocidos, List("combat", "racing"))

  // ---------------------------------------------------------------- carreras

  test("racing: el sobre del resultado"):
    val r = analizar("racing-match-ok.json")
    assertEquals(r.matchId, "m-racing-001")
    assertEquals(r.gameId, "racing")
    assertEquals(r.eventCount, 15)
    assertEquals(r.analyzedAt, "2026-10-05T00:00:00.000Z") // el reloj entra por parametro
    assertEquals(r.matchSummary.playerCount, 3)
    assertEquals(r.matchSummary.durationMs, Some(260000L))

  test("racing: gana el menor tiempo ajustado, no el de la mejor vuelta"):
    val r = analizar("racing-match-ok.json")
    assertEquals(r.matchSummary.winnerId, Some("p3"))

  test("racing: la penalizacion entra en el tiempo ajustado"):
    val r       = analizar("racing-match-ok.json")
    val ranking = r.matchSummary.gameSpecific("ranking").arr.toList
    assertEquals(ranking.map(_("playerId").str), List("p3", "p1", "p2"))
    assertEquals(ranking.map(_("adjustedTimeMs").num), List(238000.0, 250000.0, 252000.0))
    // p2 corrio 246000 ms y los 6 s de PENALTY lo mandan al ultimo puesto
    assertEquals(ranking.map(_("penaltyMs").num), List(0.0, 0.0, 6000.0))

  test("racing: estadisticas por jugador"):
    val p3 = deJugador(analizar("racing-match-ok.json"), "p3")
    assertEquals(p3("bestLapMs").num, 76000.0)
    assertEquals(p3("avgLapMs").num, 79333.33)
    assertEquals(p3("consistencyMs").num, 4027.68) // desviacion poblacional
    assertEquals(p3("overtakes").num, 1.0)
    assertEquals(p3("positions").arr.map(_.num).toList, List(3.0, 1.0, 1.0))

  test("racing: el que no adelanta a nadie tiene cero adelantamientos"):
    val p2 = deJugador(analizar("racing-match-ok.json"), "p2")
    assertEquals(p2("overtakes").num, 0.0)
    assertEquals(p2("positions").arr.map(_.num).toList, List(2.0, 2.0, 2.0))

  test("racing: detecta la remontada del ganador"):
    val r = analizar("racing-match-ok.json")
    assertEquals(r.patterns.map(_.rule), List("comeback"))
    val remontada = r.patterns.head
    assertEquals(remontada.playerId, "p3")
    assertEquals(remontada.detail("worstPosition").num, 3.0) // ultimo entre 3 jugadores
    assertEquals(remontada.detail("worstPositionLap").num, 1.0)
    assertEquals(remontada.detail("finalPosition").num, 1.0)

  test("racing: el resumen trae pista y vueltas"):
    val g = analizar("racing-match-ok.json").matchSummary.gameSpecific
    assertEquals(g("track").str, "Circuito Volcan")
    assertEquals(g("laps").num, 3.0)

  // ---------------------------------------------------------------- combate

  test("combat: el sobre del resultado"):
    val r = analizar("combat-match-ok.json")
    assertEquals(r.eventCount, 12)
    assertEquals(r.matchSummary.playerCount, 4)
    assertEquals(r.matchSummary.durationMs, Some(130000L))
    assertEquals(r.matchSummary.winnerId, Some("p2"))
    assertEquals(r.matchSummary.gameSpecific("deadliestPlayerId").str, "p2")
    assertEquals(r.matchSummary.gameSpecific("mode").str, "deathmatch")

  test("combat: las muertes salen del victimId de los eventos ajenos"):
    val p3 = deJugador(analizar("combat-match-ok.json"), "p3")
    assertEquals(p3("eliminations").num, 1.0)
    assertEquals(p3("deaths").num, 2.0) // nunca aparece como playerId en esas dos
    assertEquals(p3("kd").num, 0.5)

  test("combat: K/D sin muertes devuelve las eliminaciones y se marca"):
    val p1 = deJugador(analizar("combat-match-ok.json"), "p1")
    assertEquals(p1("deaths").num, 0.0)
    assertEquals(p1("kd").num, 2.0)
    assertEquals(p1("kdUndefined").bool, true)

  test("combat: kdUndefined es false cuando si hubo muertes"):
    val p2 = deJugador(analizar("combat-match-ok.json"), "p2")
    assertEquals(p2("kd").num, 3.0)
    assertEquals(p2("kdUndefined").bool, false)
    assertEquals(p2("avgDamage").num, 111.67)

  test("combat: sin eliminaciones el dano promedio es null, no cero"):
    val p4 = deJugador(analizar("combat-match-ok.json"), "p4")
    assertEquals(p4("eliminations").num, 0.0)
    assertEquals(p4("avgDamage"), ujson.Null)

  test("combat: eliminaciones y dano por arma"):
    val armas = analizar("combat-match-ok.json").matchSummary.gameSpecific("byWeapon").arr.toList
    assertEquals(armas.map(_("weapon").str), List("rifle", "shotgun", "pistol"))
    assertEquals(armas.map(_("eliminations").num), List(3.0, 2.0, 1.0))
    assertEquals(armas.map(_("avgDamage").num), List(116.67, 102.5, 80.0))

  test("combat: detecta la racha dentro de la ventana de 10 s"):
    val racha = analizar("combat-match-ok.json").patterns.find(_.rule == "streak").get
    assertEquals(racha.playerId, "p2")
    assertEquals(racha.detail("windowMs").num, 8000.0)
    assertEquals(racha.detail("eventIds").arr.map(_.str).toList, List("e-100007", "e-100008", "e-100009"))

  test("combat: detecta la venganza"):
    val venganza = analizar("combat-match-ok.json").patterns.find(_.rule == "revenge").get
    assertEquals(venganza.playerId, "p3")
    assertEquals(venganza.detail("againstPlayerId").str, "p2")
    assertEquals(venganza.detail("eliminatedByEventId").str, "e-100008")
    assertEquals(venganza.detail("revengeEventId").str, "e-100010")

  test("combat: la venganza se reporta una sola vez por par"):
    val r = analizar("combat-match-ok.json")
    assertEquals(r.patterns.count(_.rule == "revenge"), 1)
