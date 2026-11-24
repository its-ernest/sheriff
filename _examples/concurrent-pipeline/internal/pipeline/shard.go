package pipeline

import "sync"

// Shards is the number of shards.
const Shards = 4

// Aggregator collects per-shard counts.
type Aggregator struct {
	mu     sync.Mutex
	counts [Shards]int
}

// Add records one for shard.
func (a *Aggregator) Add(shard int) {
	a.mu.Lock()
	a.counts[shard]++
	a.mu.Unlock()
}

// Total returns the sum of all shards.
func (a *Aggregator) Total() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	total := 0
	for _, c := range a.counts {
		total += c
	}
	return total
}

// Sample demonstrates indexing a fixed shard table with an out-of-range shard.
func Sample() int {
	var counts [Shards]int
	shard := 7
	return counts[shard]
}
