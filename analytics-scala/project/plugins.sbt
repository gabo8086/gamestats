// sbt-assembly produce un jar unico con sus dependencias adentro. Hace falta para la imagen de
// Docker: asi la etapa final solo necesita un JRE y un archivo, sin sbt ni el proyecto completo.
addSbtPlugin("com.eed3si9n" % "sbt-assembly" % "2.5.0")
