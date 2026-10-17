// Simulador de eventos de GameStats.
//
// Hace de sistema de juegos externo: GameStats no implementa los videojuegos, solo consume lo que
// esta fuente produce (enunciado §8). El formato de lo que genera es contracts/event.schema.json.
//
// Genera partidas de los cuatro juegos con ventanas de tiempo solapadas, de modo que los eventos de
// distintos matchId quedan intercalados en el flujo: es lo que le da trabajo concurrente al modulo
// de ingestion. Ademas de las partidas al azar incluye seis **guionadas**, con los tiempos escritos
// a mano, que garantizan que se disparen las cuatro reglas. Que una regla se dispare por casualidad
// no sirve para la demo.
//
// Uso tipico:
//
//	simulador                                    # escribe out/ con los JSON
//	simulador -analyze http://localhost:8081     # manda los lotes directo a Scala
//	simulador -post http://localhost:8080        # manda los eventos a Go, con retardo
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

func main() {
	semilla := flag.Int64("seed", 7, "semilla del generador; la misma semilla da el mismo flujo")
	partidas := flag.Int("matches", 6, "partidas al azar, ademas de las 6 guionadas")
	desorden := flag.Int("disorder", 8, "pares de eventos adyacentes a intercambiar")
	salida := flag.String("out", "out", "carpeta donde escribir los JSON")
	sinArchivos := flag.Bool("no-files", false, "no escribir archivos, solo enviar")
	analyze := flag.String("analyze", os.Getenv("ANALYTICS_URL"), "manda los lotes a POST {URL}/analyze")
	post := flag.String("post", os.Getenv("INGEST_URL"), "manda los eventos a POST {URL}/events")
	retardo := flag.Int("delay-ms", 50, "retardo entre eventos al usar -post")
	flag.Parse()

	flujo := Generar(*semilla, *partidas, *desorden)
	r := flujo.Resumen
	fmt.Printf("semilla %d: %d partidas (%s), %d eventos, %d pares desordenados\n",
		r.Seed, r.Matches, porJuegoLegible(r.ByGame), r.Events, r.SwappedPairs)

	if !*sinArchivos {
		if err := escribir(*salida, flujo); err != nil {
			fmt.Fprintln(os.Stderr, "no se pudo escribir:", err)
			os.Exit(1)
		}
	}

	fallos := 0
	if *analyze != "" {
		fmt.Printf("\nenviando %d lotes a %s/analyze\n", len(flujo.Lotes), *analyze)
		fallos += mandarLotes(*analyze, flujo.Lotes)
	}
	if *post != "" {
		fmt.Printf("\nenviando %d eventos a %s/events\n", len(flujo.Eventos), *post)
		fallos += mandarEventos(*post, flujo.Eventos, time.Duration(*retardo)*time.Millisecond)
	}

	if fallos > 0 {
		fmt.Fprintf(os.Stderr, "\n%d envios fallaron\n", fallos)
		os.Exit(1)
	}
}

func porJuegoLegible(porJuego map[string]int) string {
	juegos := make([]string, 0, len(porJuego))
	for j := range porJuego {
		juegos = append(juegos, j)
	}
	sort.Strings(juegos)

	texto := ""
	for i, j := range juegos {
		if i > 0 {
			texto += ", "
		}
		texto += fmt.Sprintf("%d de %s", porJuego[j], j)
	}
	return texto
}

func escribir(destino string, flujo Flujo) error {
	if err := os.MkdirAll(filepath.Join(destino, "batches"), 0o755); err != nil {
		return err
	}

	volcar := func(ruta string, contenido any) error {
		datos, err := json.MarshalIndent(contenido, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(ruta, append(datos, '\n'), 0o644)
	}

	if err := volcar(filepath.Join(destino, "events.json"), flujo.Eventos); err != nil {
		return err
	}
	if err := volcar(filepath.Join(destino, "invalid-events.json"), flujo.Invalidos); err != nil {
		return err
	}
	if err := volcar(filepath.Join(destino, "summary.json"), flujo.Resumen); err != nil {
		return err
	}
	for matchID, lote := range flujo.Lotes {
		if err := volcar(filepath.Join(destino, "batches", matchID+".json"), lote); err != nil {
			return err
		}
	}

	fmt.Printf("escrito en %s/\n", destino)
	fmt.Printf("  events.json          %d eventos de %d partidas\n", len(flujo.Eventos), flujo.Resumen.Matches)
	fmt.Printf("  invalid-events.json  %d casos\n", len(flujo.Invalidos.Cases))
	fmt.Printf("  batches/             %d lotes listos para POST /analyze\n", len(flujo.Lotes))
	return nil
}
