package gamestats.analytics

import java.nio.charset.StandardCharsets
import java.nio.file.{Files, Paths}

/** K/D contra bots y primera sangre, contra el ejemplo PVE del contrato.
  *
  * En esa partida p1 elimina a bot1, bot2 y bot1 otra vez, mas a p2; y muere una vez, a manos de
  * bot2. O sea 3 eliminaciones a bots y 1 muerte por bot.
  */
class CombatBotsSuite extends munit.FunSuite:

  private val Reloj = () => "2026-10-14T00:00:00.000Z"

  private def analizar(nombre: String): MatchResult =
    val ruta  = Paths.get("..", "contracts", "examples", nombre)
    val texto = new String(Files.readAllBytes(ruta), StandardCharsets.UTF_8)
    val lote  = Batch.parse(texto).fold(e => fail(s"$nombre: $e"), identity)
    CombatAnalyzer.analizar(lote, Reloj)

  private lazy val conBots = analizar("combat-vs-bot-match-ok.json")
  private lazy val soloHumanos = analizar("combat-match-ok.json")

  private def de(r: MatchResult, jugador: String): ujson.Obj =
    r.players.find(_.playerId == jugador).fold(fail(s"falta $jugador"))(_.gameSpecific)

  // ------------------------------------------------- playerType en el lote

  test("el lote sabe quien es bot, a partir del PLAYER_JOINED"):
    val ruta  = Paths.get("..", "contracts", "examples", "combat-vs-bot-match-ok.json")
    val lote  = Batch.parse(new String(Files.readAllBytes(ruta), StandardCharsets.UTF_8))
      .fold(e => fail(e), identity)
    assert(lote.esBot("bot1"))
    assert(lote.esBot("bot2"))
    assert(!lote.esBot("p1"))
    assert(!lote.esBot("desconocido"), "un jugador que no se unio no deberia contar como bot")

  // ------------------------------------------------- K/D contra bots

  test("separa las eliminaciones a bots de las totales"):
    val p1 = de(conBots, "p1")
    assertEquals(p1("eliminations").num, 4.0)         // bot1, bot2, bot1 y p2
    assertEquals(p1("eliminationsVsBots").num, 3.0)   // las tres a bots
    assertEquals(p1("deathsByBots").num, 1.0)
    assertEquals(p1("kdVsBots").num, 3.0)
    assertEquals(p1("kdVsBotsUndefined").bool, false)

  test("quien nunca murio a manos de un bot queda con el K/D marcado"):
    val bot1 = de(conBots, "bot1")
    assertEquals(bot1("deathsByBots").num, 0.0)
    assertEquals(bot1("kdVsBotsUndefined").bool, true)

  test("en una partida sin bots el K/D contra bots es cero y queda marcado"):
    val p2 = de(soloHumanos, "p2")
    assertEquals(p2("eliminations").num, 3.0)
    assertEquals(p2("eliminationsVsBots").num, 0.0)
    assertEquals(p2("deathsByBots").num, 0.0)
    assertEquals(p2("kdVsBotsUndefined").bool, true)

  // ------------------------------------------------- primera sangre

  test("la primera sangre es el autor de la primera eliminacion"):
    assertEquals(conBots.matchSummary.gameSpecific("firstBloodBy").str, "p1")

  test("tambien en la partida de solo humanos"):
    // En combat-match-ok la primera baja es e-100006, de p1.
    assertEquals(soloHumanos.matchSummary.gameSpecific("firstBloodBy").str, "p1")

  test("sin eliminaciones no hay primera sangre"):
    val vacio = Batch(
      matchId = "m-sin-bajas",
      gameId = "combat",
      gameVersion = "1.0",
      events = List(
        Event("e1", "2026-10-14T10:00:00.000Z", "combat", "1.0", "m-sin-bajas",
              eventType = EventType.MatchStarted, data = ujson.Obj("matchMode" -> "PVP")),
        Event("e2", "2026-10-14T10:00:01.000Z", "combat", "1.0", "m-sin-bajas",
              playerId = Some("p1"), eventType = EventType.PlayerJoined,
              data = ujson.Obj("playerType" -> "HUMAN")),
        Event("e3", "2026-10-14T10:01:00.000Z", "combat", "1.0", "m-sin-bajas",
              eventType = EventType.MatchFinished)
      )
    )
    val r = CombatAnalyzer.analizar(vacio, Reloj)
    assertEquals(r.matchSummary.gameSpecific("firstBloodBy"), ujson.Null)
    assertEquals(r.matchSummary.winnerId, None)

  // ------------------------------------------------- agregado entre partidas

  test("el K/D contra bots se recalcula con los totales, no se promedia"):
    val una  = ujson.Obj("eliminations" -> 4, "deaths" -> 1, "avgDamage" -> 100.0,
                         "eliminationsVsBots" -> 3, "deathsByBots" -> 1)
    val otra = ujson.Obj("eliminations" -> 1, "deaths" -> 3, "avgDamage" -> 50.0,
                         "eliminationsVsBots" -> 1, "deathsByBots" -> 3)
    val total = CombatAnalyzer.agregarJugador(List(una, otra))

    assertEquals(total("eliminationsVsBots").num, 4.0)
    assertEquals(total("deathsByBots").num, 4.0)
    // 4/4 = 1, no el promedio de 3.0 y 0.33
    assertEquals(total("kdVsBots").num, 1.0)
    assertEquals(total("kdVsBotsUndefined").bool, false)

  test("el agregado sin muertes por bots queda marcado"):
    val solo = ujson.Obj("eliminations" -> 2, "deaths" -> 0, "avgDamage" -> 90.0,
                         "eliminationsVsBots" -> 2, "deathsByBots" -> 0)
    val total = CombatAnalyzer.agregarJugador(List(solo))
    assertEquals(total("kdVsBots").num, 2.0)
    assertEquals(total("kdVsBotsUndefined").bool, true)
