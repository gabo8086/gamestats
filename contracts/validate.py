#!/usr/bin/env python3
"""Comprueba que los ejemplos de contracts/examples/ concuerdan con los esquemas.

Es el criterio de salida de la Fase 0 ("los ejemplos validan contra los esquemas") y corre en CI.
Verifica tres cosas:

  1. Los lotes *-match-ok.json cumplen batch.schema.json.
  2. Las reglas que JSON Schema no puede expresar: eventId unico, eventos ordenados por timestamp,
     matchId/gameId/gameVersion coherentes entre el lote y sus eventos.
  3. invalid-events.json: cada caso falla o pasa el esquema segun su campo validAgainstSchema.

Uso: python contracts/validate.py        (desde la raiz del repo)
Requiere: pip install jsonschema
"""

import json
import sys
from pathlib import Path

from jsonschema import Draft202012Validator
from referencing import Registry, Resource

HERE = Path(__file__).resolve().parent
EXAMPLES = HERE / "examples"

errors: list[str] = []


def fail(msg: str) -> None:
    errors.append(msg)


def load(path: Path):
    return json.loads(path.read_text(encoding="utf-8"))


def build_validators():
    """Registra los esquemas por su $id para que el $ref de batch -> event resuelva local."""
    event = load(HERE / "event.schema.json")
    batch = load(HERE / "batch.schema.json")
    registry = Registry().with_resources(
        [(s["$id"], Resource.from_contents(s)) for s in (event, batch)]
    )
    return (
        Draft202012Validator(event, registry=registry),
        Draft202012Validator(batch, registry=registry),
    )


def schema_errors(validator, instance) -> list[str]:
    return [
        f"{'/'.join(str(p) for p in e.absolute_path) or '<raiz>'}: {e.message}"
        for e in validator.iter_errors(instance)
    ]


def check_batch(path: Path, batch_validator, event_validator) -> None:
    batch = load(path)
    name = path.name

    for msg in schema_errors(batch_validator, batch):
        fail(f"{name}: {msg}")

    events = batch.get("events", [])

    seen: set[str] = set()
    for ev in events:
        eid = ev.get("eventId")
        if eid in seen:
            fail(f"{name}: eventId repetido: {eid}")
        seen.add(eid)

    stamps = [ev.get("timestamp", "") for ev in events]
    if stamps != sorted(stamps):
        fail(f"{name}: los eventos no estan ordenados por timestamp ascendente")

    for field in ("matchId", "gameId", "gameVersion"):
        distintos = {ev.get(field) for ev in events}
        if distintos != {batch.get(field)}:
            fail(f"{name}: {field} del lote ({batch.get(field)!r}) no coincide con los eventos ({distintos!r})")

    tipos = [ev.get("type") for ev in events]
    if not tipos or tipos[0] != "MATCH_STARTED":
        fail(f"{name}: el primer evento deberia ser MATCH_STARTED, es {tipos[0] if tipos else 'ninguno'!r}")
    if not tipos or tipos[-1] != "MATCH_FINISHED":
        fail(f"{name}: el ultimo evento deberia ser MATCH_FINISHED, es {tipos[-1] if tipos else 'ninguno'!r}")

    # Redundante con el esquema, pero deja claro el porque en el mensaje de error.
    for ev in events:
        if not str(ev.get("type", "")).startswith("MATCH_") and not ev.get("playerId"):
            fail(f"{name}: {ev.get('eventId')} ({ev.get('type')}) necesita playerId")

    print(f"  {name}: {len(events)} eventos, {len({ev.get('playerId') for ev in events} - {None})} jugadores")


def check_invalid(path: Path, event_validator) -> None:
    casos = load(path)["cases"]
    razones = set()

    for i, caso in enumerate(casos):
        razon = caso["expectedReason"]
        razones.add(razon)
        etiqueta = f"invalid-events.json[{i}] ({razon})"

        for j, prev in enumerate(caso.get("precedingEvents", [])):
            for msg in schema_errors(event_validator, prev):
                fail(f"{etiqueta}: precedingEvents[{j}] deberia ser valido pero {msg}")

        errs = schema_errors(event_validator, caso["event"])
        if caso["validAgainstSchema"] and errs:
            fail(f"{etiqueta}: se declaro valido contra el esquema pero falla: {errs[0]}")
        if not caso["validAgainstSchema"] and not errs:
            fail(f"{etiqueta}: se declaro invalido contra el esquema pero lo cumple")

    esperadas = _razones_documentadas()
    faltan = esperadas - razones - {"malformed_json"}
    if faltan:
        fail(f"invalid-events.json: faltan casos para {sorted(faltan)}")
    sobran = razones - esperadas
    if sobran:
        fail(f"invalid-events.json: razones que no estan en rejections.md: {sorted(sobran)}")

    print(f"  invalid-events.json: {len(casos)} casos, {len(razones)} razones")


def _razones_documentadas() -> set[str]:
    """Saca los codigos de razon de la tabla de rejections.md para que los dos archivos no se separen."""
    texto = (HERE / "rejections.md").read_text(encoding="utf-8")
    codigos = set()
    for linea in texto.splitlines():
        if linea.startswith("| `"):
            codigos.add(linea.split("`")[1])
    return codigos


def main() -> int:
    event_validator, batch_validator = build_validators()

    print("Validando contratos...")
    for nombre in ("racing-match-ok.json", "combat-match-ok.json"):
        check_batch(EXAMPLES / nombre, batch_validator, event_validator)
    check_invalid(EXAMPLES / "invalid-events.json", event_validator)

    if errors:
        print(f"\n{len(errors)} problema(s):", file=sys.stderr)
        for e in errors:
            print(f"  - {e}", file=sys.stderr)
        return 1

    print("\nOK: los ejemplos concuerdan con los esquemas.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
