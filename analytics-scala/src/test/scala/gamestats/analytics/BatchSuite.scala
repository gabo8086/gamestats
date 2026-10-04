package gamestats.analytics

import java.nio.charset.StandardCharsets
import java.nio.file.{Files, Paths}

/** Pruebas del modelo contra los ejemplos reales de `contracts/examples/`.
  *
  * A proposito no se copian los JSON a `src/test/resources`: el contrato tiene una sola fuente de
  * verdad y es `contracts/`. Si alguien cambia el esquema y no actualiza el modelo, estas pruebas
  * se caen, que es exactamente lo que queremos que pase.
  */
class BatchSuite extends munit.FunSuite:

  private def ejemplo(nombre: String): String =
    val ruta = Paths.get("..", "contracts", "examples", nombre)
    new String(Files.readAllBytes(ruta), StandardCharsets.UTF_8)

  private def parseado(nombre: String): Batch =
    Batch.parse(ejemplo(nombre)) match
      case Right(lote) => lote
      case Left(error) => fail(s"$nombre no se pudo parsear: $error")

  test("el lote de carreras calza con el contrato"):
    val lote = parseado("racing-match-ok.json")
    assertEquals(lote.matchId, "m-racing-001")
    assertEquals(lote.gameId, "racing")
    assertEquals(lote.gameVersion, "1.0")
    assertEquals(lote.events.size, 15)

  test("el lote de combate calza con el contrato"):
    val lote = parseado("combat-match-ok.json")
    assertEquals(lote.matchId, "m-combat-001")
    assertEquals(lote.gameId, "combat")
    assertEquals(lote.events.size, 12)

  test("el campo type del contrato llega a eventType"):
    val lote   = parseado("racing-match-ok.json")
    val conteo = lote.events.groupBy(_.eventType).view.mapValues(_.size).toMap
    assertEquals(
      conteo,
      Map(
        "MATCH_STARTED"  -> 1,
        "PLAYER_JOINED"  -> 3,
        "LAP_COMPLETED"  -> 9,
        "PENALTY"        -> 1,
        "MATCH_FINISHED" -> 1
      )
    )

  test("los eventos MATCH_* vienen sin jugador y el resto con jugador"):
    val lote                 = parseado("racing-match-ok.json")
    val (dePartida, deJuego) = lote.events.partition(e => EventType.esDePartida(e.eventType))
    assert(dePartida.forall(_.playerId.isEmpty), "un MATCH_* trae playerId")
    assert(deJuego.forall(_.playerId.isDefined), "un evento de jugador vino sin playerId")

  test("data de LAP_COMPLETED se lee con los accesores"):
    val vuelta = parseado("racing-match-ok.json").deTipo("LAP_COMPLETED").head
    assertEquals(vuelta.int("lap"), Some(1))
    assertEquals(vuelta.long("lapTimeMs"), Some(80000L))
    assertEquals(vuelta.int("position"), Some(1))
    assertEquals(vuelta.playerId, Some("p1"))

  test("data de PLAYER_ELIMINATED se lee con los accesores"):
    val baja = parseado("combat-match-ok.json").deTipo("PLAYER_ELIMINATED").head
    assertEquals(baja.str("victimId"), Some("p3"))
    assertEquals(baja.str("weapon"), Some("rifle"))
    assertEquals(baja.int("damage"), Some(120))

  test("un campo que no esta en data devuelve None en vez de lanzar"):
    val vuelta = parseado("racing-match-ok.json").deTipo("LAP_COMPLETED").head
    assertEquals(vuelta.int("velocidadMedia"), None)
    assertEquals(vuelta.str("lap"), None) // existe pero no es string

  test("data null no rompe los accesores"):
    val fin = parseado("racing-match-ok.json").deTipo("MATCH_FINISHED").head
    assertEquals(fin.data, ujson.Null)
    assertEquals(fin.int("loQueSea"), None)

  test("porJugador deja fuera los eventos de partida"):
    val lote      = parseado("racing-match-ok.json")
    val agrupados = lote.porJugador
    assertEquals(agrupados.keySet, Set("p1", "p2", "p3"))
    assertEquals(agrupados.values.map(_.size).sum, 13) // 15 menos MATCH_STARTED y MATCH_FINISHED

  test("jugadores sale de los PLAYER_JOINED"):
    assertEquals(parseado("racing-match-ok.json").jugadores, List("p1", "p2", "p3"))

  test("un JSON roto devuelve Left y no lanza"):
    assert(Batch.parse("{esto no es json").isLeft)

  test("un lote sin eventos se rechaza"):
    val vacio = """{"matchId":"m-1","gameId":"racing","gameVersion":"1.0","events":[]}"""
    assertEquals(Batch.parse(vacio), Left("el lote no trae eventos"))

  test("un evento de otra partida se rechaza"):
    val mezclado = """{"matchId":"m-1","gameId":"racing","gameVersion":"1.0","events":[
      {"eventId":"e-1","timestamp":"2026-10-05T14:00:00.000Z","gameId":"racing",
       "gameVersion":"1.0","matchId":"m-2","type":"MATCH_STARTED"}]}"""
    Batch.parse(mezclado) match
      case Left(error) => assert(error.contains("e-1") && error.contains("m-2"), error)
      case Right(_)    => fail("acepto un evento de otra partida")

  test("faltar un campo obligatorio del sobre es un Left"):
    val sinTipo = """{"matchId":"m-1","gameId":"racing","gameVersion":"1.0","events":[
      {"eventId":"e-1","timestamp":"2026-10-05T14:00:00.000Z","gameId":"racing",
       "gameVersion":"1.0","matchId":"m-1"}]}"""
    assert(Batch.parse(sinTipo).isLeft)
