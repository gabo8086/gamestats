package gamestats.analytics

/** Configuracion del modulo, leida del entorno.
  *
  * `fromEnv` recibe la funcion de lectura en vez de llamar a `sys.env` por dentro: asi es una
  * funcion pura y las pruebas no necesitan tocar variables de entorno de verdad. Es el patron que
  * usamos en todo el modulo: el calculo es puro y el IO queda en los bordes (ver Main).
  */
final case class Config(port: Int)

object Config:

  val PuertoPorDefecto: Int = 8081

  def fromEnv(leer: String => Option[String]): Config =
    val puerto = leer("PORT")
      .flatMap(_.toIntOption)
      .filter(p => p > 0 && p <= 65535)
      .getOrElse(PuertoPorDefecto)

    Config(puerto)

  /** Unico punto que toca el entorno real. */
  def fromSystemEnv(): Config =
    fromEnv(clave => sys.env.get(clave))
