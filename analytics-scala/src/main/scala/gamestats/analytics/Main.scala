package gamestats.analytics

import upickle.default.{ReadWriter, write}

/** Respuesta de `/health`.
  *
  * Es una case class y no un string a mano para que upickle derive el codec: el mismo mecanismo
  * que van a usar el lote de entrada y los resultados. `derives ReadWriter` genera la conversion
  * a JSON en tiempo de compilacion, asi que un campo que se renombre rompe la compilacion en vez
  * de producir un JSON equivocado en ejecucion.
  */
final case class Health(status: String, service: String) derives ReadWriter

/** Modulo de analisis de GameStats (paradigma funcional).
  *
  * Estado: esqueleto. Solo responde /health, lo justo para que docker compose y el CI tengan algo
  * que verificar. Lo que falta, en orden (ver CLAUDE.md):
  *
  *   - Dia 1: hecho. El modelo inmutable esta en Event.scala y Batch.scala.
  *   - Dia 2: `trait GameAnalyzer` con `RacingAnalyzer` y `CombatAnalyzer`; estadisticas y reglas con
  *     filter / map / groupBy / fold.
  *   - Dia 3: POST /analyze (ya puede usar Batch.parse) y los GET /results/... con el formato de
  *     contracts/results.md.
  *
  * Este archivo es el borde HTTP del modulo y el unico que sabe que cask existe. El analisis que
  * viene despues son funciones puras sobre colecciones inmutables y no importa nada de aqui.
  */
object Main extends cask.MainRoutes:

  private val config = Config.fromSystemEnv()

  override def port: Int = config.port

  /** cask escucha en `localhost` por defecto. Dentro de un contenedor eso solo acepta conexiones
    * del propio contenedor, de modo que el servicio de Go no podria alcanzarnos y el healthcheck
    * de docker compose fallaria. `0.0.0.0` escucha en todas las interfaces.
    */
  override def host: String = "0.0.0.0"

  @cask.get("/health")
  def health(): cask.Response[String] =
    json(Health(status = "ok", service = "analytics-scala"))

  /** Serializa cualquier case class con codec derivado y le pone el Content-Type. Todas las
    * respuestas del modulo salen por aqui para no repetir la cabecera en cada endpoint.
    */
  private def json[A: ReadWriter](cuerpo: A, status: Int = 200): cask.Response[String] =
    cask.Response(
      write(cuerpo),
      statusCode = status,
      headers = Seq("Content-Type" -> "application/json")
    )

  initialize()
