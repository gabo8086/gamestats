package gamestats.analytics

import java.nio.charset.StandardCharsets
import java.nio.file.{Files, Paths}

/** Pruebas del analizador de blackjack contra el ejemplo real del contrato.
  *
  * Los valores esperados salen de `contracts/results.md` y de las cartas del ejemplo, calculados a
  * mano antes de escribir el analizador.
  */
class BlackjackSuite extends munit.FunSuite:

  private val Reloj = () => "2026-10-11T00:00:00.000Z"

  private lazy val resultado: MatchResult =
    val ruta  = Paths.get("..", "contracts", "examples", "blackjack-match-ok.json")
    val texto = new String(Files.readAllBytes(ruta), StandardCharsets.UTF_8)
    val lote  = Batch.parse(texto).fold(e => fail(s"no parsea: $e"), identity)
    BlackjackAnalyzer.analizar(lote, Reloj)

  private def de(jugador: String): ujson.Obj =
    resultado.players.find(_.playerId == jugador).fold(fail(s"falta $jugador"))(_.gameSpecific)

  // ------------------------------------------------- el fold que reconstruye la mano

  test("suma las figuras como 10 y los numeros por su valor"):
    assertEquals(BlackjackAnalyzer.valorDeMano(List("10H", "7S")), 17)
    assertEquals(BlackjackAnalyzer.valorDeMano(List("KD", "QC")), 20)
    assertEquals(BlackjackAnalyzer.valorDeMano(List("2H", "3D", "4C")), 9)

  test("el as vale 11 mientras no pase de 21"):
    assertEquals(BlackjackAnalyzer.valorDeMano(List("AS", "KS")), 21)
    assertEquals(BlackjackAnalyzer.valorDeMano(List("AD", "9C")), 20)

  test("el as baja a 1 solo cuando hace falta"):
    // 11 + 9 + 5 = 25 se pasa, con el as en 1 son 15
    assertEquals(BlackjackAnalyzer.valorDeMano(List("AS", "9C", "5H")), 15)

  test("con varios ases degrada de a uno, no todos de golpe"):
    // 11 + 1 + 9 = 21, no 1 + 1 + 9
    assertEquals(BlackjackAnalyzer.valorDeMano(List("AS", "AD", "9C")), 21)
    // 11 + 1 + 1 + 1 + 7 = 21
    assertEquals(BlackjackAnalyzer.valorDeMano(List("AS", "AD", "AH", "AC", "7S")), 21)

  test("una mano que se pasa devuelve su total real, no 21"):
    assertEquals(BlackjackAnalyzer.valorDeMano(List("10C", "6H", "8S")), 24)

  test("una mano vacia vale cero"):
    assertEquals(BlackjackAnalyzer.valorDeMano(Nil), 0)

  // ------------------------------------------------- la partida del contrato

  test("el sobre del resultado"):
    assertEquals(resultado.matchId, "m-blackjack-001")
    assertEquals(resultado.gameId, "blackjack")
    assertEquals(resultado.matchSummary.playerCount, 2) // el jugador y el crupier

  test("gana quien termina con saldo positivo"):
    assertEquals(resultado.matchSummary.winnerId, Some("p1"))

  test("el saldo del crupier es el negativo del jugador"):
    val g = resultado.matchSummary.gameSpecific
    assertEquals(g("rounds").num, 6.0)
    assertEquals(g("dealerBalance").num, -45.0)

  test("estadisticas del jugador"):
    val p1 = de("p1")
    assertEquals(p1("roundsPlayed").num, 6.0)
    assertEquals(p1("netBalance").num, 45.0)   // -10 -10 -10 +45 +0 +30
    assertEquals(p1("avgBet").num, 20.0)       // (10+10+10+30+30+30)/6
    assertEquals(p1("winRate").num, 33.33)     // BLACKJACK y WIN de 6 rondas
    assertEquals(p1("bustRate").num, 16.67)    // una sola ronda pasada de 21

  test("el crupier no aparece como jugador: no apuesta"):
    assertEquals(resultado.players.map(_.playerId), List("p1"))

  test("el bustRate derivado de las cartas coincide con el outcome del evento"):
    // Es la comprobacion de que el fold reconstruye bien: el analizador no lee "BUST", lo deduce.
    val ruta   = Paths.get("..", "contracts", "examples", "blackjack-match-ok.json")
    val texto  = new String(Files.readAllBytes(ruta), StandardCharsets.UTF_8)
    val lote   = Batch.parse(texto).fold(e => fail(e), identity)
    val segunEvento = lote.deTipo("ROUND_RESULT").count(_.str("outcome").contains("BUST"))
    assertEquals(segunEvento, 1)
    assertEquals(de("p1")("bustRate").num, Numeros.dosDecimales(100.0 * segunEvento / 6))

  // ------------------------------------------------- la regla

  test("detecta el tilt tras tres derrotas seguidas"):
    val tilt = resultado.patterns.find(_.rule == "tilt").getOrElse(fail("no detecto el tilt"))
    assertEquals(tilt.playerId, "p1")
    assertEquals(tilt.detail("lossStreak").num, 3.0)
    assertEquals(tilt.detail("betBefore").num, 10.0)
    assertEquals(tilt.detail("betAfter").num, 30.0)
    assertEquals(tilt.detail("atRound").num, 4.0)

  test("solo se reporta un tilt por jugador"):
    assertEquals(resultado.patterns.count(_.rule == "tilt"), 1)

  // ------------------------------------------------- agregado entre partidas

  test("el agregado pondera los porcentajes por rondas jugadas"):
    val corta = ujson.Obj("roundsPlayed" -> 2, "netBalance" -> 10, "winRate" -> 50.0,
                          "avgBet" -> 10.0, "bustRate" -> 0.0)
    val larga = ujson.Obj("roundsPlayed" -> 8, "netBalance" -> -20, "winRate" -> 25.0,
                          "avgBet" -> 20.0, "bustRate" -> 25.0)
    val total = BlackjackAnalyzer.agregarJugador(List(corta, larga))

    assertEquals(total("roundsPlayed").num, 10.0)
    assertEquals(total("netBalance").num, -10.0)
    // (50*2 + 25*8) / 10 = 30, no (50+25)/2 = 37.5
    assertEquals(total("winRate").num, 30.0)
    assertEquals(total("avgBet").num, 18.0)
    assertEquals(total("bustRate").num, 20.0)

  test("el agregado sin partidas no divide entre cero"):
    val vacio = BlackjackAnalyzer.agregarJugador(Nil)
    assertEquals(vacio("roundsPlayed").num, 0.0)
    assertEquals(vacio("winRate"), ujson.Null)

  test("el registro resuelve blackjack"):
    assertEquals(GameAnalyzer.para("blackjack"), Some(BlackjackAnalyzer))
