package socksflow

import (
	"sync"
	"testing"
	"time"
)

func TestAcquireBlocksUntilRelease(t *testing.T) {
	w := NewWindow(0)
	var wg sync.WaitGroup
	wg.Add(1)
	started := make(chan struct{})
	go func() {
		defer wg.Done()
		close(started)
		if err := w.Acquire(10); err != nil {
			t.Errorf("Acquire: %v", err)
		}
	}()
	<-started
	time.Sleep(20 * time.Millisecond)
	if w.Available() != 0 {
		t.Fatalf("expected 0 credit before release")
	}
	w.Release(10)
	wg.Wait()
	if w.Available() != 0 {
		t.Fatalf("expected 0 after acquire, got %d", w.Available())
	}
}

func TestAcquireFailsWhenClosed(t *testing.T) {
	w := NewWindow(0)
	w.Close()
	if err := w.Acquire(1); err != ErrClosed {
		t.Fatalf("got %v, want ErrClosed", err)
	}
}

func TestInitialWindowAcquire(t *testing.T) {
	w := NewWindow(InitialWindow)
	if err := w.Acquire(InitialWindow); err != nil {
		t.Fatal(err)
	}
	if w.Available() != 0 {
		t.Fatalf("want 0, got %d", w.Available())
	}
}
