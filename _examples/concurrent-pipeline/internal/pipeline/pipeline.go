package pipeline

import "sync"

// Pipeline fans work out to a fixed number of workers.
type Pipeline struct {
	agg    *Aggregator
	events chan int
	wg     sync.WaitGroup
}

// New creates a pipeline that records into agg.
func New(agg *Aggregator) *Pipeline {
	return &Pipeline{agg: agg, events: make(chan int)}
}

// Start launches n workers.
func (p *Pipeline) Start(n int) {
	for i := 0; i < n; i++ {
		p.wg.Add(1)
		go p.worker()
	}
}

func (p *Pipeline) worker() {
	defer p.wg.Done()
	for shard := range p.events {
		p.agg.Add(shard % Shards)
	}
}

// Submit sends a shard to the workers.
func (p *Pipeline) Submit(shard int) {
	p.events <- shard
}

// Stop closes the channel and waits for the workers to finish.
func (p *Pipeline) Stop() {
	close(p.events)
	p.wg.Wait()
}
