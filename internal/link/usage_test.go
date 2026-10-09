package link

import (
	"context"
	"errors"
	"io"
	"log"
	"sync"
	"testing"
	"time"
)

type fakeUsageStore struct {
	mu     sync.Mutex
	got    map[string]uint64
	calls  int
	err    error
	block  chan struct{} 
	called chan struct{} 
}

func newFakeUsageStore() *fakeUsageStore {
	return &fakeUsageStore{
		got:    make(map[string]uint64),
		called: make(chan struct{}, 64),
	}
}

func (f *fakeUsageStore) IncrementUsedCount(_ context.Context, code string, amount uint64) error {
	if f.block != nil {
		<-f.block
	}
	f.mu.Lock()
	f.got[code] += amount
	f.calls++
	f.mu.Unlock()

	select {
	case f.called <- struct{}{}:
	default:
	}
	return f.err
}

func (f *fakeUsageStore) snapshot() (map[string]uint64, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make(map[string]uint64, len(f.got))
	for k, v := range f.got {
		cp[k] = v
	}
	return cp, f.calls
}

func (f *fakeUsageStore) waitCalls(t *testing.T, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		select {
		case <-f.called:
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for store call %d/%d", i+1, n)
		}
	}
}

func closeCounter(t *testing.T, c *Counter) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Close(ctx); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func silenceLog(t *testing.T) {
	t.Helper()
	orig := log.Writer()
	log.SetOutput(io.Discard)
	t.Cleanup(func() { log.SetOutput(orig) })
}

func TestConfigWithDefaults(t *testing.T) {
	t.Run("zero values get defaults", func(t *testing.T) {
		got := Config{}.withDefaults()
		want := Config{
			QueueSize:      10_000,
			FlushThreshold: 700,
			FlushInterval:  5 * time.Second,
			FlushTimeout:   5 * time.Second,
		}
		if got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("negative values get defaults", func(t *testing.T) {
		got := Config{QueueSize: -1, FlushThreshold: -1, FlushInterval: -1, FlushTimeout: -1}.withDefaults()
		if got.QueueSize != 10_000 || got.FlushThreshold != 700 {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("custom values are kept", func(t *testing.T) {
		in := Config{
			QueueSize:      5,
			FlushThreshold: 3,
			FlushInterval:  time.Second,
			FlushTimeout:   2 * time.Second,
		}
		if got := in.withDefaults(); got != in {
			t.Errorf("got %+v, want %+v", got, in)
		}
	})
}

func TestCounterFlushOnThreshold(t *testing.T) {
	fs := newFakeUsageStore()
	c := NewCounter(fs, Config{FlushThreshold: 3, FlushInterval: time.Hour})

	c.Record("aaa111")
	c.Record("aaa111")
	c.Record("bbb222") 

	fs.waitCalls(t, 2) 

	got, calls := fs.snapshot()
	if calls != 2 {
		t.Errorf("calls = %d, want 2 (aggregated per code)", calls)
	}
	if got["aaa111"] != 2 || got["bbb222"] != 1 {
		t.Errorf("counts = %v, want aaa111=2 bbb222=1", got)
	}

	closeCounter(t, c)
}

func TestCounterFlushOnTicker(t *testing.T) {
	fs := newFakeUsageStore()
	c := NewCounter(fs, Config{FlushThreshold: 1000, FlushInterval: 10 * time.Millisecond})

	c.Record("aaa111")
	fs.waitCalls(t, 1)

	time.Sleep(60 * time.Millisecond)

	got, calls := fs.snapshot()
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (empty ticks must not hit the store)", calls)
	}
	if got["aaa111"] != 1 {
		t.Errorf("counts = %v", got)
	}

	closeCounter(t, c)
}

func TestCounterCloseFlushesPending(t *testing.T) {
	fs := newFakeUsageStore()
	c := NewCounter(fs, Config{FlushThreshold: 1000, FlushInterval: time.Hour})

	for i := 0; i < 5; i++ {
		c.Record("aaa111")
	}
	c.Record("bbb222")

	closeCounter(t, c)

	got, _ := fs.snapshot()
	if got["aaa111"] != 5 || got["bbb222"] != 1 {
		t.Errorf("counts = %v, want aaa111=5 bbb222=1", got)
	}
}

func TestCounterCloseWithNothingPending(t *testing.T) {
	fs := newFakeUsageStore()
	c := NewCounter(fs, Config{FlushInterval: time.Hour})

	closeCounter(t, c)

	if _, calls := fs.snapshot(); calls != 0 {
		t.Errorf("calls = %d, want 0", calls)
	}
}

func TestCounterCloseIsIdempotent(t *testing.T) {
	c := NewCounter(newFakeUsageStore(), Config{FlushInterval: time.Hour})

	closeCounter(t, c)
	closeCounter(t, c)
}

func TestCounterFlushErrorIsLoggedNotFatal(t *testing.T) {
	silenceLog(t)

	fs := newFakeUsageStore()
	fs.err = errors.New("db down")
	c := NewCounter(fs, Config{FlushThreshold: 1000, FlushInterval: time.Hour})

	c.Record("aaa111")
	c.Record("bbb222")
	closeCounter(t, c)

	if _, calls := fs.snapshot(); calls != 2 {
		t.Errorf("calls = %d, want 2 (error must not stop other codes)", calls)
	}
}

func TestCounterRecordDropsWhenQueueFull(t *testing.T) {
	c := &Counter{events: make(chan string, 2)}

	c.Record("aaa111")
	c.Record("bbb222")
	c.Record("ccc333")

	done := make(chan struct{})
	go func() {
		c.Record("ddd444")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Record blocked on a full queue")
	}

	if got := len(c.events); got != 2 {
		t.Errorf("queue len = %d, want 2", got)
	}
	if first := <-c.events; first != "aaa111" {
		t.Errorf("first = %q, want aaa111", first)
	}
}

func TestCounterCloseTimeout(t *testing.T) {
	fs := newFakeUsageStore()
	fs.block = make(chan struct{})
	c := NewCounter(fs, Config{FlushThreshold: 1000, FlushInterval: time.Hour})

	c.Record("aaa111")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err := c.Close(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close err = %v, want DeadlineExceeded", err)
	}

	close(fs.block)
	closeCounter(t, c)

	if got, _ := fs.snapshot(); got["aaa111"] != 1 {
		t.Errorf("counts = %v, want aaa111=1", got)
	}
}