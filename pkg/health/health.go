package health

import (
	"sync/atomic"
	"net/http"

	log "github.com/thkukuk/mqtt-exporter/pkg/logger"
)

type HealthState struct {
	isReady *atomic.Value
	debug *atomic.Value
}

// NewHealthState returns a new instance of a HealthState object
// which can be marked as "ready" or "not ready" for the readiness probe.
// The state is initially "not ready".
func NewHealthState() *HealthState {
	var val HealthState

	val.isReady = &atomic.Value{}
	val.isReady.Store(false)

	val.debug = &atomic.Value{}
	val.debug.Store(false)

	return &val
}

// IsReady marks the object as healthy for the readiness probe
func (hs *HealthState) IsReady() {
	hs.isReady.Store(true)
}

// NotReady marks the probe as unhealthy for the readiness probe
func (hs *HealthState) NotReady() {
	hs.isReady.Store(false)
}

func (hs *HealthState) DebugMode(enable bool) {
	hs.debug.Store(enable)
}

// ServeHTTP implements http.Handler interface
func (hs *HealthState) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/healthz":
		// healthz is a liveness probe
		if hs.debug != nil && hs.debug.Load().(bool) {
			log.Debug("/healthz liveness probe called")
		}
		w.WriteHeader(http.StatusOK)
	case "/readyz":
		// readyz is a readiness probe
		if hs.isReady == nil || !hs.isReady.Load().(bool) {
			if hs.debug != nil && hs.debug.Load().(bool) {
				log.Debug("/readyz readiness probe called: false")
			}
			http.Error(w, http.StatusText(http.StatusServiceUnavailable),
				http.StatusServiceUnavailable)
		} else {
			if hs.debug != nil && hs.debug.Load().(bool) {
				log.Debug("/readyz readiness probe called: true")
			}
			w.WriteHeader(http.StatusOK)
		}
	default:
		log.Warnf("Unknown URL: %q", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
}
