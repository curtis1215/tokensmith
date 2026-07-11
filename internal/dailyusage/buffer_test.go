package dailyusage

import (
	"errors"
	"sync"
	"testing"

	"tokensmith/internal/model"
)

type fakeSink struct {
	mu       sync.Mutex
	failures int
	saved    []Batch
	err      error // permanent error if set
}

func (f *fakeSink) Add(b Batch) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	if f.failures > 0 {
		f.failures--
		return errors.New("temporary failure")
	}
	f.saved = append(f.saved, b)
	return nil
}

func TestBufferRetriesOriginalBatchExactlyOnce(t *testing.T) {
	sink := &fakeSink{failures: 1}
	b := NewBuffer(sink)
	original := Batch{Day: "2026-07-12", ObservedAt: 10, Sources: map[string]model.SourceTotals{"grok": {In: 42}}}
	if err := b.Record(original); err == nil || b.Pending() != 1 {
		t.Fatalf("want pending after fail: err=%v pending=%d", err, b.Pending())
	}
	if err := b.Flush(); err != nil {
		t.Fatal(err)
	}
	if b.Pending() != 0 || len(sink.saved) != 1 || sink.saved[0].Day != "2026-07-12" {
		t.Fatalf("bad retry: pending=%d saved=%+v", b.Pending(), sink.saved)
	}
	if sink.saved[0].Sources["grok"].In != 42 {
		t.Fatalf("lost original batch: %+v", sink.saved[0])
	}
	if err := b.Flush(); err != nil || len(sink.saved) != 1 {
		t.Fatalf("duplicate or err: err=%v len=%d", err, len(sink.saved))
	}
}

func TestBufferEmptyRecordStillFlushesPending(t *testing.T) {
	sink := &fakeSink{failures: 1}
	b := NewBuffer(sink)
	_ = b.Record(Batch{Day: "2026-07-12", ObservedAt: 1, Sources: map[string]model.SourceTotals{"codex": {In: 1}}})
	if b.Pending() != 1 {
		t.Fatal("want pending")
	}
	// Empty batch: no new sources, but should flush previous pending.
	if err := b.Record(Batch{Day: "2026-07-12", ObservedAt: 2, Sources: nil}); err != nil {
		t.Fatal(err)
	}
	if b.Pending() != 0 || len(sink.saved) != 1 {
		t.Fatalf("pending=%d saved=%d", b.Pending(), len(sink.saved))
	}
}

func TestBufferNilSinkIsNoop(t *testing.T) {
	b := NewBuffer(nil)
	if err := b.Record(Batch{Day: "2026-07-12", ObservedAt: 1, Sources: map[string]model.SourceTotals{"codex": {In: 1}}}); err != nil {
		t.Fatal(err)
	}
	if b.Pending() != 0 {
		t.Fatalf("nil sink pending=%d", b.Pending())
	}
	if err := b.Flush(); err != nil {
		t.Fatal(err)
	}
}

func TestBufferFIFOOrder(t *testing.T) {
	sink := &fakeSink{failures: 2} // fail first two Flush attempts (first Record + first Flush? wait)
	// Record A fails → pending [A]
	// Record B: append B then Flush → fail on A → pending [A,B]
	// Better: permanent fail for first batch attempts then succeed.
	sink = &fakeSink{failures: 1}
	b := NewBuffer(sink)
	a := Batch{Day: "2026-07-11", ObservedAt: 1, Sources: map[string]model.SourceTotals{"codex": {In: 1}}}
	bb := Batch{Day: "2026-07-12", ObservedAt: 2, Sources: map[string]model.SourceTotals{"codex": {In: 2}}}
	_ = b.Record(a) // fails, pending [A]
	// Next Record will append B then Flush: first success removes A, then B also succeeds.
	if err := b.Record(bb); err != nil {
		t.Fatal(err)
	}
	if b.Pending() != 0 {
		t.Fatalf("pending=%d", b.Pending())
	}
	if len(sink.saved) != 2 {
		t.Fatalf("saved=%d", len(sink.saved))
	}
	if sink.saved[0].Day != "2026-07-11" || sink.saved[1].Day != "2026-07-12" {
		t.Fatalf("order=%+v", sink.saved)
	}
}

func TestBufferDoesNotEnqueueEmptyBatch(t *testing.T) {
	sink := &fakeSink{}
	b := NewBuffer(sink)
	if err := b.Record(Batch{Day: "2026-07-12", ObservedAt: 1, Sources: map[string]model.SourceTotals{}}); err != nil {
		t.Fatal(err)
	}
	if len(sink.saved) != 0 || b.Pending() != 0 {
		t.Fatalf("empty batch should not enqueue: saved=%d pending=%d", len(sink.saved), b.Pending())
	}
}
