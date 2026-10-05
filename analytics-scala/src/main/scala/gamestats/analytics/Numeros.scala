package gamestats.analytics

import java.time.{Instant, OffsetDateTime}
import scala.math.BigDecimal.RoundingMode
import scala.util.Try

/** Convenciones numericas de `contracts/results.md`, en un solo sitio.
  *
  * Estan aqui y no repartidas por los analizadores para que redondear distinto en dos lugares sea
  * imposible: es lo que hacia fallar las pruebas de integracion por un decimal.
  */
object Numeros:

  /** Redondeo de presentacion: 2 decimales. El calculo interno usa `Double` sin redondear. */
  def dosDecimales(valor: Double): Double =
    BigDecimal(valor).setScale(2, RoundingMode.HALF_UP).toDouble

  /** Promedio de una muestra. `None` si no hay muestras: no es lo mismo que un promedio de cero,
    * y el contrato pide `null` en ese caso.
    */
  def promedio(valores: Seq[Double]): Option[Double] =
    Option.when(valores.nonEmpty)(valores.sum / valores.size)

  /** Desviacion estandar **poblacional**: divide entre `n`, no entre `n-1`. Con una sola muestra da
    * `0.0`, que es lo que el contrato especifica para la consistencia.
    */
  def desviacionPoblacional(valores: Seq[Double]): Option[Double] =
    promedio(valores).map { media =>
      math.sqrt(valores.map(v => math.pow(v - media, 2)).sum / valores.size)
    }

/** Instantes RFC 3339. */
object Tiempo:

  /** Acepta tanto `...Z` como desplazamientos `+05:00`, los dos permitidos por el contrato. */
  def instante(texto: String): Option[Instant] =
    Try(OffsetDateTime.parse(texto).toInstant).toOption

  /** Milisegundos entre dos marcas, si ambas se pueden leer. */
  def duracionMs(desde: String, hasta: String): Option[Long] =
    for
      a <- instante(desde)
      b <- instante(hasta)
    yield b.toEpochMilli - a.toEpochMilli
