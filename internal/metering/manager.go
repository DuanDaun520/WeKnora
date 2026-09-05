// Package metering implements WeKnora's own model-usage ledger. It is the
// billing data source described in docs/Token统计与计费设计.md and is fully
// independent of Langfuse: the meter wrappers in internal/models/* append a
// record for every model call that consumed tokens (failed and interrupted
// calls included), and this package buffers + batch-inserts them.
//
// Reliability contract: counting only, no deduction. Submit is non-blocking
// and may drop records under backpressure or on insert failure — a small
// loss window is explicitly accepted by the design, so there is no retry
// and no blocking of model calls.
package metering

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
)

const (
	// defaultQueueSize bounds in-flight records. Beyond this Submit drops
	// (counted) rather than blocking a model call.
	defaultQueueSize = 8192
	// defaultFlushInterval is the max latency from Submit to insert.
	defaultFlushInterval = 5 * time.Second
	// defaultBatchSize is the max rows per INSERT statement.
	defaultBatchSize = 256
)

// Manager owns the record queue and the background flusher. A singleton is
// installed via Init(); callers must tolerate a nil *Manager — every public
// method is nil-safe, so wrappers can call unconditionally.
type Manager struct {
	db            *gorm.DB
	queue         chan *types.ModelUsageRecord
	flushInterval time.Duration
	batchSize     int

	done    chan struct{}
	stopped sync.WaitGroup

	closed         atomic.Bool
	droppedFull    atomic.Uint64
	droppedNoScope atomic.Uint64
	insertFailed   atomic.Uint64
}

var (
	globalMu sync.RWMutex
	global   *Manager
)

// Init installs the package-wide singleton and starts its flusher. Safe to
// call once at container setup; db == nil leaves everything a no-op.
func Init(db *gorm.DB) *Manager {
	m := &Manager{
		db:            db,
		queue:         make(chan *types.ModelUsageRecord, defaultQueueSize),
		flushInterval: defaultFlushInterval,
		batchSize:     defaultBatchSize,
		done:          make(chan struct{}),
	}
	if db != nil {
		m.stopped.Add(1)
		go m.run()
	}
	globalMu.Lock()
	global = m
	globalMu.Unlock()
	logger.Infof(context.Background(),
		"[Metering] usage ledger enabled (queue=%d flush=%s batch=%d)",
		defaultQueueSize, defaultFlushInterval, defaultBatchSize)
	return m
}

// GetManager returns the installed singleton, or nil before Init. Callers
// must tolerate nil.
func GetManager() *Manager {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return global
}

// Submit enqueues one record without blocking. Nil-manager, post-shutdown,
// unattributable (tenant 0) and zero-consumption records are skipped.
func (m *Manager) Submit(rec *types.ModelUsageRecord) {
	if m == nil || rec == nil || m.closed.Load() || m.db == nil {
		return
	}
	if rec.TenantID == 0 {
		// Nothing to attribute the consumption to — cannot appear in any
		// of the three report levels. Counted, not stored.
		m.droppedNoScope.Add(1)
		return
	}
	if rec.InputTokens == 0 && rec.OutputTokens == 0 && len(rec.Extra) == 0 {
		return
	}
	if rec.OccurredAt.IsZero() {
		rec.OccurredAt = time.Now().UTC()
	}
	select {
	case m.queue <- rec:
	default:
		n := m.droppedFull.Add(1)
		if n%1000 == 1 {
			logger.Warnf(context.Background(),
				"[Metering] queue full, dropped %d records so far", n)
		}
	}
}

// Shutdown drains the queue and inserts what remains, bounded by ctx.
// Registered on the ResourceCleaner so buffered usage survives a graceful
// restart.
func (m *Manager) Shutdown(ctx context.Context) error {
	if m == nil || m.db == nil || !m.closed.CompareAndSwap(false, true) {
		return nil
	}
	close(m.done)
	m.stopped.Wait()
	// run() flushed its batch on exit; sweep the queue once more for any
	// record that raced Submit's closed check.
	rest := make([]*types.ModelUsageRecord, 0, cap(m.queue))
sweep:
	for {
		select {
		case rec := <-m.queue:
			rest = append(rest, rec)
		default:
			break sweep
		}
	}
	m.flush(ctx, rest...)
	logger.Infof(context.Background(),
		"[Metering] shut down (dropped_full=%d dropped_unattributed=%d insert_failed=%d)",
		m.droppedFull.Load(), m.droppedNoScope.Load(), m.insertFailed.Load())
	return nil
}

// run is the flusher loop: drains the queue into batches, flushing on
// batch-full or ticker.
func (m *Manager) run() {
	defer m.stopped.Done()
	ticker := time.NewTicker(m.flushInterval)
	defer ticker.Stop()
	batch := make([]*types.ModelUsageRecord, 0, m.batchSize)
	for {
		select {
		case rec := <-m.queue:
			batch = append(batch, rec)
			if len(batch) >= m.batchSize {
				m.flush(context.Background(), batch...)
				batch = batch[:0]
			}
		case <-ticker.C:
			// Drain whatever is immediately available without blocking.
			for len(batch) < m.batchSize {
				select {
				case rec := <-m.queue:
					batch = append(batch, rec)
				default:
					goto flushed
				}
			}
		flushed:
			if len(batch) > 0 {
				m.flush(context.Background(), batch...)
				batch = batch[:0]
			}
		case <-m.done:
			// Drain-and-flush before exiting so records submitted just
			// before Shutdown are not lost with the local batch.
			for {
				select {
				case rec := <-m.queue:
					batch = append(batch, rec)
				default:
					m.flush(context.Background(), batch...)
					return
				}
			}
		}
	}
}

// flush inserts one batch. Failure is logged and counted — no retry by
// design.
func (m *Manager) flush(ctx context.Context, records ...*types.ModelUsageRecord) {
	if len(records) == 0 {
		return
	}
	if err := m.db.WithContext(ctx).CreateInBatches(records, m.batchSize).Error; err != nil {
		n := m.insertFailed.Add(uint64(len(records)))
		logger.Warnf(ctx, "[Metering] insert failed (%d records lost, %d total): %v",
			len(records), n, err)
	}
}

// Stats returns the loss counters (observability for the report page /
// logs).
func (m *Manager) Stats() (droppedFull, droppedUnattributed, insertFailed uint64) {
	if m == nil {
		return 0, 0, 0
	}
	return m.droppedFull.Load(), m.droppedNoScope.Load(), m.insertFailed.Load()
}
