package urlservice

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/yazdanjavadi/urlshort/internal/worker"
)

// workerErrPoolClosed mirrors worker.ErrPoolClosed for assertion convenience.
var workerErrPoolClosed = worker.ErrPoolClosed

// recordingPool wraps a real worker.Pool and records Submit calls so tests can
// assert that the service enqueued an async insert without waiting for the DB.
// It satisfies worker.Submitter.
type recordingPool struct {
	*worker.Pool
	mu    sync.Mutex
	count int
}

func newRecordingPool(t *testing.T) *recordingPool {
	save := func(context.Context, string, string) error { return nil }
	p := worker.New(save, 1, 8, testLogger())
	rp := &recordingPool{Pool: p}
	t.Cleanup(func() { _ = p.Close() })

	return rp
}

// Submit overrides the embedded Pool's Submit to count calls. Because the
// service holds a worker.Submitter (interface), this override IS used.
func (r *recordingPool) Submit(code, longURL string) error {
	r.mu.Lock()
	r.count++
	r.mu.Unlock()

	return r.Pool.Submit(code, longURL)
}

// assertSubmitted waits until at least n submissions were recorded.
func (r *recordingPool) assertSubmitted(t *testing.T, n int) {
	t.Helper()
	assert.Eventually(t, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.count >= n
	}, time.Second, 10*time.Millisecond, "expected >= %d submissions, got %d", n, r.count)
}

// newClosedPool returns a pool that is already closed, so Submit fails fast.
func newClosedPool() *worker.Pool {
	save := func(context.Context, string, string) error { return nil }
	p := worker.New(save, 1, 8, testLogger())
	_ = p.Close()

	return p
}
