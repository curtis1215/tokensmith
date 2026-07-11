package dailyusage

import (
	"sync"

	"tokensmith/internal/model"
)

// Buffer is an in-memory FIFO retry queue in front of a Sink.
// Failed batches stay pending with their original Day until a successful Add.
// A nil Sink is a disabled compatibility no-op.
type Buffer struct {
	mu      sync.Mutex
	sink    Sink
	pending []Batch
}

// NewBuffer wraps sink. Pass nil for a disabled buffer (Record/Flush no-op).
func NewBuffer(sink Sink) *Buffer {
	return &Buffer{sink: sink}
}

// Record enqueues a non-empty batch (has any source entry) then calls Flush.
// Empty batches still invoke Flush so prior pending work can retry.
func (b *Buffer) Record(batch Batch) error {
	if b == nil || b.sink == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(batch.Sources) > 0 {
		// Defensive copy so callers cannot mutate queued batches.
		cp := Batch{
			Day:        batch.Day,
			ObservedAt: batch.ObservedAt,
			Sources:    copySourceTotals(batch.Sources),
		}
		b.pending = append(b.pending, cp)
	}
	return b.flushLocked()
}

// Flush retries pending batches in FIFO order, removing the head only after
// a successful sink.Add.
func (b *Buffer) Flush() error {
	if b == nil || b.sink == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.flushLocked()
}

// Pending returns the number of batches waiting for a successful write.
func (b *Buffer) Pending() int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.pending)
}

func (b *Buffer) flushLocked() error {
	for len(b.pending) > 0 {
		head := b.pending[0]
		if err := b.sink.Add(head); err != nil {
			return err
		}
		// Success: drop head exactly once.
		b.pending = b.pending[1:]
	}
	return nil
}

func copySourceTotals(in map[string]model.SourceTotals) map[string]model.SourceTotals {
	if in == nil {
		return nil
	}
	out := make(map[string]model.SourceTotals, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
