package socksflow

import (
	"errors"
	"sync"
)

// ErrOverflow means a peer exceeded the receive window (protocol violation).
var ErrOverflow = errors.New("socksflow: receive buffer overflow")

// ByteQueue is a bounded chunk queue for ordered SOCKS payload delivery.
type ByteQueue struct {
	mu     sync.Mutex
	cond   *sync.Cond
	chunks [][]byte
	total  int
	max    int
	closed bool
}

// NewByteQueue creates a queue that rejects pushes that would exceed max bytes.
func NewByteQueue(max int) *ByteQueue {
	q := &ByteQueue{max: max}
	q.cond = sync.NewCond(&q.mu)
	return q
}

// Push appends a chunk. Returns ErrOverflow if it would exceed max, ErrClosed if closed.
func (q *ByteQueue) Push(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return ErrClosed
	}
	if q.total+len(b) > q.max {
		return ErrOverflow
	}
	q.chunks = append(q.chunks, append([]byte(nil), b...))
	q.total += len(b)
	q.cond.Signal()
	return nil
}

// Pop blocks until a chunk is available or the queue is closed and empty.
func (q *ByteQueue) Pop() ([]byte, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.chunks) == 0 && !q.closed {
		q.cond.Wait()
	}
	if len(q.chunks) == 0 {
		return nil, ErrClosed
	}
	b := q.chunks[0]
	q.chunks = q.chunks[1:]
	q.total -= len(b)
	return b, nil
}

// Close unblocks Pop waiters.
func (q *ByteQueue) Close() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	q.cond.Broadcast()
}
