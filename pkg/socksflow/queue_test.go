package socksflow

import "testing"

func TestByteQueuePushPop(t *testing.T) {
	q := NewByteQueue(100)
	if err := q.Push([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	b, err := q.Pop()
	if err != nil || string(b) != "hello" {
		t.Fatalf("got %q %v", b, err)
	}
}

func TestByteQueueOverflow(t *testing.T) {
	q := NewByteQueue(4)
	if err := q.Push([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	if err := q.Push([]byte("de")); err != ErrOverflow {
		t.Fatalf("got %v, want ErrOverflow", err)
	}
}
