package engine

import (
	"testing"
)

func TestRingBuffer_Push_And_Len(t *testing.T) {
	rb := NewRingBuffer[int](5)

	if rb.Len() != 0 {
		t.Errorf("expected len 0, got %d", rb.Len())
	}

	rb.Push(1)
	rb.Push(2)
	rb.Push(3)

	if rb.Len() != 3 {
		t.Errorf("expected len 3, got %d", rb.Len())
	}
}

func TestRingBuffer_Overflow(t *testing.T) {
	rb := NewRingBuffer[int](3)

	rb.Push(1)
	rb.Push(2)
	rb.Push(3)
	rb.Push(4) // overwrites 1
	rb.Push(5) // overwrites 2

	if rb.Len() != 3 {
		t.Errorf("expected len 3 after overflow, got %d", rb.Len())
	}

	all := rb.All()
	expected := []int{3, 4, 5}
	for i, v := range all {
		if v != expected[i] {
			t.Errorf("All()[%d] = %d, want %d", i, v, expected[i])
		}
	}
}

func TestRingBuffer_All_OrderOldestFirst(t *testing.T) {
	rb := NewRingBuffer[string](4)

	rb.Push("a")
	rb.Push("b")
	rb.Push("c")

	all := rb.All()
	if len(all) != 3 {
		t.Fatalf("expected 3 items, got %d", len(all))
	}
	if all[0] != "a" || all[1] != "b" || all[2] != "c" {
		t.Errorf("unexpected order: %v", all)
	}
}

func TestRingBuffer_Last(t *testing.T) {
	rb := NewRingBuffer[int](5)

	_, ok := rb.Last()
	if ok {
		t.Error("expected ok=false for empty buffer")
	}

	rb.Push(10)
	rb.Push(20)
	rb.Push(30)

	v, ok := rb.Last()
	if !ok {
		t.Error("expected ok=true")
	}
	if v != 30 {
		t.Errorf("Last() = %d, want 30", v)
	}
}

func TestRingBuffer_LastN(t *testing.T) {
	rb := NewRingBuffer[int](5)

	rb.Push(1)
	rb.Push(2)
	rb.Push(3)
	rb.Push(4)

	got := rb.LastN(2)
	if len(got) != 2 {
		t.Fatalf("expected 2 items, got %d", len(got))
	}
	// newest first
	if got[0] != 4 || got[1] != 3 {
		t.Errorf("LastN(2) = %v, want [4, 3]", got)
	}

	// Request more than available
	got = rb.LastN(10)
	if len(got) != 4 {
		t.Errorf("expected 4 items when requesting 10, got %d", len(got))
	}
}

func TestRingBuffer_LastN_AfterOverflow(t *testing.T) {
	rb := NewRingBuffer[int](3)

	rb.Push(1)
	rb.Push(2)
	rb.Push(3)
	rb.Push(4)
	rb.Push(5)

	got := rb.LastN(3)
	expected := []int{5, 4, 3}
	for i, v := range got {
		if v != expected[i] {
			t.Errorf("LastN(3)[%d] = %d, want %d", i, v, expected[i])
		}
	}
}

func TestRingBuffer_Clear(t *testing.T) {
	rb := NewRingBuffer[int](5)

	rb.Push(1)
	rb.Push(2)
	rb.Push(3)
	rb.Clear()

	if rb.Len() != 0 {
		t.Errorf("expected len 0 after clear, got %d", rb.Len())
	}

	all := rb.All()
	if all != nil {
		t.Errorf("expected nil after clear, got %v", all)
	}
}

func TestRingBuffer_Empty(t *testing.T) {
	rb := NewRingBuffer[int](5)

	all := rb.All()
	if all != nil {
		t.Errorf("expected nil for empty buffer, got %v", all)
	}

	lastN := rb.LastN(3)
	if lastN != nil {
		t.Errorf("expected nil for empty buffer LastN, got %v", lastN)
	}
}

func TestRingBuffer_MinCapacity(t *testing.T) {
	rb := NewRingBuffer[int](0)
	if rb.cap != 1 {
		t.Errorf("expected minimum capacity 1, got %d", rb.cap)
	}

	rb.Push(42)
	v, ok := rb.Last()
	if !ok || v != 42 {
		t.Errorf("expected 42, got %d (ok=%v)", v, ok)
	}
}
