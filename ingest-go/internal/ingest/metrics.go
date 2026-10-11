package ingest

import (
	"maps"
	"sync"

	"gamestats/ingest/internal/contract"
)

// Metrics cuenta lo que pasa en la ingestion. No sabe nada de los juegos: son eventos que entraron,
// cuantos se rechazaron y por que, y que paso con los lotes enviados a Scala.
type Metrics struct {
	mu            sync.Mutex
	received      int
	valid         int
	byReason      map[string]int
	batchesSent   int
	batchesFailed int
}

// Snapshot es el cuerpo de GET /metrics.
type Snapshot struct {
	EventsReceived     int            `json:"eventsReceived"`
	EventsValid        int            `json:"eventsValid"`
	EventsInvalid      int            `json:"eventsInvalid"`
	RejectionsByReason map[string]int `json:"rejectionsByReason"`
	ActiveMatches      int            `json:"activeMatches"`
	BatchesSent        int            `json:"batchesSent"`
	BatchesFailed      int            `json:"batchesFailed"`
}

func newMetrics() *Metrics {
	// Todas las razones arrancan en 0 para que GET /metrics muestre el catalogo completo.
	porRazon := make(map[string]int, len(contract.AllReasons))
	for _, r := range contract.AllReasons {
		porRazon[r] = 0
	}
	return &Metrics{byReason: porRazon}
}

func (m *Metrics) record(rej *contract.Rejection) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.received++
	if rej == nil {
		m.valid++
		return
	}
	m.byReason[rej.Reason]++
}

func (m *Metrics) batchSent() {
	m.mu.Lock()
	m.batchesSent++
	m.mu.Unlock()
}

func (m *Metrics) batchFailed() {
	m.mu.Lock()
	m.batchesFailed++
	m.mu.Unlock()
}

func (m *Metrics) snapshot(activeMatches int) Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return Snapshot{
		EventsReceived:     m.received,
		EventsValid:        m.valid,
		EventsInvalid:      m.received - m.valid,
		RejectionsByReason: maps.Clone(m.byReason),
		ActiveMatches:      activeMatches,
		BatchesSent:        m.batchesSent,
		BatchesFailed:      m.batchesFailed,
	}
}
