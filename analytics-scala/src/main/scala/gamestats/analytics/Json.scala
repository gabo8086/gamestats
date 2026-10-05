package gamestats.analytics

import upickle.core.AbortException
import upickle.default.{Reader, read}
import scala.util.Try

/** Decodificacion de JSON sin excepciones.
  *
  * upickle lanza cuando el JSON no calza con el tipo. Aqui esa excepcion se convierte una sola vez
  * en `Either`, que es lo que el resto del modulo maneja (regla de estilo del CLAUDE.md: `Option` /
  * `Either` en vez de excepciones). Es el unico lugar del modulo que atrapa una excepcion.
  */
object Json:

  def decode[A: Reader](texto: String): Either[String, A] =
    Try(read[A](texto)).toEither.left.map(describir)

  /** Traduce la excepcion a algo que sirva para depurar del otro lado del cable.
    *
    * upickle envuelve el error real en un `TraceException` cuyo `getMessage` es solo la ruta dentro
    * del documento: para un JSON mal formado eso es `"$"`, que en un 400 no le dice nada a nadie. La
    * excepcion con la informacion util va en la causa, asi que hay que recorrer la cadena.
    */
  private def describir(error: Throwable): String =
    causas(error)
      .collectFirst {
        case e: AbortException       => s"${e.clue}${ubicacion(e.line, e.col, e.index)}"
        case e: ujson.ParseException => s"JSON mal formado: ${e.clue} (posicion ${e.index})"
      }
      .getOrElse(Option(error.getMessage).filter(_.nonEmpty).getOrElse(error.toString))

  /** Donde fallo, con lo que upickle sepa. Cuando no conoce la linea las deja en -1, y un
    * "(linea -1, columna -1)" en un mensaje de error es peor que no decir nada.
    */
  private def ubicacion(linea: Int, columna: Int, indice: Int): String =
    if linea >= 0 then s" (linea $linea, columna $columna)"
    else if indice >= 0 then s" (posicion $indice)"
    else ""

  /** La excepcion y sus causas. El `take` evita quedarse dando vueltas si alguna se apunta a si
    * misma; cinco niveles son de sobra para lo que anida upickle.
    */
  private def causas(error: Throwable): LazyList[Throwable] =
    LazyList.unfold(Option(error))(_.map(actual => (actual, Option(actual.getCause)))).take(5)
