package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync"
)

const hashRounds = 50_000

func heavyHash(data []byte) [sha256.Size]byte {
	sum := sha256.Sum256(data)
	for i := 0; i < hashRounds; i++ {
		sum = sha256.Sum256(sum[:])
	}
	return sum
}

var ErrOverloaded = errors.New("worker pool: queue is full")

type job struct {
	ctx  context.Context
	data []byte
	res  chan [sha256.Size]byte
}

type WorkerPool struct {
	jobs chan job
	wg   sync.WaitGroup
}

func NewWorkerPool(workers, queue int) *WorkerPool {
	p := &WorkerPool{jobs: make(chan job, queue)}
	p.wg.Add(workers)
	for i := 0; i < workers; i++ {
		go p.worker()
	}
	return p
}

func (p *WorkerPool) worker() {
	defer p.wg.Done()
	for j := range p.jobs {
		if j.ctx.Err() != nil {
			continue
		}
		j.res <- heavyHash(j.data)
	}
}

func (p *WorkerPool) Do(ctx context.Context, data []byte) ([sha256.Size]byte, error) {
	var zero [sha256.Size]byte
	j := job{ctx: ctx, data: data, res: make(chan [sha256.Size]byte, 1)}

	select {
	case p.jobs <- j:
	default:
		return zero, ErrOverloaded
	}

	select {
	case sum := <-j.res:
		return sum, nil
	case <-ctx.Done():
		return zero, ctx.Err()
	}
}

func (p *WorkerPool) Close() {
	close(p.jobs)
	p.wg.Wait()
}
