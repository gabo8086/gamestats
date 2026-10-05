package gamestats.analytics

import java.nio.charset.StandardCharsets
import java.time.Instant
import java.time.temporal.ChronoUnit
import upickle.default.{ReadWriter, write}

/** Respuesta de `/health`.
  *
  * Es una case class y no un string a mano para que upickle derive el codec: el mismo mecanismo
  * que usan el lote de entrada y los resultados.
  */
final case class Health(status: String, service: String) derives ReadWriter

/** Modulo de analisis de GameStats (paradigma funcional).
  *
  * Este archivo es el borde del modulo: lo unico que sabe que cask existe, lo unico que mira el
  * reloj y lo unico que toca el almacen. Todo lo de adentro (`Batch`, los `GameAnalyzer`,
  * `Numeros`) son funciones puras sobre datos inmutables y no importan nada de aqui.
  *
  * Endpoints, segun `contracts/results.md`:
  *
  *   - `POST /analyze` recibe el lote de una partida, lo analiza, lo guarda y lo devuelve.
  *   - `GET /results/matches/{matchId}` el resultado de una partida analizada.
  *   - `GET /results/players/{playerId}` el agregado de un jugador.
  *   - `GET /health` para docker compose y el CI.
  */
object Main extends cask.MainRoutes:

  private val config    = Config.fromSystemEnv()
  private val resultados = ResultStore()

  /** El reloj entra como funcion para que los analizadores sigan siendo puros: ellos reciben
    * `ahora` en vez de llamar a `Instant.now()` por dentro.
    *
    * Truncado a milisegundos: `Instant.now()` trae nanosegundos y el contrato usa milisegundos en
    * todas sus marcas de tiempo, igual que los `timestamp` de los eventos.
    */
  private val reloj: () => String = () => Instant.now().truncatedTo(ChronoUnit.MILLIS).toString

  override def port: Int = config.port

  /** cask escucha en `localhost` por defecto. Dentro de un contenedor eso solo acepta conexiones
    * del propio contenedor, de modo que el servicio de Go no podria alcanzarnos y el healthcheck
    * de docker compose fallaria. `0.0.0.0` escucha en todas las interfaces.
    */
  override def host: String = "0.0.0.0"

  @cask.get("/health")
  def health(): cask.Response[String] =
    json(Health(status = "ok", service = "analytics-scala"))

  /** Analiza el lote de una partida terminada.
    *
    * Las dos razones de 400 son distintas y conviene distinguirlas: el lote no cumple el contrato,
    * o el juego no tiene analizador. La segunda le dice a Go que mando un `gameId` que este modulo
    * todavia no sabe analizar, que es un problema de despliegue y no de datos.
    */
  @cask.post("/analyze")
  def analyze(request: cask.Request): cask.Response[String] =
    val cuerpo = new String(request.readAllBytes(), StandardCharsets.UTF_8)

    Batch.parse(cuerpo) match
      case Left(error) =>
        json(ErrorBody(s"lote invalido: $error"), 400)

      case Right(lote) =>
        GameAnalyzer.para(lote.gameId) match
          case None =>
            val conocidos = GameAnalyzer.conocidos.mkString(", ")
            json(ErrorBody(s"no hay analizador para '${lote.gameId}'; conocidos: $conocidos"), 400)

          case Some(analizador) =>
            val resultado = analizador.analizar(lote, reloj)
            resultados.guardar(resultado)
            json(resultado)

  @cask.get("/results/matches/:matchId")
  def resultadoDePartida(matchId: String): cask.Response[String] =
    resultados
      .partida(matchId)
      .fold(json(ErrorBody(s"no hay resultados para la partida '$matchId'"), 404))(json(_))

  @cask.get("/results/players/:playerId")
  def resultadoDeJugador(playerId: String): cask.Response[String] =
    resultados
      .jugador(playerId)
      .fold(json(ErrorBody(s"el jugador '$playerId' no aparece en ninguna partida analizada"), 404))(json(_))

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
