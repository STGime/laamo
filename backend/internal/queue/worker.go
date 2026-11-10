package queue

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"runtime/debug"
	"sync"
	"time"
)

// Handler runs one job. Return nil when done, Permanent(err) for errors a
// retry won't fix, any other error to retry with backoff. The context is
// cancelled when the worker shuts down or loses the job's lease; handlers
// must stop then. Handlers must be idempotent: a job can run more than once
// (e.g. after a crash), so side effects check their own keys first.
type Handler func(ctx context.Context, j Job) error

// Worker polls the queue and runs jobs. Polling is the source of truth;
// Wake (e.g. realtime inserts on jobs) only shortens the wait.
type Worker struct {
	Queue    Queue
	ID       string             // unique per process, e.g. hostname-pid
	Handlers map[string]Handler // by job kind

	Batch   int             // jobs per claim (default 10)
	Poll    time.Duration   // wait when idle (default 1s)
	Wake    <-chan struct{} // optional: something was enqueued
	Lease   time.Duration   // lease per extension (default LeaseDuration)
	Backoff func(attempt int) time.Duration
	// OnFailed is called when a job ends failed (permanent error or out of
	// attempts), e.g. to post a Problem card. Optional.
	OnFailed func(ctx context.Context, j Job, cause error)
	Log      *slog.Logger
}

// Run claims and runs jobs until ctx is cancelled. Jobs in flight get their
// context cancelled and are put back in the queue for another worker.
func (w *Worker) Run(ctx context.Context) error {
	w.defaults()
	w.Log.Info("worker started", "worker", w.ID)
	for {
		if ctx.Err() != nil {
			w.Log.Info("worker stopped", "worker", w.ID)
			return nil
		}
		jobs, err := w.Queue.Claim(ctx, w.ID, w.Batch)
		if err != nil {
			if ctx.Err() == nil {
				w.Log.Error("claim failed", "worker", w.ID, "err", err)
			}
			w.wait(ctx)
			continue
		}
		if len(jobs) == 0 {
			w.wait(ctx)
			continue
		}
		var wg sync.WaitGroup
		for _, j := range jobs {
			wg.Go(func() { w.process(ctx, j) })
		}
		wg.Wait()
	}
}

func (w *Worker) defaults() {
	if w.Batch <= 0 {
		w.Batch = 10
	}
	if w.Poll <= 0 {
		w.Poll = time.Second
	}
	if w.Lease <= 0 {
		w.Lease = LeaseDuration
	}
	if w.Backoff == nil {
		w.Backoff = DefaultBackoff
	}
	if w.Log == nil {
		w.Log = slog.Default()
	}
}

func (w *Worker) wait(ctx context.Context) {
	t := time.NewTimer(w.Poll)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	case <-w.Wake: // nil channel blocks forever: fine
	}
}

// UnknownKindDelay is how long a job of an unknown kind waits before the
// next try.
const UnknownKindDelay = time.Minute

// DefaultBackoff: 30s, 1m, 2m, … up to 30m, ±20% jitter.
func DefaultBackoff(attempt int) time.Duration {
	d := min(30*time.Second<<min(max(attempt-1, 0), 6), 30*time.Minute)
	jitter := time.Duration(rand.Int64N(int64(d)/5+1)) - d/10 //nolint:gosec // jitter, not security
	return d + jitter
}

func (w *Worker) process(ctx context.Context, j Job) {
	log := w.Log.With("worker", w.ID, "job", j.ID, "kind", j.Kind, "attempt", j.Attempts)
	h, ok := w.Handlers[j.Kind]
	if !ok {
		// Not permanent: during a rolling deploy an old worker can claim a
		// kind only new workers know. Hand it back for them (this uses one
		// attempt; after max_attempts the job fails).
		cause := fmt.Errorf("worker %s has no handler for job kind %q", w.ID, j.Kind)
		failed, err := w.Queue.Retry(ctx, j, cause, UnknownKindDelay)
		switch {
		case err != nil:
			log.Error("could not hand back job of unknown kind", "err", err)
		case failed:
			log.Error("job failed: no worker handles its kind", "err", cause)
			w.onFailed(ctx, j, cause)
		default:
			log.Warn("no handler for job kind; handed back", "after", UnknownKindDelay)
		}
		return
	}

	hctx, cancel := context.WithCancel(ctx)
	defer cancel()
	lost := make(chan struct{})
	stop := w.heartbeat(hctx, cancel, log, j, lost)

	err := safeCall(hctx, h, j)
	stop()

	select {
	case <-lost:
		log.Warn("lease lost; another worker owns the job now", "handler_err", err)
		return
	default:
	}

	// Finishing must happen even while shutting down.
	fctx, fcancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer fcancel()

	switch {
	case err == nil: // finished, even if shutdown started meanwhile
		if cerr := w.Queue.Complete(fctx, j); cerr != nil {
			log.Error("complete failed", "err", cerr)
		}
	case ctx.Err() != nil:
		// Shutdown interrupted the handler: hand the job back right away
		// instead of waiting for the lease to expire. The attempt still counts.
		if _, rerr := w.Queue.Retry(fctx, j, errors.New("worker shut down"), 0); rerr != nil && !errors.Is(rerr, ErrLeaseLost) {
			log.Error("could not hand back job on shutdown", "err", rerr)
		}
	case errors.Is(err, ErrPermanent):
		w.finishFailed(fctx, log, j, err)
	default:
		after := w.Backoff(j.Attempts)
		failed, rerr := w.Queue.Retry(fctx, j, err, after)
		switch {
		case rerr != nil:
			log.Error("retry failed", "err", rerr, "cause", err)
		case failed:
			log.Warn("job failed after last attempt", "err", err)
			w.onFailed(fctx, j, err)
		default:
			log.Info("job will retry", "err", err, "after", after)
		}
	}
}

func (w *Worker) finishFailed(ctx context.Context, log *slog.Logger, j Job, cause error) {
	if err := w.Queue.Fail(ctx, j, cause); err != nil {
		log.Error("fail failed", "err", err, "cause", cause)
		return
	}
	log.Warn("job failed", "err", cause)
	w.onFailed(ctx, j, cause)
}

func (w *Worker) onFailed(ctx context.Context, j Job, cause error) {
	if w.OnFailed != nil {
		w.OnFailed(ctx, j, cause)
	}
}

// heartbeat extends the lease every Lease/3 while the handler runs. If the
// lease is lost it closes lost and cancels the handler. stop ends it.
func (w *Worker) heartbeat(ctx context.Context, cancel context.CancelFunc, log *slog.Logger, j Job, lost chan struct{}) (stop func()) {
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		t := time.NewTicker(w.Lease / 3)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				err := w.Queue.Extend(ctx, j, w.Lease)
				switch {
				case errors.Is(err, ErrLeaseLost):
					close(lost)
					cancel()
					return
				case err != nil && ctx.Err() == nil:
					// Keep going: the next tick may work, and the lease
					// still has two thirds left.
					log.Warn("lease extension failed", "err", err)
				}
			}
		}
	})
	return func() {
		close(done)
		wg.Wait()
	}
}

func safeCall(ctx context.Context, h Handler, j Job) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("handler panic: %v\n%s", r, debug.Stack())
		}
	}()
	return h(ctx, j)
}
