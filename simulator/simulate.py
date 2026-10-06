#!/usr/bin/env python3
"""Simulador de eventos de GameStats.

Hace de sistema de juegos externo: GameStats no implementa los videojuegos, solo consume lo que
esta fuente produce (enunciado §8). El formato de lo que genera es `contracts/event.schema.json`.

Genera partidas de los dos juegos con ventanas de tiempo solapadas, de modo que los eventos de
distintos `matchId` quedan intercalados en el flujo: es lo que le da trabajo concurrente al modulo
de Go. Ademas de las partidas al azar incluye dos partidas **guionadas**, con los tiempos escritos a
mano, que garantizan que se disparen la remontada, la racha y la venganza. Que una regla se dispare
por casualidad no sirve para la demo.

Todo es determinista: con la misma semilla sale exactamente el mismo flujo. Eso permite repetir una
demo y escribir pruebas con resultados fijos.

Uso tipico:

    python simulate.py                          # escribe out/ con los JSON
    python simulate.py --analyze http://localhost:8081   # manda los lotes directo a Scala
    python simulate.py --post http://localhost:8080      # manda los eventos a Go, con retardo
"""

from __future__ import annotations

import argparse
import json
import os
import random
import sys
import time
import urllib.error
import urllib.request
from datetime import datetime, timedelta, timezone
from pathlib import Path

GAME_VERSION = "1.0"
PISTAS = ["Circuito Volcan", "Bahia Norte", "Valle Seco", "Puerto Viejo"]
ARENAS = ["Bodega", "Azotea", "Subsuelo", "Mercado"]
ARMAS = ["rifle", "shotgun", "pistol", "sniper"]


# --------------------------------------------------------------------------- utilidades

def rfc3339(momento: datetime) -> str:
    """Marca de tiempo en el formato que exige el contrato: UTC, milisegundos y sufijo Z."""
    utc = momento.astimezone(timezone.utc)
    return f"{utc:%Y-%m-%dT%H:%M:%S}.{utc.microsecond // 1000:03d}Z"


class Contador:
    """Genera `eventId` unicos. El prefijo `s-` distingue lo simulado de los ejemplos del contrato."""

    def __init__(self, prefijo: str = "s") -> None:
        self.prefijo = prefijo
        self.siguiente = 0

    def __call__(self) -> str:
        self.siguiente += 1
        return f"{self.prefijo}-{self.siguiente:06d}"


def evento(
    event_id: str,
    momento: datetime,
    game_id: str,
    match_id: str,
    tipo: str,
    player_id: str | None = None,
    data: dict | None = None,
) -> dict:
    """Arma el sobre comun. El orden de las claves sigue al del contrato, para que los archivos
    generados se lean igual que los ejemplos de `contracts/examples/`.

    `action` va siempre en `null`: hoy ningun juego la usa, pero es parte del sobre y el esquema es
    cerrado, asi que omitirla seria tan incorrecto como inventarla.
    """
    return {
        "eventId": event_id,
        "timestamp": rfc3339(momento),
        "gameId": game_id,
        "gameVersion": GAME_VERSION,
        "matchId": match_id,
        "playerId": player_id,
        "type": tipo,
        "action": None,
        "data": data,
    }


# --------------------------------------------------------------------------- carreras

def partida_de_carreras(
    nuevo_id: Contador,
    match_id: str,
    inicio: datetime,
    jugadores: list[str],
    tiempos_por_vuelta: dict[str, list[int]],
    pista: str,
    penalizaciones: list[tuple[str, int, str]] | None = None,
) -> list[dict]:
    """Construye una partida de carreras a partir de los tiempos de vuelta de cada jugador.

    La coherencia de los datos sale de aqui: la `position` de una vuelta **se deriva** del tiempo
    acumulado, no se inventa. Si p1 lleva menos tiempo acumulado que p2 al terminar la vuelta 2,
    entonces p1 va delante de p2 en esa vuelta. Sin esto las estadisticas salen contradictorias y la
    remontada no se puede justificar en la defensa.

    El `timestamp` de cada `LAP_COMPLETED` es el instante real en que ese jugador cruza la meta, o
    sea inicio + acumulado. Por eso los eventos de los jugadores quedan intercalados solos.
    """
    vueltas = len(next(iter(tiempos_por_vuelta.values())))
    eventos = [evento(nuevo_id(), inicio, "racing", match_id, "MATCH_STARTED",
                      data={"laps": vueltas, "track": pista})]

    for desplazamiento, jugador in enumerate(jugadores, start=1):
        eventos.append(evento(nuevo_id(), inicio + timedelta(milliseconds=200 * desplazamiento),
                              "racing", match_id, "PLAYER_JOINED", player_id=jugador))

    acumulado = {jugador: 0 for jugador in jugadores}
    for vuelta in range(vueltas):
        for jugador in jugadores:
            acumulado[jugador] += tiempos_por_vuelta[jugador][vuelta]

        # La posicion es el puesto por tiempo acumulado al cerrar esta vuelta.
        orden = sorted(jugadores, key=lambda j: acumulado[j])
        posicion = {jugador: puesto for puesto, jugador in enumerate(orden, start=1)}

        for jugador in jugadores:
            eventos.append(evento(
                nuevo_id(),
                inicio + timedelta(milliseconds=acumulado[jugador]),
                "racing", match_id, "LAP_COMPLETED", player_id=jugador,
                data={"lap": vuelta + 1,
                      "lapTimeMs": tiempos_por_vuelta[jugador][vuelta],
                      "position": posicion[jugador]},
            ))

    for jugador, segundos, razon in (penalizaciones or []):
        # La penalizacion se registra a mitad de carrera; no altera los tiempos de vuelta, solo el
        # tiempo ajustado que calcula Scala.
        eventos.append(evento(
            nuevo_id(), inicio + timedelta(milliseconds=acumulado[jugador] // 2),
            "racing", match_id, "PENALTY", player_id=jugador,
            data={"seconds": segundos, "reason": razon},
        ))

    fin = inicio + timedelta(milliseconds=max(acumulado.values()) + 5_000)
    eventos.append(evento(nuevo_id(), fin, "racing", match_id, "MATCH_FINISHED"))
    return eventos


def carrera_con_remontada(nuevo_id: Contador, match_id: str, inicio: datetime) -> list[dict]:
    """Partida guionada: p3 termina la primera vuelta ultimo y gana la carrera.

    Los tiempos estan escritos a mano para que la remontada ocurra con seguridad. Acumulados:

        vuelta 1   p1 80000 (1.o)   p2 81000 (2.o)   p3 90000 (3.o, ultimo)
        vuelta 2   p2 163000 (1.o)  p1 165000 (2.o)  p3 166000 (3.o)
        vuelta 3   p3 238000 (1.o)  p2 246000 (2.o)  p1 250000 (3.o)

    p3 estuvo en la ultima posicion (position == 3 == numero de jugadores) y gana por tiempo
    ajustado: es exactamente la definicion de remontada del CLAUDE.md.
    """
    return partida_de_carreras(
        nuevo_id, match_id, inicio,
        jugadores=["p1", "p2", "p3"],
        tiempos_por_vuelta={
            "p1": [80_000, 85_000, 85_000],
            "p2": [81_000, 82_000, 83_000],
            "p3": [90_000, 76_000, 72_000],
        },
        pista="Circuito Volcan",
    )


def carrera_con_penalizacion_decisiva(nuevo_id: Contador, match_id: str, inicio: datetime) -> list[dict]:
    """Partida guionada: p1 llega primero por tiempo de vuelta pero pierde por la penalizacion.

    Acumulados sin penalizar: p1 240000, p2 243000. Con los 5 s de p1, el tiempo ajustado queda
    p1 245000 contra p2 243000, asi que gana p2. Es el caso que demuestra que el ganador sale de
    combinar dos tipos de evento distintos (LAP_COMPLETED + PENALTY) y no de uno solo.
    """
    return partida_de_carreras(
        nuevo_id, match_id, inicio,
        jugadores=["p1", "p2", "p4"],
        tiempos_por_vuelta={
            "p1": [80_000, 80_000, 80_000],
            "p2": [81_000, 81_000, 81_000],
            "p4": [84_000, 83_000, 85_000],
        },
        pista="Bahia Norte",
        penalizaciones=[("p1", 5, "track_cut")],
    )


def carrera_al_azar(nuevo_id: Contador, match_id: str, inicio: datetime,
                    jugadores: list[str], vueltas: int, azar: random.Random) -> list[dict]:
    """Carrera sin guion: cada jugador tiene un ritmo base y variacion vuelta a vuelta."""
    ritmo = {jugador: azar.randint(74_000, 92_000) for jugador in jugadores}
    tiempos = {
        jugador: [ritmo[jugador] + azar.randint(-3_000, 4_000) for _ in range(vueltas)]
        for jugador in jugadores
    }
    penalizaciones = []
    if azar.random() < 0.4:
        penalizaciones.append((azar.choice(jugadores), azar.choice([3, 5, 6, 10]),
                               azar.choice(["track_cut", "collision", "speeding_pit"])))
    return partida_de_carreras(nuevo_id, match_id, inicio, jugadores, tiempos,
                               azar.choice(PISTAS), penalizaciones)


# --------------------------------------------------------------------------- combate

def partida_de_combate(
    nuevo_id: Contador,
    match_id: str,
    inicio: datetime,
    jugadores: list[str],
    bajas: list[tuple[int, str, str, str, int]],
    arena: str,
    modo: str = "deathmatch",
) -> list[dict]:
    """Construye una partida de combate a partir de la lista de bajas.

    Cada baja es `(segundo, autor, victima, arma, dano)`. El `playerId` del sobre es **quien
    elimina** y la victima va en `data`, tal como fija el contrato.
    """
    eventos = [evento(nuevo_id(), inicio, "combat", match_id, "MATCH_STARTED",
                      data={"mode": modo, "arena": arena})]

    for desplazamiento, jugador in enumerate(jugadores, start=1):
        eventos.append(evento(nuevo_id(), inicio + timedelta(milliseconds=200 * desplazamiento),
                              "combat", match_id, "PLAYER_JOINED", player_id=jugador))

    for segundo, autor, victima, arma, dano in bajas:
        eventos.append(evento(
            nuevo_id(), inicio + timedelta(seconds=segundo),
            "combat", match_id, "PLAYER_ELIMINATED", player_id=autor,
            data={"victimId": victima, "weapon": arma, "damage": dano},
        ))

    ultima = max((segundo for segundo, *_ in bajas), default=0)
    eventos.append(evento(nuevo_id(), inicio + timedelta(seconds=ultima + 10),
                          "combat", match_id, "MATCH_FINISHED"))
    return eventos


def combate_con_racha_y_venganza(nuevo_id: Contador, match_id: str, inicio: datetime) -> list[dict]:
    """Partida guionada que dispara las dos reglas de combate.

    Racha de p3: elimina en los segundos 50, 53 y 58 — tres bajas en una ventana de 8 s — y no
    aparece como victima entre medio (lo matan recien en el segundo 80).

    Venganzas: p4 elimina a p1 en el segundo 15 y p1 se lo cobra en el 30; p3 elimina a p2 en el 50
    y p2 se lo cobra en el 80.
    """
    return partida_de_combate(
        nuevo_id, match_id, inicio,
        jugadores=["p1", "p2", "p3", "p4"],
        bajas=[
            (15, "p4", "p1", "pistol", 70),
            (30, "p1", "p4", "rifle", 115),
            (50, "p3", "p2", "shotgun", 90),
            (53, "p3", "p4", "shotgun", 105),
            (58, "p3", "p1", "rifle", 125),
            (80, "p2", "p3", "sniper", 140),
        ],
        arena="Azotea",
    )


def combate_al_azar(nuevo_id: Contador, match_id: str, inicio: datetime,
                    jugadores: list[str], azar: random.Random) -> list[dict]:
    """Combate sin guion. Las bajas se reparten al azar y las reglas pueden o no dispararse."""
    bajas = []
    segundo = azar.randint(8, 20)
    for _ in range(azar.randint(4, 9)):
        autor, victima = azar.sample(jugadores, 2)
        bajas.append((segundo, autor, victima, azar.choice(ARMAS), azar.randint(60, 150)))
        segundo += azar.randint(4, 25)
    return partida_de_combate(nuevo_id, match_id, inicio, jugadores, bajas, azar.choice(ARENAS))


# --------------------------------------------------------------------------- eventos invalidos

def casos_invalidos(base: datetime) -> dict:
    """Un caso por cada razon de `contracts/rejections.md`.

    Mismo formato que `contracts/examples/invalid-events.json`, para que las dos colecciones se
    puedan usar indistintamente. `validAgainstSchema` dice si el evento es estructuralmente valido:
    los que llevan `true` solo se pueden rechazar con el estado de la partida que mantiene Go, no
    con el esquema.

    `malformed_json` es el unico que no se puede representar como objeto, porque justamente no es
    JSON valido: va en `rawBody` en vez de en `event`.
    """
    momento = rfc3339(base)
    match_id = "m-sim-invalid"

    def sobre(**campos) -> dict:
        completo = {
            "eventId": "x-000000",
            "timestamp": momento,
            "gameId": "racing",
            "gameVersion": GAME_VERSION,
            "matchId": match_id,
            "playerId": None,
            "type": "MATCH_STARTED",
            "action": None,
            "data": None,
        }
        completo.update(campos)
        return completo

    arranque = sobre(eventId="x-000100", data={"laps": 3, "track": "Valle Seco"})

    return {
        "$comment": (
            "Generado por simulator/simulate.py. Un caso por cada razon de contracts/rejections.md. "
            "'precedingEvents' son los eventos que Go debe haber procesado antes para que el rechazo "
            "ocurra; 'rawBody' aparece solo en malformed_json, que no es representable como objeto."
        ),
        "cases": [
            {
                "expectedReason": "malformed_json",
                "note": "el cuerpo se corta a la mitad",
                "validAgainstSchema": False,
                "precedingEvents": [],
                "rawBody": '{"eventId": "x-000001", "timestamp":',
            },
            {
                "expectedReason": "missing_required_field",
                "note": "falta matchId",
                "validAgainstSchema": False,
                "precedingEvents": [],
                "event": {k: v for k, v in sobre(eventId="x-000002").items() if k != "matchId"},
            },
            {
                "expectedReason": "invalid_timestamp",
                "note": "fecha en formato local, no RFC 3339",
                "validAgainstSchema": False,
                "precedingEvents": [],
                "event": sobre(eventId="x-000003", timestamp="05/10/2026 16:00:00"),
            },
            {
                "expectedReason": "missing_player_id",
                "note": "LAP_COMPLETED no es MATCH_*, asi que playerId es obligatorio",
                "validAgainstSchema": False,
                "precedingEvents": [arranque],
                "event": sobre(eventId="x-000004", type="LAP_COMPLETED", playerId=None,
                               data={"lap": 1, "lapTimeMs": 80_000, "position": 1}),
            },
            {
                "expectedReason": "unknown_field",
                "note": "el sobre es cerrado: un campo de mas se rechaza, no se ignora",
                "validAgainstSchema": False,
                "precedingEvents": [],
                "event": sobre(eventId="x-000005", **{"serverRegion": "cr-central"}),
            },
            {
                "expectedReason": "duplicate_event",
                "note": "el mismo eventId que el evento anterior",
                "validAgainstSchema": True,
                "precedingEvents": [arranque],
                "event": sobre(eventId="x-000100", data={"laps": 3, "track": "Valle Seco"}),
            },
            {
                "expectedReason": "match_not_started",
                "note": "primer evento de la partida y no es MATCH_STARTED",
                "validAgainstSchema": True,
                "precedingEvents": [],
                "event": sobre(eventId="x-000006", matchId="m-sim-sin-inicio",
                               type="PLAYER_JOINED", playerId="p1"),
            },
            {
                "expectedReason": "match_already_finished",
                "note": "llega una vuelta despues del MATCH_FINISHED de esa partida",
                "validAgainstSchema": True,
                "precedingEvents": [
                    arranque,
                    sobre(eventId="x-000101", type="MATCH_FINISHED"),
                ],
                "event": sobre(eventId="x-000007", type="LAP_COMPLETED", playerId="p1",
                               data={"lap": 1, "lapTimeMs": 80_000, "position": 1}),
            },
        ],
    }


# --------------------------------------------------------------------------- flujo

def desordenar(eventos: list[dict], cuantos: int, azar: random.Random) -> list[dict]:
    """Intercambia pares de eventos adyacentes para que el flujo llegue desordenado.

    Go tiene que ordenar por `timestamp` antes de mandar el lote, asi que conviene darle trabajo.
    Pero el desorden no puede inventar rechazos falsos: si un evento se adelanta a su
    `MATCH_STARTED` o se atrasa detras de su `MATCH_FINISHED`, Go lo rechazaria con razon y no
    estariamos probando lo que queremos. Por eso solo se intercambian pares donde **ninguno** de los
    dos es un evento `MATCH_*`.
    """
    mezclados = list(eventos)
    candidatos = [
        i for i in range(len(mezclados) - 1)
        if not mezclados[i]["type"].startswith("MATCH_")
        and not mezclados[i + 1]["type"].startswith("MATCH_")
    ]
    for posicion in azar.sample(candidatos, min(cuantos, len(candidatos))):
        mezclados[posicion], mezclados[posicion + 1] = mezclados[posicion + 1], mezclados[posicion]
    return mezclados


def generar(semilla: int, partidas_al_azar: int, desorden: int) -> dict:
    """Genera el flujo completo. Devuelve los eventos, los lotes por partida y un resumen."""
    azar = random.Random(semilla)
    nuevo_id = Contador()
    base = datetime(2026, 10, 5, 14, 0, 0, tzinfo=timezone.utc)

    partidas: list[tuple[str, str, list[dict]]] = []

    # Guionadas primero, para que sus eventId sean estables aunque cambie el numero de partidas
    # al azar: asi una demo repetida habla siempre de los mismos identificadores.
    partidas.append(("m-sim-racing-comeback", "racing",
                     carrera_con_remontada(nuevo_id, "m-sim-racing-comeback", base)))
    partidas.append(("m-sim-racing-penalty", "racing",
                     carrera_con_penalizacion_decisiva(nuevo_id, "m-sim-racing-penalty",
                                                       base + timedelta(seconds=30))))
    partidas.append(("m-sim-combat-streak", "combat",
                     combate_con_racha_y_venganza(nuevo_id, "m-sim-combat-streak",
                                                  base + timedelta(seconds=15))))

    # Las partidas al azar arrancan dentro de los primeros dos minutos, de modo que sus ventanas se
    # solapan con las guionadas: eso es lo que deja los matchId intercalados en el flujo.
    for numero in range(partidas_al_azar):
        arranque = base + timedelta(seconds=azar.randint(0, 120))
        if numero % 2 == 0:
            match_id = f"m-sim-racing-{numero:02d}"
            jugadores = [f"p{n}" for n in range(1, azar.randint(3, 5) + 1)]
            partidas.append((match_id, "racing",
                             carrera_al_azar(nuevo_id, match_id, arranque, jugadores,
                                             azar.randint(3, 5), azar)))
        else:
            match_id = f"m-sim-combat-{numero:02d}"
            jugadores = [f"p{n}" for n in range(1, azar.randint(4, 6) + 1)]
            partidas.append((match_id, "combat",
                             combate_al_azar(nuevo_id, match_id, arranque, jugadores, azar)))

    lotes = {
        match_id: {
            "matchId": match_id,
            "gameId": game_id,
            "gameVersion": GAME_VERSION,
            "events": sorted(eventos, key=lambda e: e["timestamp"]),
        }
        for match_id, game_id, eventos in partidas
    }

    todos = [e for _, _, eventos in partidas for e in eventos]
    todos.sort(key=lambda e: e["timestamp"])
    flujo = desordenar(todos, desorden, azar)

    resumen = {
        "seed": semilla,
        "matches": len(partidas),
        "events": len(flujo),
        "swappedPairs": desorden,
        "scripted": {
            "m-sim-racing-comeback": "p3 termina ultimo la vuelta 1 y gana: dispara 'comeback'",
            "m-sim-racing-penalty": "p1 es el mas rapido pero la penalizacion de 5 s le da la victoria a p2",
            "m-sim-combat-streak": "p3 elimina 3 veces en 8 s sin morir: dispara 'streak'; ademas hay dos 'revenge'",
        },
        "byGame": {
            juego: sum(1 for _, g, _ in partidas if g == juego)
            for juego in ("racing", "combat")
        },
    }
    return {"events": flujo, "batches": lotes, "summary": resumen}


# --------------------------------------------------------------------------- salidas

def escribir(destino: Path, datos: dict, invalidos: dict) -> None:
    destino.mkdir(parents=True, exist_ok=True)
    (destino / "batches").mkdir(exist_ok=True)

    def volcar(ruta: Path, contenido) -> None:
        ruta.write_text(json.dumps(contenido, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

    volcar(destino / "events.json", datos["events"])
    volcar(destino / "invalid-events.json", invalidos)
    volcar(destino / "summary.json", datos["summary"])
    for match_id, lote in datos["batches"].items():
        volcar(destino / "batches" / f"{match_id}.json", lote)

    print(f"escrito en {destino}/")
    print(f"  events.json          {len(datos['events'])} eventos de {datos['summary']['matches']} partidas")
    print(f"  invalid-events.json  {len(invalidos['cases'])} casos")
    print(f"  batches/             {len(datos['batches'])} lotes listos para POST /analyze")


def enviar(url: str, cuerpo, descripcion: str) -> tuple[int, str]:
    datos = json.dumps(cuerpo, ensure_ascii=False).encode("utf-8")
    peticion = urllib.request.Request(url, data=datos, method="POST",
                                      headers={"Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(peticion, timeout=10) as respuesta:
            return respuesta.status, respuesta.read().decode("utf-8", "replace")
    except urllib.error.HTTPError as error:
        return error.code, error.read().decode("utf-8", "replace")
    except urllib.error.URLError as error:
        return 0, f"no se pudo conectar con {descripcion}: {error.reason}"


def mandar_lotes_a_scala(url_base: str, lotes: dict) -> int:
    """Manda cada lote a `POST /analyze` del modulo Scala, saltandose Go.

    Sirve para probar el analisis sin depender de que la ingestion este lista, y para la demo
    cuando se quiere mostrar solo la parte funcional.
    """
    fallos = 0
    for match_id, lote in lotes.items():
        estado, cuerpo = enviar(f"{url_base.rstrip('/')}/analyze", lote, "el modulo de analisis")
        marca = "ok " if estado == 200 else "FALLO"
        print(f"  [{marca}] {match_id:28} HTTP {estado}")
        if estado != 200:
            fallos += 1
            print(f"           {cuerpo[:200]}")
    return fallos


def mandar_eventos_a_go(url_base: str, eventos: list[dict], retardo_ms: int) -> int:
    """Manda los eventos uno a uno a `POST /events` del modulo Go, con retardo entre cada uno.

    El retardo es para la demo en vivo: con los matchId intercalados se ve que Go atiende varias
    partidas a la vez.
    """
    url = f"{url_base.rstrip('/')}/events"
    fallos = 0
    for numero, uno in enumerate(eventos, start=1):
        estado, cuerpo = enviar(url, uno, "el modulo de ingestion")
        if estado not in (200, 201, 202):
            fallos += 1
            if fallos <= 3:
                print(f"  [FALLO] {uno['eventId']} {uno['type']:18} HTTP {estado} {cuerpo[:120]}")
            if estado == 0:
                print("  se corta el envio: el servicio no responde")
                break
        elif numero % 20 == 0:
            print(f"  enviados {numero}/{len(eventos)}")
        if retardo_ms:
            time.sleep(retardo_ms / 1000)
    return fallos


# --------------------------------------------------------------------------- entrada

def main(argv: list[str] | None = None) -> int:
    analizador = argparse.ArgumentParser(
        description="Simulador de eventos de GameStats.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    analizador.add_argument("--seed", type=int, default=7,
                            help="semilla del generador; la misma semilla da el mismo flujo (por defecto 7)")
    analizador.add_argument("--matches", type=int, default=6,
                            help="partidas al azar, ademas de las 3 guionadas (por defecto 6)")
    analizador.add_argument("--disorder", type=int, default=8,
                            help="pares de eventos adyacentes a intercambiar (por defecto 8)")
    analizador.add_argument("--out", type=Path, default=Path(__file__).parent / "out",
                            help="carpeta donde escribir los JSON")
    analizador.add_argument("--no-files", action="store_true",
                            help="no escribir archivos, solo enviar")
    # Los valores por defecto salen del entorno para que el servicio de docker compose funcione
    # sin argumentos: alli INGEST_URL ya viene definido en el compose.
    analizador.add_argument("--analyze", metavar="URL", default=os.environ.get("ANALYTICS_URL"),
                            help="manda los lotes a POST {URL}/analyze del modulo Scala "
                                 "(por defecto, la variable ANALYTICS_URL)")
    analizador.add_argument("--post", metavar="URL", default=os.environ.get("INGEST_URL"),
                            help="manda los eventos a POST {URL}/events del modulo Go "
                                 "(por defecto, la variable INGEST_URL)")
    analizador.add_argument("--delay-ms", type=int, default=50,
                            help="retardo entre eventos al usar --post (por defecto 50)")
    args = analizador.parse_args(argv)

    datos = generar(args.seed, args.matches, args.disorder)
    invalidos = casos_invalidos(datetime(2026, 10, 5, 16, 0, 0, tzinfo=timezone.utc))

    resumen = datos["summary"]
    print(f"semilla {resumen['seed']}: {resumen['matches']} partidas "
          f"({resumen['byGame']['racing']} de carreras, {resumen['byGame']['combat']} de combate), "
          f"{resumen['events']} eventos, {resumen['swappedPairs']} pares desordenados")

    if not args.no_files:
        escribir(args.out, datos, invalidos)

    fallos = 0
    if args.analyze:
        print(f"\nenviando {len(datos['batches'])} lotes a {args.analyze}/analyze")
        fallos += mandar_lotes_a_scala(args.analyze, datos["batches"])
    if args.post:
        print(f"\nenviando {len(datos['events'])} eventos a {args.post}/events")
        fallos += mandar_eventos_a_go(args.post, datos["events"], args.delay_ms)

    if fallos:
        print(f"\n{fallos} envios fallaron", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
