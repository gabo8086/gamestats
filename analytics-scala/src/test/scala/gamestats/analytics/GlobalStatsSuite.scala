package gamestats.analytics

import java.nio.charset.StandardCharsets
import java.nio.file.{Files, Paths}

/** Estadisticas que cruzan partidas y juegos: perfil global, contra bots frente a humanos,
  * desconexiones y ranking.
  *
  * El almacen se llena con tres partidas: una de carreras (gana p3), una de combate entre humanos
  * (gana p2) y una de combate contra bots (gana p1). p1 y p2 juegan las tres, asi que sirven para
  * comprobar los cortes. Los valores esperados se calcularon por fuera antes de escribir el codigo.
  */
class GlobalStatsSuite extends munit.FunSuite:

  private val Reloj = () => "2026-10-14T00:00:00.000Z"

  private def loteDe(nombre: String): Batch =
    val ruta = Paths.get("..", "contracts", "examples", nombre)
    Batch
      .parse(new String(Files.readAllBytes(ruta), StandardCharsets.UTF_8))
      .fold(e => fail(s"$nombre: $e"), identity)

  private def almacen: ResultStore =
    val store = ResultStore()
    for nombre <- List("racing-match-ok.json", "combat-match-ok.json", "combat-vs-bot-match-ok.json") do
      val lote = loteDe(nombre)
      val r = GameAnalyzer.para(lote.gameId).fold(fail(s"sin analizador"))(_.analizar(lote, Reloj))
      store.guardar(r, lote)
    store

  private def perfil(id: String): PlayerSummary =
    almacen.jugador(id).getOrElse(fail(s"$id deberia aparecer"))

  // ------------------------------------------------- perfil global

  test("el win rate global es sobre todas las partidas, sin separar por juego"):
    val p1 = perfil("p1")
    assertEquals(p1.matchesPlayed, 3)
    assertEquals(p1.wins, 1)          // solo gana la de combate contra bots
    assertEquals(p1.winRate, 33.33)

  test("un jugador que no aparece en ninguna partida es None"):
    assertEquals(almacen.jugador("p99"), None)

  // ------------------------------------------------- contra bots frente a humanos

  test("una partida cuenta como contra bots si tuvo al menos un BOT"):
    val p1 = perfil("p1")
    assertEquals(p1.vsBots, SplitPorRival(matchesPlayed = 1, wins = 1, winRate = 100.0))
    assertEquals(p1.vsHumans, SplitPorRival(matchesPlayed = 2, wins = 0, winRate = 0.0))

  test("los dos cortes suman el total de partidas"):
    for id <- List("p1", "p2") do
      val p = perfil(id)
      assertEquals(p.vsBots.matchesPlayed + p.vsHumans.matchesPlayed, p.matchesPlayed, id)
      assertEquals(p.vsBots.wins + p.vsHumans.wins, p.wins, id)

  test("p2 gana entre humanos y pierde contra bots: los cortes lo distinguen"):
    val p2 = perfil("p2")
    assertEquals(p2.vsBots, SplitPorRival(1, 0, 0.0))
    assertEquals(p2.vsHumans, SplitPorRival(2, 1, 50.0))

  // ------------------------------------------------- desconexiones

  test("volver dentro de la ventana cuenta como reconexion"):
    // En la partida contra bots p2 se desconecta y vuelve 30 s despues.
    assertEquals(perfil("p2").disconnections, Desconexiones(reconnections = 1, abandons = 0))

  test("quien nunca se desconecto queda en cero"):
    assertEquals(perfil("p1").disconnections, Desconexiones(0, 0))

  test("no volver es abandono"):
    val store = ResultStore()
    val lote = loteDe("combat-vs-bot-match-ok.json")
    val sinRegreso = lote.copy(events = lote.events.filterNot(_.eventType == EventType.PlayerReconnected))
    store.guardar(CombatAnalyzer.analizar(sinRegreso, Reloj), sinRegreso)
    assertEquals(store.jugador("p2").get.disconnections, Desconexiones(reconnections = 0, abandons = 1))

  test("volver demasiado tarde tambien es abandono"):
    val lote = loteDe("combat-vs-bot-match-ok.json")
    // El reconnect estaba a los 30 s; se corre a los 5 minutos, fuera de la ventana de 60 s.
    val tarde = lote.copy(events = lote.events.map { e =>
      if e.eventType == EventType.PlayerReconnected then e.copy(timestamp = "2026-10-09T20:05:50.000Z")
      else e
    })
    val store = ResultStore()
    store.guardar(CombatAnalyzer.analizar(tarde, Reloj), tarde)
    assertEquals(store.jugador("p2").get.disconnections, Desconexiones(reconnections = 0, abandons = 1))

  // ------------------------------------------------- listado para la interfaz

  test("el listado trae una fila por partida, ordenada por identificador"):
    val filas = almacen.listado.matches
    assertEquals(filas.map(_.matchId), List("m-combat-001", "m-combat-002", "m-racing-001"))
    assertEquals(filas.map(_.gameId), List("combat", "combat", "racing"))

  test("cada fila resume lo justo para una lista"):
    val carrera = almacen.listado.matches.find(_.matchId == "m-racing-001").get
    assertEquals(carrera.winnerId, Some("p3"))
    assertEquals(carrera.eventCount, 15)
    assertEquals(carrera.patterns, List("comeback"))

  test("las reglas repetidas se listan una sola vez"):
    // En la partida contra bots hay dos venganzas; en el listado 'revenge' aparece una vez.
    val conBots = almacen.listado.matches.find(_.matchId == "m-combat-002").get
    assertEquals(conBots.patterns.distinct, conBots.patterns)

  test("un almacen vacio devuelve una lista vacia, no un error"):
    assertEquals(ResultStore().listado.matches, Nil)

  // ------------------------------------------------- ranking

  test("el ranking ordena por victorias y desempata por porcentaje"):
    val r = almacen.ranking("combat", 10).getOrElse(fail("deberia haber ranking de combat"))
    assertEquals(r.gameId, "combat")
    assertEquals(r.ranking.map(_.playerId), List("p1", "p2", "bot1", "bot2", "p3", "p4"))
    assertEquals(r.ranking.map(_.rank), List(1, 2, 3, 4, 5, 6))
    assertEquals(r.ranking.head, RankingEntry(1, "p1", matchesPlayed = 2, wins = 1, winRate = 50.0))

  test("top recorta la lista"):
    val r = almacen.ranking("combat", 2).getOrElse(fail("deberia haber ranking"))
    assertEquals(r.ranking.map(_.playerId), List("p1", "p2"))
    assertEquals(r.top, 2)

  test("el ranking solo mira las partidas de ese juego"):
    val r = almacen.ranking("racing", 10).getOrElse(fail("deberia haber ranking de racing"))
    assertEquals(r.ranking.map(_.playerId).sorted, List("p1", "p2", "p3"))
    assertEquals(r.ranking.head.playerId, "p3") // el unico que gano

  test("un juego sin partidas analizadas es None, que el endpoint traduce a 404"):
    assertEquals(almacen.ranking("blackjack", 10), None)
    assertEquals(almacen.ranking("ajedrez", 10), None)

  test("un top negativo devuelve la lista vacia en vez de reventar"):
    assertEquals(almacen.ranking("combat", -5).get.ranking, Nil)
