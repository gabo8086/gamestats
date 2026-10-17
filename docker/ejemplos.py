"""Manda a Go los ejemplos del contrato, para probar el sistema entero con datos conocidos.

Dos partes:

  1. Las partidas de contracts/examples/*-match-ok.json: los cuatro juegos, incluidos blackjack y
     battleship, que el simulador todavia no genera. Cada lote se manda como un arreglo de eventos
     a POST /events, igual que lo haria la fuente externa.
  2. Los casos de invalid-events.json, cada uno con sus eventos previos, comparando la razon de
     rechazo que devuelve Go con la que espera el contrato. Al final, un cuerpo que no es JSON para
     que tambien aparezca malformed_json en GET /metrics.

Uso: python3 ejemplos.py http://127.0.0.1:8080
Solo biblioteca estandar, igual que el simulador.
"""

import json
import sys
import urllib.error
import urllib.request
from pathlib import Path

EJEMPLOS = Path(__file__).resolve().parent.parent / "contracts" / "examples"


def enviar(url: str, cuerpo: bytes) -> tuple[int, dict]:
    pedido = urllib.request.Request(f"{url}/events", data=cuerpo, method="POST",
                                    headers={"Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(pedido, timeout=10) as respuesta:
            return respuesta.status, json.load(respuesta)
    except urllib.error.HTTPError as error:
        return error.code, json.load(error)


def mandar_partidas(url: str) -> int:
    fallos = 0
    for archivo in sorted(EJEMPLOS.glob("*-match-ok.json")):
        lote = json.loads(archivo.read_text(encoding="utf-8"))
        status, respuesta = enviar(url, json.dumps(lote["events"]).encode())
        rechazados = respuesta.get("rejected", [])
        print(f"  {lote['matchId']:<24} {lote['gameId']:<11} HTTP {status}: "
              f"{respuesta.get('accepted', 0)} aceptados, {len(rechazados)} rechazados")
        for r in rechazados:
            print(f"      {r.get('eventId')}: {r.get('reason')} ({r.get('detail')})")
        fallos += len(rechazados)
    return fallos


def mandar_invalidos(url: str) -> int:
    casos = json.loads((EJEMPLOS / "invalid-events.json").read_text(encoding="utf-8"))["cases"]
    fallos = 0
    for caso in casos:
        if caso["precedingEvents"]:
            enviar(url, json.dumps(caso["precedingEvents"]).encode())
        _, respuesta = enviar(url, json.dumps([caso["event"]]).encode())
        razones = [r.get("reason") for r in respuesta.get("rejected", [])]
        obtenida = razones[0] if razones else "aceptado"
        ok = obtenida == caso["expectedReason"]
        fallos += 0 if ok else 1
        print(f"  {'ok ' if ok else 'MAL'} {caso['expectedReason']:<30} -> {obtenida}")

    _, respuesta = enviar(url, b"{esto no es json")
    razones = [r.get("reason") for r in respuesta.get("rejected", [])]
    ok = razones == ["malformed_json"]
    fallos += 0 if ok else 1
    print(f"  {'ok ' if ok else 'MAL'} {'malformed_json':<30} -> {razones[0] if razones else 'aceptado'}")
    return fallos


def main() -> int:
    url = sys.argv[1].rstrip("/") if len(sys.argv) > 1 else "http://127.0.0.1:8080"

    print(f"partidas de ejemplo -> {url}/events")
    fallos = mandar_partidas(url)
    print(f"\ncasos invalidos -> {url}/events")
    fallos += mandar_invalidos(url)

    if fallos:
        print(f"\n{fallos} resultados distintos de lo esperado", file=sys.stderr)
        return 1
    print("\ntodo como dice el contrato")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
