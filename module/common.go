package module

import (
	"errors"
	"sync"

	"github.com/onflow/flow-go/module/irrecoverable"
)

// WARNING: The semantics of this interface will be changing in the near future, with
// startup / shutdown capabilities being delegated to the Startable interface instead.
// For more details, see https://github.com/onflow/flow-go/pull/1167
//
// ReadyDoneAware provides an easy interface to wait for module startup and shutdown.
// Modules that implement this interface only support a single start-stop cycle, and
// will not restart if Ready() is called again after shutdown has already commenced.
type ReadyDoneAware interface {
	// Ready commences startup of the module, and returns a ready channel that is closed once
	// startup has completed. Note that the ready channel may never close if errors are
	// encountered during startup.
	// If shutdown has already commenced before this method is called for the first time,
	// startup will not be performed and the returned channel will also never close.
	// This should be an idempotent method.
	Ready() <-chan struct{}

	// Done commences shutdown of the module, and returns a done channel that is closed once
	// shutdown has completed. Note that the done channel should be closed even if errors are
	// encountered during shutdown.
	// This should be an idempotent method.
	Done() <-chan struct{}
}

// NoopReadyDoneAware is a ReadyDoneAware implementation whose ready/done channels close
// immediately
type NoopReadyDoneAware struct{}

func (n *NoopReadyDoneAware) Ready() <-chan struct{} {
	ready := make(chan struct{})
	defer close(ready)
	return ready
}

func (n *NoopReadyDoneAware) Done() <-chan struct{} {
	done := make(chan struct{})
	defer close(done)
	return done
}

// ProxiedReadyDoneAware is a ReadyDoneAware implementation that proxies the ReadyDoneAware interface
// from another implementation. This allows for usecases where the Ready/Done methods are needed before
// the proxied object is initialized.
type ProxiedReadyDoneAware struct {
	ready chan struct{}
	done  chan struct{}

	initOnce sync.Once
}

// NewProxiedReadyDoneAware returns a new ProxiedReadyDoneAware instance
func NewProxiedReadyDoneAware() *ProxiedReadyDoneAware {
	return &ProxiedReadyDoneAware{
		ready: make(chan struct{}),
		done:  make(chan struct{}),
	}
}

// Init adds the proxied ReadyDoneAware implementation and sets up the ready/done channels
// to close when the respective channel on the proxied object closes.
// Init can only be called once.
//
// IMPORTANT: the proxied ReadyDoneAware implementation must be idempotent since the Ready and Done
// methods will be called immediately when calling Init.
func (n *ProxiedReadyDoneAware) Init(rda ReadyDoneAware) {
	n.initOnce.Do(func() {
		go func() {
			<-rda.Ready()
			close(n.ready)
		}()
		go func() {
			<-rda.Done()
			close(n.done)
		}()
	})
}

func (n *ProxiedReadyDoneAware) Ready() <-chan struct{} {
	return n.ready
}

func (n *ProxiedReadyDoneAware) Done() <-chan struct{} {
	return n.done
}

var ErrMultipleStartup = errors.New("component may only be started once")

// Startable provides an interface to start a component. Once started, the component
// can be stopped by cancelling the given context.
type Startable interface {
	// Start starts the component. Any irrecoverable errors encountered while the component is running
	// should be thrown with the given SignalerContext.
	// This method should only be called once, and subsequent calls should panic with ErrMultipleStartup.
	Start(irrecoverable.SignalerContext)
}
