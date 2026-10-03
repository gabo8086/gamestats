# Imagen del modulo de ingestion (Go).
#
# Multi-stage: la primera etapa trae el compilador de Go (~350 MB) y la segunda solo el binario.
# El binario se compila estatico (CGO_ENABLED=0), asi que la imagen final no necesita toolchain.

FROM golang:1.24-alpine AS build

WORKDIR /src

# go.mod primero: mientras no cambie, Docker reutiliza esta capa y no vuelve a resolver modulos.
COPY go.mod ./
RUN go mod download

COPY . .

# -trimpath quita las rutas absolutas del binario (compilacion reproducible).
# -s -w quitan la tabla de simbolos y la informacion de depuracion.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/ingest .

FROM alpine:3.21

# Usuario sin privilegios: el servicio no necesita root para escuchar en 8080.
RUN adduser -D -u 10001 gamestats

COPY --from=build /out/ingest /usr/local/bin/ingest

USER gamestats
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/ingest"]
