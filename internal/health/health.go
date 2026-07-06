package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/ifeanyiecheruo/morsel/internal/version"
)

// statusUpdate reports the health of a named component. Any function holding
// a context can call health.From(ctx).Report(upd) without being explicitly
// wired to the Receiver.
type statusUpdate struct {
	Component string // unique component name within the service
	Critical  bool   // if true, an unhealthy status makes the service not ready
	Healthy   bool
	Reason    string    // human-readable explanation when Healthy is false
	UpdatedAt time.Time // timestamp of the last update
}

type Component struct {
	ch       chan<- statusUpdate // write-only; owned by the underlying channel
	name     string
	critical bool
}

func (c *Component) Report(healthy bool, reason string) {
	if c.ch == nil {
		return
	}
	select {
	case c.ch <- statusUpdate{Component: c.name, Critical: c.critical, Healthy: healthy, Reason: reason}:
	default:
	}
}

// Reporter dispatches StatusUpdates to its paired Receiver. Attach one to a
// context with health.With(ctx, reporter) so any downstream code can call
// health.From(ctx).Report(upd) without tight coupling to the Receiver.
type Reporter struct {
	ch chan<- statusUpdate // write-only; owned by the underlying channel
}

// Report enqueues a status update. Never blocks; drops silently when the
// channel is full or when called on the no-op reporter.
func (r *Reporter) NewComponent(name string, critical bool) *Component {
	c := &Component{
		ch:       r.ch,
		name:     name,
		critical: critical,
	}

	c.Report(false, "initializing")

	return c
}

type contextKey struct{}

var noopReporter = &Reporter{}

// With returns a new context carrying reporter.
func With(ctx context.Context, reporter *Reporter) context.Context {
	return context.WithValue(ctx, contextKey{}, reporter)
}

// From returns the Reporter stored in ctx, or a no-op reporter that silently
// discards all updates when none has been injected.
func From(ctx context.Context) (*Reporter, error) {
	if r, ok := ctx.Value(contextKey{}).(*Reporter); ok && r != nil {
		return r, nil
	}
	return noopReporter, errors.New("no health reporter found in context")
}

// Receiver listens on the reporter's channel and accumulates component
// statuses. Pass it to ReadyzHandler and HealthzHandler. Start it with
// go receiver.Run(ctx) before the HTTP server begins accepting connections.
type Receiver struct {
	ch    <-chan statusUpdate // read-only view of the reporter's channel
	mu    sync.RWMutex
	state map[string]statusUpdate
}

// NewReporter creates a Reporter and a Receiver wired to the same buffered channel.
// Inject the Reporter into context with health.With(ctx, reporter) and start
// the Receiver with go receiver.Run(ctx).
func NewReporter() (*Reporter, *Receiver) {
	ch := make(chan statusUpdate, 32)
	return &Reporter{ch: ch}, &Receiver{ch: ch, state: make(map[string]statusUpdate)}
}

// Run drains status updates from the reporter's channel until ctx is
// cancelled. Call as a goroutine before starting the HTTP server.
func (r *Receiver) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case upd := <-r.ch:
			upd.UpdatedAt = time.Now().UTC()
			r.mu.Lock()
			r.state[upd.Component] = upd
			r.mu.Unlock()
		}
	}
}

// ReadyAndSnapshot atomically returns whether all critical components are
// healthy and a sorted snapshot of all registered components.
// Returns ready=false when no components have reported yet.
func (r *Receiver) Read() (ready bool, components []statusUpdate) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.state) == 0 {
		return false, nil
	}
	ready = true
	components = make([]statusUpdate, 0, len(r.state))
	for _, upd := range r.state {
		if upd.Critical && !upd.Healthy {
			ready = false
		}
		components = append(components, upd)
	}
	sort.Slice(components, func(i, j int) bool {
		return components[i].Component < components[j].Component
	})
	return ready, components
}

// DrainForTest synchronously processes all buffered updates without starting
// the Run goroutine. Call in tests after sending status updates to avoid
// timing dependencies on goroutine scheduling.
func (r *Receiver) DrainForTest() {
	for {
		select {
		case upd := <-r.ch:
			r.mu.Lock()
			r.state[upd.Component] = upd
			r.mu.Unlock()
		default:
			return
		}
	}
}

// LivezHandler always returns 200 — the process is alive. Wire to GET /livez.
func (r *Receiver) LivezHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

// ReadyzHandler returns 200 when all critical components are healthy, 503
// otherwise. Wire to GET /readyz.
func (r *Receiver) ReadyzHandler(w http.ResponseWriter, _ *http.Request) {
	ready, _ := r.Read()
	if !ready {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

type healthzBody struct {
	Status     string      `json:"status"`
	Version    string      `json:"version"`
	Components []compEntry `json:"components"`
}

type compEntry struct {
	Name      string    `json:"name"`
	Critical  bool      `json:"critical"`
	Healthy   bool      `json:"healthy"`
	Reason    string    `json:"reason,omitempty"`
	UpdatedAt time.Time `json:"updated"`
}

// HealthzHandler writes JSON describing all component statuses. HTTP status is
// 200 when all critical components are healthy, 503 otherwise. Wire to
// GET /healthz for services that do not use an OAS-generated handler.
func (r *Receiver) HealthzHandler(w http.ResponseWriter, _ *http.Request) {
	ready, snaps := r.Read()
	status := "ok"
	if !ready {
		status = "degraded"
	}
	comps := make([]compEntry, len(snaps))
	for i, s := range snaps {
		comps[i] = compEntry{Name: s.Component, Critical: s.Critical, Healthy: s.Healthy, Reason: s.Reason, UpdatedAt: s.UpdatedAt}
	}
	w.Header().Set("Content-Type", "application/json")
	if !ready {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	_ = json.NewEncoder(w).Encode(healthzBody{
		Status:     status,
		Version:    version.Get().String(),
		Components: comps,
	})
}
