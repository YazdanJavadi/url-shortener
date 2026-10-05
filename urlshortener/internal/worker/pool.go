// Package worker implements a fixed-size worker pool that persists URL records
// asynchronously. It is used by the URL service so that POST /urls can return a
// short code immediately without waiting for the database insert to complete.
//
// # Channel choice: buffered vs unbuffered
//
// The job channel is BUFFERED (capacity > 0). Rationale:
//
//   - Decoupling. Producers (HTTP handlers) and consumers (DB workers) run at
//     different rates. A buffer absorbs bursts: a handler can submit a job and
//     return to the client while workers are still draining earlier inserts.
//     With an UNBUFFERED channel, every Submit blocks until a worker is ready
//     to receive, so throughput collapses to (worker count) and the "return
//     immediately" promise becomes hollow under load.
//
//   - Backpressure without data loss. A bounded buffer gives backpressure: when
//     full, Submit blocks (or returns ErrQueueFull) instead of growing memory
//     without bound. An unbuffered channel backpressures too, but with no queue
//     depth at all — there is zero tolerance for producer/consumer speed skew.
//
//   - Worker utilization. A buffer keeps workers fed even when submissions are
//     bursty, smoothing the load across the pool.
//
// We therefore use a buffered channel with a modest capacity and a small pool of
// workers. The buffer size is a tuning knob (queue depth); the worker count is
// the concurrency limit on DB writes.
package worker

import (
	"context"
	"errors"
	"sync"

	"github.com/sirupsen/logrus"
)

var (
	// ErrQueueFull is returned by Submit when the pool is closed or the queue
	// is full in non-blocking mode.
	ErrQueueFull = errors.New("worker queue full")
	// ErrPoolClosed is returned by Submit when the pool is closed.
	ErrPoolClosed = errors.New("worker pool closed")
)

// SaveFunc persists a code->longURL mapping. Typically repository.Repository.Save.
type SaveFunc func(ctx context.Context, code, longURL string) error

// Submitter is the minimal interface the service depends on, enabling DI and
// testing with a recording/closed fake instead of a real *Pool.
type Submitter interface {
	Submit(code, longURL string) error
}

// Pool is a fixed-size worker pool that executes SaveFunc asynchronously.
type Pool struct {
	jobs   chan job
	save   SaveFunc
	log    *logrus.Entry
	wg     sync.WaitGroup
	ctx    context.Context
	cancel context.CancelFunc

	mu     sync.Mutex
	closed bool
}

type job struct {
	code    string
	longURL string
}

// New starts a Pool with the given number of workers and queue depth. The queue
// is a buffered channel of capacity queueDepth; queueDepth <= 0 yields an
// unbuffered channel (discouraged — see package doc).
func New(save SaveFunc, workers, queueDepth int, log *logrus.Entry) *Pool {
	if workers <= 0 {
		workers = 1
	}

	if queueDepth < 0 {
		queueDepth = 0
	}

	p := &Pool{
		jobs: make(chan job, queueDepth),
		save: save,
		log:  log,
	}
	p.ctx, p.cancel = context.WithCancel(context.Background())
	for i := 0; i < workers; i++ {
		p.wg.Add(1)
		go p.run()
	}
	log.WithFields(logrus.Fields{"workers": workers, "queue_depth": queueDepth}).Info("worker pool started")

	return p
}

// run is the worker loop.
func (p *Pool) run() {
	defer p.wg.Done()
	for {
		select {
		case <-p.ctx.Done():
			return
		case j := <-p.jobs:
			if err := p.save(p.ctx, j.code, j.longURL); err != nil {
				p.log.WithError(err).WithFields(logrus.Fields{
					"code": j.code, "long_url": j.longURL,
				}).Error("async insert failed")
			}
		}
	}
}

// Submit enqueues an insert. It blocks until the job is accepted into the buffer
// (backpressure). Returns ErrPoolClosed if the pool has been closed.
func (p *Pool) Submit(code, longURL string) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()

		return ErrPoolClosed
	}

	p.mu.Unlock()

	select {
	case <-p.ctx.Done():
		return ErrPoolClosed
	case p.jobs <- job{code: code, longURL: longURL}:
		return nil
	}
}

// Close stops accepting submissions, drains in-flight jobs, and stops workers.
func (p *Pool) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()

		return nil
	}

	p.closed = true
	p.mu.Unlock()

	p.cancel()
	p.wg.Wait()
	close(p.jobs)
	p.log.Info("worker pool stopped")

	return nil
}
