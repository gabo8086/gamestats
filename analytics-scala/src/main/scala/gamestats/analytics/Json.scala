package gamestats.analytics

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

  private def describir(error: Throwable): String =
    Option(error.getMessage).filter(_.nonEmpty).getOrElse(error.toString)
