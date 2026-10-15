package gamestats.analytics

import java.nio.charset.StandardCharsets
import java.nio.file.{Files, Paths}

/** Pruebas del almacen y del agregado por jugador.
  *
  * Aprovecha que `p2` juega en los dos ejemplos del contrato: es el caso que demuestra por que
  * `byGame` separa los juegos en vez de sumar todo junto.
  */
class ResultStoreSuite extends munit.FunSuite:

  private val Reloj = () => "2026-10-05T00:00:00.000Z"

  private def loteDe(nombre: String): Batch =
    val ruta  = Paths.get("..", "contracts", "examples", nombre)
    val texto = new String(Files.readAllBytes(ruta), StandardCharsets.UTF_8)
    Batch.parse(texto).fold(e => fail(s"$nombre: $e"), identity)

  /** Guarda el resultado y el lote: el almacen necesita los dos para las estadisticas globales. */
  private def guardar(almacen: ResultStore, nombre: String): Unit =
    val lote = loteDe(nombre)
    val r = GameAnalyzer.para(lote.gameId).fold(fail(s"sin analizador para ${lote.gameId}"))(_.analizar(lote, Reloj))
    almacen.guardar(r, lote)

  private def almacenLleno: ResultStore =
    val almacen = ResultStore()
    guardar(almacen, "racing-match-ok.json")
    guardar(almacen, "combat-match-ok.json")
    almacen

  test("guarda y devuelve una partida"):
    val almacen = almacenLleno
    assertEquals(almacen.partida("m-racing-001").map(_.gameId), Some("racing"))
    assertEquals(almacen.partida("m-combat-001").map(_.eventCount), Some(12))

  test("una partida que no existe es None, que el endpoint traduce a 404"):
    assertEquals(almacenLleno.partida("m-no-existe"), None)

  test("guardar la misma partida dos veces la reemplaza, no la duplica"):
    val almacen = almacenLleno
    guardar(almacen, "racing-match-ok.json")
    assertEquals(almacen.jugador("p1").map(_.matchesPlayed), Some(2))

  test("p2 jugo en los dos juegos y cada uno se agrega por separado"):
    val p2 = almacenLleno.jugador("p2").getOrElse(fail("p2 deberia aparecer"))
    assertEquals(p2.matchesPlayed, 2)
    assertEquals(p2.wins, 1) // gana en combate, pierde en carreras
    assertEquals(p2.byGame.map(_.gameId), List("combat", "racing"))
    assertEquals(p2.matches.map(_.matchId), List("m-combat-001", "m-racing-001"))
    assertEquals(p2.matches.map(_.winner), List(true, false))

  test("el agregado de combate recalcula el K/D con los totales"):
    val combate = almacenLleno.jugador("p2").get.byGame.find(_.gameId == "combat").get
    assertEquals(combate.gameSpecific("eliminations").num, 3.0)
    assertEquals(combate.gameSpecific("deaths").num, 1.0)
    assertEquals(combate.gameSpecific("kd").num, 3.0)

  test("el agregado de carreras toma el minimo de las mejores vueltas"):
    val carreras = almacenLleno.jugador("p2").get.byGame.find(_.gameId == "racing").get
    assertEquals(carreras.gameSpecific("bestLapMs").num, 81000.0)
    assertEquals(carreras.gameSpecific("overtakes").num, 0.0)

  test("un jugador que no aparece en ninguna partida es None"):
    assertEquals(almacenLleno.jugador("p99"), None)

  test("p4 no gano nada y se refleja"):
    val p4 = almacenLleno.jugador("p4").getOrElse(fail("p4 deberia aparecer"))
    assertEquals(p4.matchesPlayed, 1)
    assertEquals(p4.wins, 0)
    assertEquals(p4.byGame.head.gameSpecific("avgDamage"), ujson.Null)
