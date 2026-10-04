package gamestats.analytics

class ConfigSuite extends munit.FunSuite:

  test("sin PORT usa el puerto por defecto"):
    assertEquals(Config.fromEnv(_ => None).port, 8081)

  test("usa el PORT del entorno cuando es valido"):
    val config = Config.fromEnv(Map("PORT" -> "9091").get)
    assertEquals(config.port, 9091)

  test("ignora un PORT que no es numero"):
    val config = Config.fromEnv(Map("PORT" -> "ocho-mil").get)
    assertEquals(config.port, Config.PuertoPorDefecto)

  test("ignora un PORT fuera de rango"):
    assertEquals(Config.fromEnv(Map("PORT" -> "0").get).port, Config.PuertoPorDefecto)
    assertEquals(Config.fromEnv(Map("PORT" -> "70000").get).port, Config.PuertoPorDefecto)
