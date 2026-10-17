#!/usr/bin/env bash
# Healthcheck de la imagen: revisa los servicios del modo con que arranco el contenedor, que el
# punto de entrada deja escrito en /tmp/gamestats-mode.

INGEST_PORT="${INGEST_PORT:-8080}"
ANALYTICS_PORT="${ANALYTICS_PORT:-8081}"

vivo() { curl -fsS -o /dev/null "http://127.0.0.1:$1/health"; }

case "$(cat /tmp/gamestats-mode 2>/dev/null)" in
  all)       vivo "$ANALYTICS_PORT" && vivo "$INGEST_PORT" ;;
  analytics) vivo "$ANALYTICS_PORT" ;;
  ingest)    vivo "$INGEST_PORT" ;;
  # simulator y ejemplos son procesos que terminan solos: no hay servicio que revisar.
  *)         exit 0 ;;
esac
