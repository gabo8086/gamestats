// Modulo de analisis de GameStats (paradigma funcional).
//
// Versiones: Scala 3.3.8 es la ultima de la linea LTS y MUnit 1.3.6 la ultima estable.
// HTTP y JSON: cask 0.11.3 y upickle 4.4.3 (decision 9 de la Fase 0, ya cerrada). El porque de
// esas dos y no http4s + circe esta en el README de esta carpeta.

ThisBuild / scalaVersion := "3.3.8"
ThisBuild / version      := "0.1.0-SNAPSHOT"
ThisBuild / organization := "gamestats"

lazy val root = (project in file("."))
  .settings(
    name := "analytics",
    libraryDependencies ++= Seq(
      "com.lihaoyi" %% "cask"    % "0.11.3",
      "com.lihaoyi" %% "upickle" % "4.4.3",
      "org.scalameta" %% "munit" % "1.3.6" % Test
    ),
    scalacOptions ++= Seq(
      "-deprecation",
      "-feature",
      "-unchecked"
    ),
    Compile / mainClass        := Some("gamestats.analytics.Main"),
    assembly / mainClass       := Some("gamestats.analytics.Main"),
    assembly / assemblyJarName := "analytics.jar"
  )
