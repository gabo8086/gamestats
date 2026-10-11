// Package ingest es el nucleo imperativo y concurrente del modulo: recibe eventos ya decodificados,
// los enruta por matchId a una goroutine por partida y, cuando la partida termina, manda el lote a
// Scala.
//
// Reparto del estado:
//
//   - El Dispatcher guarda lo que cruza partidas: que partidas estan vivas, cuales terminaron y que
//     eventId ya se aceptaron. Lo protege un sync.Mutex porque lo tocan todas las peticiones HTTP.
//   - Cada partida guarda lo suyo (eventos, jugadores activos, desconectados) dentro de su propia
//     goroutine. Nadie mas lo toca, asi que no necesita lock: los eventos le llegan por un channel
//     y se procesan de a uno, en el orden en que llegaron.
package ingest

import (
	"log"
	"slices"
	"sync"

	"gamestats/ingest/internal/contract"
)

// Sender entrega el lote de una partida terminada al modulo de analisis. En produccion es
// HTTPSender; en las pruebas, una funcion que guarda el lote.
type Sender func(contract.Batch) error

// Dispatcher enruta cada evento a la goroutine de su partida.
type Dispatcher struct {
	send    Sender
	metrics *Metrics

	mu       sync.Mutex
	matches  map[string]*match // partidas vivas
	finished map[string]bool   // partidas que ya recibieron MATCH_FINISHED
	seen     map[string]bool   // eventId aceptados, para detectar duplicados
	failed   []contract.Batch  // lotes que Scala no acepto: no se pierden, quedan en memoria
}

// match es el extremo del channel de una partida. done se cierra cuando la goroutine deja de
// recibir eventos: quien estuviera esperando para enviarle uno se entera y no queda colgado.
type match struct {
	in   chan request
	done chan struct{}
}

// request es un evento con el channel por el que la partida contesta: "" si lo acepto, o la razon
// de rechazo. La respuesta es sincrona porque POST /events tiene que devolver los rechazos.
type request struct {
	ev    contract.Event
	reply chan string
}

func NewDispatcher(send Sender) *Dispatcher {
	return &Dispatcher{
		send:     send,
		metrics:  newMetrics(),
		matches:  map[string]*match{},
		finished: map[string]bool{},
		seen:     map[string]bool{},
	}
}

// Ingest procesa un evento crudo de principio a fin: lo decodifica, valida el sobre, lo entrega a
// su partida y lleva la cuenta en las metricas. Devuelve nil si el evento fue aceptado.
func (d *Dispatcher) Ingest(raw []byte) *contract.Rejection {
	ev, rej := contract.Parse(raw)
	if rej == nil {
		rej = d.submit(ev)
	}
	d.metrics.record(rej)
	if rej != nil {
		log.Printf("rechazado %s: %s (%s)", rej.EventID, rej.Reason, rej.Detail)
	}
	return rej
}

// submit aplica las reglas que dependen del historial y entrega el evento a su partida.
func (d *Dispatcher) submit(ev contract.Event) *contract.Rejection {
	m, rej := d.route(ev)
	if rej != nil {
		return rej
	}

	reply := make(chan string, 1)
	select {
	case m.in <- request{ev: ev, reply: reply}:
		if razon := <-reply; razon != "" {
			d.forget(ev.EventID)
			return &contract.Rejection{EventID: ev.EventID, Reason: razon, Detail: ev.MatchID}
		}
		return nil
	case <-m.done:
		// La partida termino entre que la buscamos y que le enviamos el evento.
		d.forget(ev.EventID)
		return &contract.Rejection{EventID: ev.EventID, Reason: contract.ReasonMatchAlreadyFinished, Detail: ev.MatchID}
	}
}

// route busca (o crea) la partida del evento y reserva su eventId. Todo bajo el mismo lock, para
// que dos peticiones simultaneas con el mismo eventId no entren las dos.
func (d *Dispatcher) route(ev contract.Event) (*match, *contract.Rejection) {
	d.mu.Lock()
	defer d.mu.Unlock()

	rechazo := func(razon string) (*match, *contract.Rejection) {
		return nil, &contract.Rejection{EventID: ev.EventID, Reason: razon, Detail: ev.MatchID}
	}

	if d.seen[ev.EventID] {
		return rechazo(contract.ReasonDuplicateEvent)
	}
	if d.finished[ev.MatchID] {
		return rechazo(contract.ReasonMatchAlreadyFinished)
	}

	m, vive := d.matches[ev.MatchID]
	if !vive {
		if ev.Type != contract.TypeMatchStarted {
			return rechazo(contract.ReasonMatchNotStarted)
		}
		m = &match{in: make(chan request), done: make(chan struct{})}
		d.matches[ev.MatchID] = m
		go d.run(ev.MatchID, m)
	}

	d.seen[ev.EventID] = true
	return m, nil
}

// forget libera un eventId reservado cuyo evento termino rechazado: "duplicado" significa que ya
// se acepto antes, no que alguna vez se intento.
func (d *Dispatcher) forget(eventID string) {
	d.mu.Lock()
	delete(d.seen, eventID)
	d.mu.Unlock()
}

// run es la goroutine de una partida. Vive desde MATCH_STARTED hasta que entrega el lote.
func (d *Dispatcher) run(matchID string, m *match) {
	estado := newMatchState(matchID)

	for req := range m.in {
		razon := estado.apply(req.ev)
		req.reply <- razon
		if razon == "" && req.ev.Type == contract.TypeMatchFinished {
			break
		}
	}

	// Primero se cierra la partida (ya no recibe eventos) y despues se envia: si Scala tarda, los
	// eventos tardios se rechazan de inmediato en vez de quedar esperando.
	d.mu.Lock()
	d.finished[matchID] = true
	delete(d.matches, matchID)
	close(m.done)
	d.mu.Unlock()

	d.deliver(estado.batch())
}

func (d *Dispatcher) deliver(lote contract.Batch) {
	if err := d.send(lote); err != nil {
		log.Printf("no se pudo enviar %s a analisis: %v", lote.MatchID, err)
		d.metrics.batchFailed()
		d.mu.Lock()
		d.failed = append(d.failed, lote)
		d.mu.Unlock()
		return
	}
	log.Printf("enviada %s a analisis (%d eventos)", lote.MatchID, len(lote.Events))
	d.metrics.batchSent()
}

// Metrics devuelve una foto de las metricas de ingestion para GET /metrics.
func (d *Dispatcher) Metrics() Snapshot {
	d.mu.Lock()
	activas := len(d.matches)
	d.mu.Unlock()
	return d.metrics.snapshot(activas)
}

// matchState es el estado mutable de una partida. Solo lo toca su goroutine.
type matchState struct {
	matchID      string
	gameID       string
	gameVersion  string
	events       []contract.Event
	active       map[string]bool // jugadores unidos y conectados
	disconnected map[string]bool
}

func newMatchState(matchID string) *matchState {
	return &matchState{
		matchID:      matchID,
		active:       map[string]bool{},
		disconnected: map[string]bool{},
	}
}

// apply incorpora un evento al estado. Devuelve "" si lo acepta, o la razon de rechazo.
// Go solo mira el sobre: sabe quien entro y quien se fue, pero no que es una vuelta ni un disparo.
func (s *matchState) apply(ev contract.Event) string {
	var jugador string
	if ev.PlayerID != nil {
		jugador = *ev.PlayerID
	}

	switch ev.Type {
	case contract.TypeMatchStarted:
		s.gameID, s.gameVersion = ev.GameID, ev.GameVersion
	case contract.TypePlayerJoined:
		s.active[jugador] = true
	case contract.TypePlayerDisconnected:
		s.disconnected[jugador] = true
		delete(s.active, jugador)
	case contract.TypePlayerReconnected:
		if !s.disconnected[jugador] {
			return contract.ReasonReconnectWithoutDisconnect
		}
		delete(s.disconnected, jugador)
		s.active[jugador] = true
	}

	s.events = append(s.events, ev)
	return ""
}

// batch arma el lote para Scala. Los eventos pueden haber llegado desordenados; el contrato pide
// mandarlos por timestamp ascendente. El orden es estable: dos eventos con el mismo timestamp
// conservan el orden de llegada.
func (s *matchState) batch() contract.Batch {
	eventos := slices.Clone(s.events)
	slices.SortStableFunc(eventos, func(a, b contract.Event) int {
		return a.Time().Compare(b.Time())
	})
	return contract.Batch{
		MatchID:     s.matchID,
		GameID:      s.gameID,
		GameVersion: s.gameVersion,
		Events:      eventos,
	}
}
