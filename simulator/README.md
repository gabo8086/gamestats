# Simulador de eventos

> **Responsable: Samuel.** Esta carpeta está vacía a propósito al terminar la Fase 0.

Hace de sistema de juegos externo: GameStats no implementa los videojuegos, solo consume lo que esta
fuente produce (enunciado §8). El contrato de lo que genera está en
[`../contracts/event.schema.json`](../contracts/event.schema.json).

## Qué tiene que generar

Lo que pide el enunciado §8, más lo que necesitamos para la demo:

- Varias partidas y varios jugadores.
- Partidas de los dos juegos (`racing` y `combat`) **en paralelo**, para que se vea la concurrencia
  de Go: eventos de distintos `matchId` intercalados en el tiempo.
- Casos normales y casos armados a mano que disparen cada regla: al menos una remontada en carreras
  y una racha en combate. Que una regla se dispare por casualidad no sirve para la demo.
- Algunos eventos inválidos, uno por cada razón de
  [`../contracts/rejections.md`](../contracts/rejections.md), para mostrar que la validación los
  rechaza con el código correcto en vez de descartarlos en silencio.
- Eventos fuera de orden, porque Go tiene que ordenarlos por `timestamp` antes de enviarlos.

## Coherencia de los datos de carreras

Los datos de `racing` no pueden ser aleatorios sueltos: `position` tiene que salir de los tiempos
acumulados. Si p1 va con menos tiempo acumulado que p2 en la vuelta 2, entonces p1 tiene mejor
posición que p2 en esa vuelta. Si no, las estadísticas salen contradictorias y la remontada no se
puede justificar en la defensa.

[`../contracts/examples/racing-match-ok.json`](../contracts/examples/racing-match-ok.json) ya está
armado así y sirve de referencia: tres jugadores, tres vueltas, una penalización que cambia el orden
del ranking respecto al orden de llegada, y una remontada de `p3`.

## Salida

Las dos formas, porque sirven para cosas distintas:

1. **Archivos JSON** — para pruebas repetibles y para el CI (datos fijos, resultado fijo).
2. **`POST /events` contra el módulo Go** — para la demo en vivo, enviando con retardos para que se
   vea el procesamiento concurrente.

## Lenguaje

Sin decidir. El simulador no es parte de la nota de paradigmas (no es ni el módulo imperativo ni el
funcional), así que puede ser lo que resulte más rápido: Scala reusando las case classes del
análisis, Go, o un script. Queda a criterio de Samuel.
