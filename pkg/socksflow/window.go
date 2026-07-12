package socksflow

import (
	"errors"
	"sync"
)

// InitialWindow is the per-direction byte credit granted when a SOCKS stream starts.
const InitialWindow = 256 << 10 // 256 KiB

var (
	// ErrClosed is returned when Acquire is called on a closed window.
	ErrClosed = errors.New("socksflow: window closed")
)

// Window is a per-stream send credit counter.
type Window struct {
	mu     sync.Mutex
	cond   *sync.Cond
	credit uint64
	closed bool
}

// NewWindow creates a send window with the given initial credit.
func NewWindow(initial uint64) *Window {
	w := &Window{credit: initial}
	w.cond = sync.NewCond(&w.mu)
	return w
}

// Acquire blocks until at least n bytes of credit are available, then consumes them.
func (w *Window) Acquire(n uint64) error {
	if n == 0 {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for w.credit < n && !w.closed {
		w.cond.Wait()
	}
	if w.closed {
		return ErrClosed
	}
	w.credit -= n
	return nil
}

// Release adds credit (typically from a peer ACK) and wakes waiters.
func (w *Window) Release(n uint64) {
	if n == 0 {
		return
	}
	w.mu.Lock()
	w.credit += n
	w.mu.Unlock()
	w.cond.Broadcast()
}

// Available returns the current send credit (for tests / diagnostics).
func (w *Window) Available() uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.credit
}

// Close marks the window closed and wakes all waiters.
func (w *Window) Close() {
	w.mu.Lock()
	w.closed = true
	w.mu.Unlock()
	w.cond.Broadcast()
}
