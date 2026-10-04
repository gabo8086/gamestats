# Imagen del modulo de analisis (Scala).
#
# Multi-stage: la primera etapa tiene sbt y el compilador de Scala (~1 GB) y produce un jar unico
# con sbt-assembly; la segunda solo necesita un JRE y ese jar.
# El tag fija Java 21, sbt 1.13.0 y Scala 3.3.8, las mismas versiones de build.sbt.

FROM sbtscala/scala-sbt:eclipse-temurin-21.0.12_8_1.13.0_3.3.8 AS build

# La imagen de sbt corre como usuario sin privilegios; necesitamos escribir en /src y en el cache.
USER root
WORKDIR /src

# La definicion del build primero: mientras no cambie, Docker reutiliza las dependencias ya bajadas.
COPY project/build.properties project/plugins.sbt ./project/
COPY build.sbt ./
RUN sbt update

COPY src ./src
# El jar sale en target/scala-<version>/; lo movemos a una ruta fija para no repetir la version aqui.
RUN sbt assembly && mkdir -p /out && cp target/scala-*/analytics.jar /out/analytics.jar

FROM eclipse-temurin:21-jre-jammy

# curl solo para el healthcheck de docker compose; la imagen base no trae ningun cliente HTTP.
RUN apt-get update \
 && apt-get install -y --no-install-recommends curl \
 && rm -rf /var/lib/apt/lists/*

RUN useradd --system --uid 10001 gamestats

COPY --from=build /out/analytics.jar /app/analytics.jar

USER gamestats
EXPOSE 8081

ENTRYPOINT ["java", "-jar", "/app/analytics.jar"]
