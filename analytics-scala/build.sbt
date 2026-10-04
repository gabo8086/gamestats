// Modulo de analisis de GameStats (paradigma funcional).
//
// Versiones: Scala 3.3.8 es la ultima de la linea LTS y MUnit 1.3.6 la ultima estable.
// Dependencias a proposito minimas: por ahora solo la de pruebas. La biblioteca de HTTP y la de
// JSON estan sin decidir (decision 9 de la Fase 0, pendiente de revision de Samuel); los
// candidatos y el porque estan en el README de esta carpeta.

ThisBuild / scalaVersion := "3.3.8"
ThisBuild / version      := "0.1.0-SNAPSHOT"
ThisBuild / organization := "gamestats"

lazy val root = (project in file("."))
  .settings(
    name := "analytics",
    libraryDependencies += "org.scalameta" %% "munit" % "1.3.6" % Test,
    scalacOptions ++= Seq(
      "-deprecation",
      "-feature",
      "-unchecked"
    ),
    Compile / mainClass        := Some("gamestats.analytics.Main"),
    assembly / mainClass       := Some("gamestats.analytics.Main"),
    assembly / assemblyJarName := "analytics.jar"
  )
