package link // its intresting for my self but i used AI for helping write this

// its so coooool and fun :))

import (
	"context"
	"log"
	"sync"
	"time"
)

type UsageRecorder interface {
	Record(code string)
}

type UsageStore interface {
	IncrementUsedCount(ctx context.Context, code string, amount uint64) error
}

type Config struct {
	QueueSize      int
	FlushThreshold int
	FlushInterval  time.Duration
	FlushTimeout   time.Duration 
}

func (c Config) withDefaults() Config {
	if c.QueueSize <= 0 {
		c.QueueSize = 10_000
	}
	if c.FlushThreshold <= 0 {
		c.FlushThreshold = 700
	}
	if c.FlushInterval <= 0 {
		c.FlushInterval = 5 * time.Second
	}
	if c.FlushTimeout <= 0 {
		c.FlushTimeout = 5 * time.Second
	}
	return c
}

type Counter struct {
	events chan string
	store  UsageStore
	cfg    Config

	done      chan struct{} 
	closeOnce sync.Once
	stop      chan struct{}
}

func NewCounter(store UsageStore, cfg Config) *Counter {
	cfg = cfg.withDefaults()
	c := &Counter{
		events: make(chan string, cfg.QueueSize),
		store:  store,
		cfg:    cfg,
		done:   make(chan struct{}),
		stop:   make(chan struct{}),
	}
	go c.run()
	return c
}

func (c *Counter) Record(code string) {
	select {
	case c.events <- code:
	default:
		// que is full so drop
	}
}

func (c *Counter) run() {
	defer close(c.done)

	counts := make(map[string]int64)
	pending := 0

	ticker := time.NewTicker(c.cfg.FlushInterval)
	defer ticker.Stop()

	flush := func() {
		if pending == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), c.cfg.FlushTimeout)
		defer cancel()

		for code, n := range counts {
			if err := c.store.IncrementUsedCount(ctx, code, uint64(n)); err != nil {
				log.Printf("flush used_count failed for %s: %v", code, err)
			}
		}
		counts = make(map[string]int64)
		pending = 0
	}

	for {
		select {
		case code := <-c.events:
			counts[code]++
			pending++
			if pending >= c.cfg.FlushThreshold {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-c.stop:
			for {
				select {
				case code := <-c.events:
					counts[code]++
					pending++
				default:
					flush()
					return
				}
			}
		}
	}
}

func (c *Counter) Close(ctx context.Context) error {
	c.closeOnce.Do(func() { close(c.stop) })
	select {
	case <-c.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
