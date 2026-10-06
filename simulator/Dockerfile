# Imagen del simulador de eventos.
#
# Una sola etapa: el simulador es un script de Python con biblioteca estandar, no hay nada que
# compilar ni dependencias que instalar. La imagen slim alcanza y pesa poco.

FROM python:3.13-slim

RUN useradd --system --uid 10002 gamestats
WORKDIR /app

COPY simulate.py ./

# La carpeta de salida se crea aqui y se le da al usuario sin privilegios. Si no, el proceso no
# puede crearla dentro de /app, que pertenece a root, y el contenedor muere con un Errno 13.
RUN mkdir -p /app/out && chown gamestats:gamestats /app/out

USER gamestats

# Sin argumentos manda los eventos a INGEST_URL, que el compose define, y ademas escribe los JSON
# en /app/out. Para solo enviar, sin escribir nada:
#   docker compose run --rm simulator --no-files
ENTRYPOINT ["python", "simulate.py"]
