package gamestats.analytics

import com.sun.net.httpserver.{HttpExchange, HttpServer}
import java.net.InetSocketAddress
import java.nio.charset.StandardCharsets

/** Modulo de analisis de GameStats (paradigma funcional).
  *
  * Estado: esqueleto de la Fase 0. Solo responde /health, lo justo para que docker compose y el CI
  * tengan algo que verificar. Lo que falta, en orden (ver CLAUDE.md):
  *
  *   - Dia 1: case classes inmutables del evento y del lote (event.schema.json, batch.schema.json).
  *   - Dia 2: `trait GameAnalyzer` con `RacingAnalyzer` y `CombatAnalyzer`; estadisticas y reglas con
  *     filter / map / groupBy / fold.
  *   - Dia 3: POST /analyze y los GET /results/... con el formato de contracts/results.md.
  *
  * El servidor usa `com.sun.net.httpserver`, que viene en el JDK, para no fijar todavia la
  * biblioteca de HTTP: esa decision es de Samuel (decision 9 de la Fase 0). Cuando se decida,
  * se reemplaza este archivo sin tocar el resto del modulo, porque el analisis no sabe de HTTP.
  */
object Main:

  def main(args: Array[String]): Unit =
    val config = Config.fromSystemEnv()
    val server = HttpServer.create(new InetSocketAddress(config.port), 0)

    server.createContext("/health", exchange => responder(exchange, 200, HealthJson))
    server.setExecutor(null) // un executor por defecto alcanza para el esqueleto
    server.start()

    println(s"analytics-scala escuchando en :${config.port}")

  private val HealthJson =
    """{"status":"ok","service":"analytics-scala"}"""

  private def responder(exchange: HttpExchange, status: Int, cuerpo: String): Unit =
    val bytes = cuerpo.getBytes(StandardCharsets.UTF_8)
    exchange.getResponseHeaders.add("Content-Type", "application/json")
    exchange.sendResponseHeaders(status, bytes.length.toLong)
    val salida = exchange.getResponseBody
    try salida.write(bytes)
    finally salida.close()
