#!/usr/bin/env python3
"""Pruebas del simulador.

Lo que se comprueba no es "el programa corre" sino las dos propiedades de las que depende la demo:

1. **Coherencia.** En carreras la `position` tiene que salir de los tiempos acumulados. Si no, las
   estadisticas salen contradictorias y la remontada no se puede justificar en la defensa.
2. **Que las reglas se disparen de verdad.** Las partidas guionadas existen porque que una regla
   ocurra por casualidad no sirve para mostrarla. Aqui se verifica que las condiciones de cada regla
   se cumplen en los datos generados, sin depender de que el modulo Scala este levantado.

Correr con:  python -m unittest   (desde esta carpeta)
"""

import json
import re
import unittest
from datetime import datetime, timezone
from pathlib import Path

import simulate

RAIZ = Path(__file__).resolve().parent.parent


def instante(texto: str) -> datetime:
    return datetime.fromisoformat(texto.replace("Z", "+00:00"))


class GeneracionTest(unittest.TestCase):

    @classmethod
    def setUpClass(cls) -> None:
        cls.datos = simulate.generar(semilla=7, partidas_al_azar=6, desorden=8)
        cls.lotes = cls.datos["batches"]

    def test_la_misma_semilla_da_el_mismo_flujo(self):
        """Sin esto no se puede repetir una demo ni fijar un resultado esperado en el CI."""
        otra = simulate.generar(semilla=7, partidas_al_azar=6, desorden=8)
        self.assertEqual(self.datos["events"], otra["events"])

    def test_semillas_distintas_dan_flujos_distintos(self):
        otra = simulate.generar(semilla=99, partidas_al_azar=6, desorden=8)
        self.assertNotEqual(self.datos["events"], otra["events"])

    def test_los_eventId_no_se_repiten(self):
        ids = [e["eventId"] for e in self.datos["events"]]
        self.assertEqual(len(ids), len(set(ids)))

    def test_hay_partidas_de_los_dos_juegos(self):
        juegos = {lote["gameId"] for lote in self.lotes.values()}
        self.assertEqual(juegos, {"racing", "combat"})

    def test_las_partidas_se_solapan_en_el_tiempo(self):
        """El enunciado §8 pide partidas de juegos distintos en paralelo: es lo que le da trabajo
        concurrente a Go. Se comprueba mirando si algun evento cae dentro de la ventana de otra
        partida distinta."""
        ventanas = {
            match_id: (instante(lote["events"][0]["timestamp"]),
                       instante(lote["events"][-1]["timestamp"]))
            for match_id, lote in self.lotes.items()
        }
        solapan = [
            (a, b)
            for a, (ia, fa) in ventanas.items()
            for b, (ib, fb) in ventanas.items()
            if a < b and ia < fb and ib < fa
        ]
        self.assertTrue(solapan, "ninguna partida se solapa con otra")

    def test_el_flujo_llega_desordenado(self):
        """Go tiene que ordenar por timestamp; si el simulador entrega todo ordenado, esa parte
        nunca se ejercita."""
        marcas = [e["timestamp"] for e in self.datos["events"]]
        self.assertNotEqual(marcas, sorted(marcas))

    def test_el_desorden_no_mueve_los_eventos_de_partida(self):
        """Si un MATCH_STARTED quedara detras de otro evento de su partida, Go rechazaria con
        `match_not_started` y seria un rechazo inventado por el simulador, no un caso real."""
        for match_id, lote in self.lotes.items():
            del_flujo = [e for e in self.datos["events"] if e["matchId"] == match_id]
            self.assertEqual(del_flujo[0]["type"], "MATCH_STARTED", match_id)
            self.assertEqual(del_flujo[-1]["type"], "MATCH_FINISHED", match_id)

    def test_los_lotes_van_ordenados(self):
        """El contrato dice que el lote llega ordenado por timestamp y que Scala no reordena."""
        for match_id, lote in self.lotes.items():
            marcas = [e["timestamp"] for e in lote["events"]]
            self.assertEqual(marcas, sorted(marcas), match_id)


class CoherenciaDeCarrerasTest(unittest.TestCase):

    @classmethod
    def setUpClass(cls) -> None:
        cls.lotes = simulate.generar(semilla=7, partidas_al_azar=6, desorden=8)["batches"]

    def test_la_posicion_sale_del_tiempo_acumulado(self):
        for match_id, lote in self.lotes.items():
            if lote["gameId"] != "racing":
                continue
            acumulado: dict[str, int] = {}
            por_vuelta: dict[int, dict[str, int]] = {}
            posiciones: dict[int, dict[str, int]] = {}

            for e in lote["events"]:
                if e["type"] != "LAP_COMPLETED":
                    continue
                jugador, vuelta = e["playerId"], e["data"]["lap"]
                acumulado[jugador] = acumulado.get(jugador, 0) + e["data"]["lapTimeMs"]
                por_vuelta.setdefault(vuelta, {})[jugador] = acumulado[jugador]
                posiciones.setdefault(vuelta, {})[jugador] = e["data"]["position"]

            for vuelta, tiempos in por_vuelta.items():
                esperado = {
                    jugador: puesto
                    for puesto, jugador in enumerate(sorted(tiempos, key=tiempos.get), start=1)
                }
                self.assertEqual(posiciones[vuelta], esperado,
                                 f"{match_id} vuelta {vuelta}: la posicion no refleja el acumulado")

    def test_la_remontada_guionada_cumple_su_definicion(self):
        """Remontada = estuvo en la ultima posicion en alguna vuelta y gano por tiempo ajustado."""
        lote = self.lotes["m-sim-racing-comeback"]
        jugadores = [e["playerId"] for e in lote["events"] if e["type"] == "PLAYER_JOINED"]

        ajustado = {j: 0 for j in jugadores}
        posiciones: dict[str, list[int]] = {j: [] for j in jugadores}
        for e in lote["events"]:
            if e["type"] == "LAP_COMPLETED":
                ajustado[e["playerId"]] += e["data"]["lapTimeMs"]
                posiciones[e["playerId"]].append(e["data"]["position"])
            elif e["type"] == "PENALTY":
                ajustado[e["playerId"]] += e["data"]["seconds"] * 1000

        ganador = min(ajustado, key=ajustado.get)
        self.assertEqual(ganador, "p3")
        self.assertIn(len(jugadores), posiciones[ganador],
                      "el ganador nunca estuvo en la ultima posicion")

    def test_la_penalizacion_guionada_cambia_al_ganador(self):
        """El caso que demuestra que el ganador combina dos tipos de evento y no uno solo."""
        lote = self.lotes["m-sim-racing-penalty"]
        solo_vueltas: dict[str, int] = {}
        con_penalizacion: dict[str, int] = {}
        for e in lote["events"]:
            if e["type"] == "LAP_COMPLETED":
                solo_vueltas[e["playerId"]] = solo_vueltas.get(e["playerId"], 0) + e["data"]["lapTimeMs"]
            elif e["type"] == "PENALTY":
                con_penalizacion[e["playerId"]] = e["data"]["seconds"] * 1000

        ajustado = {j: t + con_penalizacion.get(j, 0) for j, t in solo_vueltas.items()}
        self.assertEqual(min(solo_vueltas, key=solo_vueltas.get), "p1", "p1 deberia ser el mas rapido")
        self.assertEqual(min(ajustado, key=ajustado.get), "p2", "la penalizacion deberia dar vuelta el resultado")


class ReglasDeCombateTest(unittest.TestCase):

    @classmethod
    def setUpClass(cls) -> None:
        cls.lote = simulate.generar(semilla=7, partidas_al_azar=6, desorden=8)["batches"]["m-sim-combat-streak"]

    def bajas(self):
        return [e for e in self.lote["events"] if e["type"] == "PLAYER_ELIMINATED"]

    def test_la_racha_guionada_cumple_su_definicion(self):
        """Racha = 3 eliminaciones del mismo jugador en <= 10 s sin aparecer como victima en medio."""
        bajas = self.bajas()
        suyas = [e for e in bajas if e["playerId"] == "p3"]
        self.assertGreaterEqual(len(suyas), 3)

        tramo = suyas[:3]
        ventana = (instante(tramo[-1]["timestamp"]) - instante(tramo[0]["timestamp"])).total_seconds()
        self.assertLessEqual(ventana, 10, "la ventana se paso de 10 s")

        muertes = [instante(e["timestamp"]) for e in bajas if e["data"]["victimId"] == "p3"]
        entre = [m for m in muertes
                 if instante(tramo[0]["timestamp"]) <= m <= instante(tramo[-1]["timestamp"])]
        self.assertEqual(entre, [], "p3 muere dentro de su propia racha")

    def test_ningun_otro_jugador_alcanza_una_racha(self):
        """Regresion del bug que encontro este simulador: los jugadores con una o dos
        eliminaciones no deben contar como racha."""
        bajas = self.bajas()
        for jugador in {e["playerId"] for e in bajas} - {"p3"}:
            suyas = [e for e in bajas if e["playerId"] == jugador]
            if len(suyas) < 3:
                continue
            ventana = (instante(suyas[2]["timestamp"]) - instante(suyas[0]["timestamp"])).total_seconds()
            self.assertGreater(ventana, 10, f"{jugador} tambien arma una racha sin querer")

    def test_la_venganza_guionada_existe(self):
        """Venganza = A elimina a B y despues B elimina a A."""
        bajas = self.bajas()
        pares = [(e["playerId"], e["data"]["victimId"], instante(e["timestamp"])) for e in bajas]
        venganzas = [
            (autor, victima)
            for autor, victima, momento in pares
            for a2, v2, m2 in pares
            if a2 == victima and v2 == autor and m2 > momento
        ]
        self.assertTrue(venganzas, "no hay ninguna venganza en la partida guionada")


class CasosInvalidosTest(unittest.TestCase):

    @classmethod
    def setUpClass(cls) -> None:
        cls.casos = simulate.casos_invalidos(datetime(2026, 10, 5, 16, 0, tzinfo=timezone.utc))["cases"]

    def test_hay_un_caso_por_cada_razon_del_catalogo(self):
        """Si alguien agrega una razon a contracts/rejections.md, esta prueba lo recuerda."""
        catalogo = Path(RAIZ / "contracts/rejections.md").read_text(encoding="utf-8")
        razones = set(re.findall(r"^\| `([a-z_]+)` \|", catalogo, re.M))
        self.assertTrue(razones, "no se pudieron leer las razones de rejections.md")
        self.assertEqual(razones - {c["expectedReason"] for c in self.casos}, set())

    def test_cada_caso_trae_un_evento_o_un_cuerpo_crudo(self):
        for caso in self.casos:
            self.assertTrue("event" in caso or "rawBody" in caso, caso["expectedReason"])

    def test_malformed_json_no_es_json(self):
        crudo = next(c for c in self.casos if c["expectedReason"] == "malformed_json")["rawBody"]
        with self.assertRaises(json.JSONDecodeError):
            json.loads(crudo)


class ContratoTest(unittest.TestCase):
    """Valida lo generado contra los esquemas reales. Se salta si falta `jsonschema`, que es
    dependencia del validador del contrato y no del simulador."""

    @classmethod
    def setUpClass(cls) -> None:
        try:
            from jsonschema import Draft202012Validator
        except ImportError:  # pragma: no cover
            raise unittest.SkipTest("jsonschema no esta instalado (pip install jsonschema)")
        esquema = json.loads((RAIZ / "contracts/event.schema.json").read_text(encoding="utf-8"))
        cls.validador = Draft202012Validator(esquema)
        cls.datos = simulate.generar(semilla=7, partidas_al_azar=6, desorden=8)

    def test_todos_los_eventos_cumplen_el_esquema(self):
        for e in self.datos["events"]:
            errores = [x.message for x in self.validador.iter_errors(e)]
            self.assertEqual(errores, [], f"{e['eventId']} {e['type']}")

    def test_los_casos_invalidos_son_invalidos_como_dicen(self):
        casos = simulate.casos_invalidos(datetime(2026, 10, 5, 16, 0, tzinfo=timezone.utc))["cases"]
        for caso in casos:
            if "event" not in caso:
                continue
            self.assertEqual(self.validador.is_valid(caso["event"]), caso["validAgainstSchema"],
                             caso["expectedReason"])


if __name__ == "__main__":
    unittest.main()
