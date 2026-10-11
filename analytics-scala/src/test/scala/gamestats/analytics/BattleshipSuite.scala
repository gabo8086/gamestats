package gamestats.analytics

import java.nio.charset.StandardCharsets
import java.nio.file.{Files, Paths}

/** Pruebas del analizador de hundir la flota contra el ejemplo real del contrato.
  *
  * Disparos del ejemplo: p1 falla A1 y acierta B2, B3, C5, C6, C7 (5 de 6). bot1 acierta D1 y D2 y
  * falla las tres siguientes (2 de 5). p1 hunde los dos barcos de bot1 y gana.
  */
class BattleshipSuite extends munit.FunSuite:

  private val Reloj = () => "2026-10-11T00:00:00.000Z"

  private lazy val resultado: MatchResult =
    val ruta  = Paths.get("..", "contracts", "examples", "battleship-match-ok.json")
    val texto = new String(Files.readAllBytes(ruta), StandardCharsets.UTF_8)
    val lote  = Batch.parse(texto).fold(e => fail(s"no parsea: $e"), identity)
    BattleshipAnalyzer.analizar(lote, Reloj)

  private def de(jugador: String): ujson.Obj =
    resultado.players.find(_.playerId == jugador).fold(fail(s"falta $jugador"))(_.gameSpecific)

  test("el sobre del resultado"):
    assertEquals(resultado.matchId, "m-battleship-001")
    assertEquals(resultado.gameId, "battleship")
    assertEquals(resultado.matchSummary.playerCount, 2)
    assertEquals(resultado.matchSummary.durationMs, Some(55000L))

  test("gana quien hunde la flota rival completa"):
    assertEquals(resultado.matchSummary.winnerId, Some("p1"))

  test("precision del jugador"):
    val p1 = de("p1")
    assertEquals(p1("shots").num, 6.0)
    assertEquals(p1("hits").num, 5.0)
    assertEquals(p1("accuracy").num, 83.33)

  test("precision de la maquina"):
    val bot = de("bot1")
    assertEquals(bot("shots").num, 5.0)
    assertEquals(bot("hits").num, 2.0)
    assertEquals(bot("accuracy").num, 40.0)

  test("los barcos hundidos salen de los SHIP_SUNK del rival"):
    // Hay tres SHIP_SUNK: uno de p1 (lo hundio bot1) y dos de bot1 (los hundio p1).
    assertEquals(de("p1")("shipsSunk").num, 2.0)
    assertEquals(de("bot1")("shipsSunk").num, 1.0)

  test("resumen de la partida, calculado sobre el ganador"):
    val g = resultado.matchSummary.gameSpecific
    assertEquals(g("boardSize").num, 10.0)
    assertEquals(g("shotsToWin").num, 6.0)
    assertEquals(g("avgShotsPerShipSunk").num, 3.0) // 6 disparos / 2 barcos

  test("detecta la racha y reporta la mas larga, no las primeras tres"):
    val racha = resultado.patterns.find(_.rule == "hitStreak").getOrElse(fail("no detecto la racha"))
    assertEquals(racha.playerId, "p1")
    assertEquals(racha.detail("hits").num, 5.0)
    assertEquals(racha.detail("fromCoord").str, "B2")
    assertEquals(racha.detail("toCoord").str, "C7")

  test("dos aciertos seguidos no son racha"):
    // bot1 acierta D1 y D2 y despues falla: se queda en dos.
    assertEquals(resultado.patterns.count(_.playerId == "bot1"), 0)
    assertEquals(resultado.patterns.size, 1)

  test("el registro resuelve battleship"):
    assertEquals(GameAnalyzer.para("battleship"), Some(BattleshipAnalyzer))

  test("el agregado recalcula la precision con los totales"):
    val buena = ujson.Obj("shots" -> 10, "hits" -> 9, "accuracy" -> 90.0, "shipsSunk" -> 3)
    val mala  = ujson.Obj("shots" -> 90, "hits" -> 9, "accuracy" -> 10.0, "shipsSunk" -> 1)
    val total = BattleshipAnalyzer.agregarJugador(List(buena, mala))

    assertEquals(total("shots").num, 100.0)
    assertEquals(total("hits").num, 18.0)
    // 18/100 = 18, no el promedio de 90 y 10 que daria 50
    assertEquals(total("accuracy").num, 18.0)
    assertEquals(total("shipsSunk").num, 4.0)

  test("el agregado sin disparos no divide entre cero"):
    val vacio = BattleshipAnalyzer.agregarJugador(Nil)
    assertEquals(vacio("shots").num, 0.0)
    assertEquals(vacio("accuracy"), ujson.Null)
