package gamestats.analytics

import upickle.default.{read, write}

/** Prueba del codec derivado por upickle.
  *
  * No levanta el servidor: el codec es una funcion pura y se prueba como tal. Lo que se verifica
  * es que los nombres de los campos del JSON son los que el contrato espera, porque son lo unico
  * que ve quien consume el endpoint.
  */
class HealthSuite extends munit.FunSuite:

  test("serializa con los nombres de campo del contrato"):
    val json = write(Health(status = "ok", service = "analytics-scala"))
    assertEquals(json, """{"status":"ok","service":"analytics-scala"}""")

  test("el codec va y vuelve sin perder nada"):
    val original = Health(status = "ok", service = "analytics-scala")
    assertEquals(read[Health](write(original)), original)
