#!/usr/bin/env bash
# Punto de entrada de la imagen unica de GameStats. El primer argumento elige el modo; el resto se
# le pasa al programa de ese modo.
#
#   all        Go y Scala juntos, en dos procesos (por defecto)
#   analytics  solo Scala, en ANALYTICS_PORT
#   ingest     solo Go, en INGEST_PORT; ANALYTICS_URL dice donde esta Scala
#   simulator  el simulador; los argumentos los lista con -h
#   ejemplos   manda a Go las partidas y los casos invalidos de contracts/examples/
#
# Los dos servicios leen el puerto de PORT. En el modo `all` corren en el mismo contenedor, asi que
# cada uno recibe su PORT al lanzarlo, a partir de INGEST_PORT y ANALYTICS_PORT.

set -uo pipefail

INGEST_PORT="${INGEST_PORT:-8080}"
ANALYTICS_PORT="${ANALYTICS_PORT:-8081}"

modo="${1:-all}"
[ $# -gt 0 ] && shift

# El healthcheck lo lee para saber que revisar: no ve los argumentos con los que arranco el
# contenedor.
echo "$modo" > /tmp/gamestats-mode

esperar_analytics() {
  local url="http://127.0.0.1:${ANALYTICS_PORT}/health"
  for _ in $(seq 60); do
    curl -fsS -o /dev/null "$url" 2>/dev/null && return 0
    kill -0 "$pid_analytics" 2>/dev/null || return 1
    sleep 1
  done
  return 1
}

case "$modo" in
  analytics)
    PORT="$ANALYTICS_PORT" exec java -jar /app/analytics.jar
    ;;

  ingest)
    PORT="$INGEST_PORT" exec ingest
    ;;

  simulator)
    # Fuera de compose, Go esta en este mismo contenedor.
    export INGEST_URL="${INGEST_URL:-http://127.0.0.1:${INGEST_PORT}}"
    # El simulador escribe en ./out; aqui es donde la imagen dejo permiso de escritura.
    cd /app/simulator
    exec /usr/local/bin/simulador "$@"
    ;;

  ejemplos)
    exec python3 /app/docker/ejemplos.py "${INGEST_URL:-http://127.0.0.1:${INGEST_PORT}}"
    ;;

  all)
    # Scala primero y Go cuando Scala ya responde, para que el primer POST /analyze no se pierda.
    # Es lo mismo que hace el depends_on del docker-compose.yml, pero dentro del contenedor.
    parando=0
    pid_ingest=""
    trap 'parando=1; kill -TERM $pid_analytics $pid_ingest 2>/dev/null' TERM INT

    PORT="$ANALYTICS_PORT" java -jar /app/analytics.jar &
    pid_analytics=$!

    if ! esperar_analytics; then
      [ "$parando" -eq 1 ] && exit 0
      echo "gamestats: analytics no respondio en ${ANALYTICS_PORT}" >&2
      kill -TERM "$pid_analytics" 2>/dev/null
      exit 1
    fi

    PORT="$INGEST_PORT" ANALYTICS_URL="http://127.0.0.1:${ANALYTICS_PORT}" ingest &
    pid_ingest=$!

    # Si uno de los dos se cae, se baja el otro y el contenedor termina con error: un contenedor
    # con medio sistema vivo pareceria sano y no lo esta.
    wait -n
    estado=$?
    kill -TERM $pid_analytics $pid_ingest 2>/dev/null
    wait
    [ "$parando" -eq 1 ] && exit 0
    echo "gamestats: un servicio termino (codigo $estado); se detiene el contenedor" >&2
    exit 1
    ;;

  help|-h|--help)
    sed -n '2,9p' "$0" | sed 's/^# \{0,1\}//'
    ;;

  *)
    echo "gamestats: modo desconocido '$modo'. Modos: all, analytics, ingest, simulator, ejemplos, help" >&2
    exit 2
    ;;
esac
