# Imagen unica de GameStats: ingestion (Go), analisis (Scala) y simulador, todo en una imagen.
#
# Multi-stage: una etapa compila Go, otra compila Scala, y la final solo trae lo necesario para
# correr: un JRE para el jar de Scala y los binarios estaticos de Go (ingestion y simulador).
# Los compiladores (~1.3 GB entre los dos) no llegan a la imagen final.
#
# Una imagen no quiere decir un proceso: el punto de entrada (docker/entrypoint.sh) elige que
# levantar. Por defecto levanta los dos servicios, cada uno en su propio proceso y hablandose por
# HTTP; docker compose usa esta misma imagen para correrlos en contenedores separados.
#
#   docker build -t gamestats .
#   docker run --rm -p 8080:8080 -p 8081:8081 gamestats          # Go + Scala
#   docker run --rm gamestats help                               # los demas modos

# Multi-arquitectura (amd64 y arm64): las dos etapas de compilacion corren en la arquitectura de la
# maquina que construye ($BUILDPLATFORM), sin emulacion. Go compila cruzado para la arquitectura de
# destino y el jar de Scala es el mismo en cualquiera, asi que solo la etapa final es por
# arquitectura.

# ---------------------------------------------------------------------------- Go
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS go-build
ARG TARGETOS=linux
ARG TARGETARCH

WORKDIR /src

# go.mod primero: mientras no cambie, Docker reutiliza esta capa y no vuelve a resolver modulos.
COPY ingest-go/go.mod ./
RUN go mod download

COPY ingest-go/ ./

# CGO_ENABLED=0 da un binario estatico, que corre en cualquier base sin el toolchain de Go.
# -trimpath quita las rutas absolutas del binario; -s -w quitan simbolos y depuracion.
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/ingest .

# El simulador es un modulo aparte, con su propio go.mod y sin dependencias externas, asi que
# no hace falta go mod download.
WORKDIR /src/simulator
COPY simulator/go.mod ./
COPY simulator/*.go ./
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/simulador .

# ---------------------------------------------------------------------------- Scala
# El tag fija Java 21, sbt 1.13.0 y Scala 3.3.8, las mismas versiones de build.sbt.
FROM --platform=$BUILDPLATFORM sbtscala/scala-sbt:eclipse-temurin-21.0.12_8_1.13.0_3.3.8 AS scala-build

# La imagen de sbt corre como usuario sin privilegios; necesitamos escribir en /src y en el cache.
USER root
WORKDIR /src

# La definicion del build primero: mientras no cambie, Docker reutiliza las dependencias ya bajadas.
COPY analytics-scala/project/build.properties analytics-scala/project/plugins.sbt ./project/
COPY analytics-scala/build.sbt ./
RUN sbt update

# Solo src/main: las pruebas corren en el CI, no en la imagen.
COPY analytics-scala/src/main ./src/main
# El jar sale en target/scala-<version>/; lo movemos a una ruta fija para no repetir la version aqui.
RUN sbt assembly && mkdir -p /out && cp target/scala-*/analytics.jar /out/analytics.jar

# ---------------------------------------------------------------------------- final
# Ubuntu 24.04 con el JRE 21: trae bash y su apt tiene Python 3.12, que usa el modo `ejemplos`.
FROM eclipse-temurin:21-jre-noble

# curl para el healthcheck y para esperar a Scala al arrancar; python3 para docker/ejemplos.py, que solo
# usa la biblioteca estandar.
RUN apt-get update \
 && apt-get install -y --no-install-recommends curl python3 \
 && rm -rf /var/lib/apt/lists/*

RUN useradd --system --uid 10001 gamestats

COPY --from=go-build    /out/ingest        /usr/local/bin/ingest
COPY --from=scala-build /out/analytics.jar /app/analytics.jar
COPY --from=go-build    /out/simulador     /usr/local/bin/simulador
# Los ejemplos del contrato viajan en la imagen para el modo `ejemplos`: son las unicas partidas de
# blackjack y battleship hasta que el simulador cubra los cuatro juegos.
COPY contracts/examples/                   /app/contracts/examples/
COPY docker/ejemplos.py                    /app/docker/ejemplos.py
COPY --chmod=755 docker/entrypoint.sh      /usr/local/bin/gamestats
COPY --chmod=755 docker/healthcheck.sh     /usr/local/bin/gamestats-health

# El simulador escribe sus JSON en /app/simulator/out. Sin esto el usuario sin privilegios no puede
# crear la carpeta dentro de /app, que es de root, y el proceso muere con un Errno 13.
RUN mkdir -p /app/simulator/out && chown gamestats:gamestats /app/simulator/out

USER gamestats
WORKDIR /app

ENV INGEST_PORT=8080 \
    ANALYTICS_PORT=8081
EXPOSE 8080 8081

# El script sabe que modo se levanto y revisa solo los servicios de ese modo.
HEALTHCHECK --interval=5s --timeout=3s --start-period=20s --retries=12 CMD ["gamestats-health"]

ENTRYPOINT ["gamestats"]
CMD ["all"]
