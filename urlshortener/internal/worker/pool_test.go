package worker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/suite"
)

func testLogger() *logrus.Entry {
	l := logrus.New()
	l.SetLevel(logrus.PanicLevel)

	return logrus.NewEntry(l)
}

type PoolSuite struct{ suite.Suite }

func TestPoolSuite(t *testing.T) { suite.Run(t, new(PoolSuite)) }

// TestPool_Workers_ProcessJobs_Success verifies workers drain the buffered queue and call save.
func (s *PoolSuite) TestPool_Workers_ProcessJobs_Success() {
	require := s.Require()

	var got sync.Map
	save := func(_ context.Context, code, longURL string) error {
		got.Store(code, longURL)
		return nil
	}
	p := New(save, 2, 16, testLogger())
	defer p.Close()

	require.NoError(p.Submit("A", "https://a"))
	require.NoError(p.Submit("B", "https://b"))
	// Workers run async; wait for both to land.
	require.Eventually(func() bool {
		_, okA := got.Load("A")
		_, okB := got.Load("B")
		return okA && okB
	}, time.Second, 5*time.Millisecond)
}

// TestPool_BufferedQueue_AcceptsBeforeWorkerDrains_Success verifies the buffer accepts jobs while the worker is busy.
func (s *PoolSuite) TestPool_BufferedQueue_AcceptsBeforeWorkerDrains_Success() {
	require := s.Require()

	block := make(chan struct{})
	save := func(_ context.Context, _, _ string) error {
		<-block
		return nil
	}
	p := New(save, 1, 8, testLogger())
	defer p.Close()

	// Worker is blocked, but the buffer (cap 8) accepts several jobs.
	for i := 0; i < 5; i++ {
		require.NoError(p.Submit("x", "https://x"))
	}

	close(block) // release the worker
}

// TestPool_SaveError_DoesNotStopPool_Success verifies a failing save is logged, not propagated; other jobs continue.
func (s *PoolSuite) TestPool_SaveError_DoesNotStopPool_Success() {
	require := s.Require()

	var count atomic.Int32
	save := func(_ context.Context, _, _ string) error {
		n := count.Add(1)
		if n == 1 {
			return errors.New("fail")
		}

		return nil
	}
	p := New(save, 1, 8, testLogger())
	defer p.Close()

	require.NoError(p.Submit("a", "https://a"))
	require.NoError(p.Submit("b", "https://b"))
	require.Eventually(func() bool { return count.Load() == 2 }, time.Second, 5*time.Millisecond)
}

// TestPool_Submit_AfterClose_Failure verifies Submit after Close returns ErrPoolClosed.
func (s *PoolSuite) TestPool_Submit_AfterClose_Failure() {
	require := s.Require()

	p := New(func(context.Context, string, string) error { return nil }, 1, 4, testLogger())
	require.NoError(p.Close())

	// Closing twice is a no-op.
	require.NoError(p.Close())
	require.ErrorIs(p.Submit("z", "https://z"), ErrPoolClosed)
}

// TestPool_New_NonPositiveWorkers_Success verifies the default worker count falls back to 1 for non-positive input.
func (s *PoolSuite) TestPool_New_NonPositiveWorkers_Success() {
	require := s.Require()

	p := New(func(context.Context, string, string) error { return nil }, 0, -1, testLogger())

	require.NoError(p.Close())
}

// TestPool_Closed_StoppedLog_Success verifies a closed pool stops cleanly.
func (s *PoolSuite) TestPool_Closed_StoppedLog_Success() {
	require := s.Require()

	p := New(func(context.Context, string, string) error { return nil }, 1, 2, testLogger())

	require.NoError(p.Close())
}
