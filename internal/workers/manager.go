package workers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"pi-web/internal/chat"
)

type State string

const (
	WorkerStateIdle    State = "idle"
	WorkerStateRunning State = "running"
	WorkerStateError   State = "error"
	WorkerStateUnknown State = "unknown"
	WorkerStateAbsent  State = "absent"
)

type WorkerStatus struct {
	State         State  `json:"state"`
	Error         string `json:"error,omitempty"`
	Model         string `json:"model,omitempty"`
	ModelName     string `json:"modelName,omitempty"`
	ModelProvider string `json:"modelProvider,omitempty"`
	ThinkingLevel string `json:"thinkingLevel,omitempty"`
	CheckedAt     int64  `json:"checkedAt,omitempty"`
}

// SlashCommand is a slash-invokable command exposed by a pi worker via the
// get_commands RPC. Source is one of "extension", "prompt", or "skill".
type SlashCommand struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"`
}

// WorkerSnapshot is a point-in-time view of a single live worker for the
// metrics dashboard. PID/UptimeS/IdleForS are only populated for workers that
// implement the optional inspector interface (real pi processes); test fakes
// that don't will report zero values for those fields.
type WorkerSnapshot struct {
	SessionID string  `json:"session_id"`
	PID       int     `json:"pid"`
	State     State   `json:"state"`
	Model     string  `json:"model,omitempty"`
	UptimeS   float64 `json:"uptime_s"`
	IdleForS  float64 `json:"idle_for_s"`
	Error     string  `json:"error,omitempty"`
	CheckedAt int64   `json:"checked_at,omitempty"`
}

// inspector is the optional interface a worker implements to expose
// process-level details for the metrics dashboard.
type inspector interface {
	PID() int
	StartedAt() time.Time
	IdleSince(now time.Time) time.Duration
}

type ChatWorker interface {
	Prompt(ctx context.Context, chat chat.Request) error
	SetModel(ctx context.Context, provider, modelID string) error
	SetThinkingLevel(ctx context.Context, level string) error
	Abort(ctx context.Context) error
	GetState(ctx context.Context) (WorkerStatus, error)
	GetCommands(ctx context.Context) ([]SlashCommand, error)
	Status() WorkerStatus
	Close() error
}

type Factory func(sessionID, sessionPath string) (ChatWorker, error)

type Manager struct {
	mu       sync.Mutex
	workers  map[string]ChatWorker
	creating map[string]*createCall
	factory  Factory

	// pendingSends counts Send calls that have been accepted but whose prompt
	// has not been acked yet. Spawning a worker (process start + switch_session
	// + get_state) can take seconds, and the worker only reports Running once
	// Prompt lands — without this, Status dips to idle mid-send, letting the
	// queue drainer dispatch queued items into a run that is still starting and
	// making status polls fire a spurious idle transition.
	pendingSends map[string]int
	operations   map[string]int
	epochs       map[string]uint64
	retiring     map[string]chan struct{}

	idleTTL    time.Duration
	reaperStop chan struct{}
	reaperDone chan struct{}
}

type createCall struct {
	done   chan struct{}
	worker ChatWorker
	err    error
}

// DefaultIdleTTL is how long a worker may sit idle before the reaper closes it.
// The metrics dashboard uses it as the threshold for flagging "zombie" workers.
const DefaultIdleTTL = 10 * time.Minute

func NewManager(factory Factory) *Manager {
	return NewManagerWithTTL(factory, DefaultIdleTTL)
}

// NewManagerWithTTL is the same as NewManager but lets callers override the
// idle TTL. A non-positive ttl disables reaping.
func NewManagerWithTTL(factory Factory, ttl time.Duration) *Manager {
	m := &Manager{
		workers:      make(map[string]ChatWorker),
		creating:     make(map[string]*createCall),
		factory:      factory,
		pendingSends: make(map[string]int),
		operations:   make(map[string]int),
		epochs:       make(map[string]uint64),
		retiring:     make(map[string]chan struct{}),
		idleTTL:      ttl,
		reaperStop:   make(chan struct{}),
		reaperDone:   make(chan struct{}),
	}
	if ttl > 0 {
		go m.reapLoop()
	} else {
		close(m.reaperDone)
	}
	return m
}

func (m *Manager) reapLoop() {
	defer close(m.reaperDone)
	interval := m.idleTTL / 5
	if interval < time.Second {
		interval = time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-m.reaperStop:
			return
		case now := <-t.C:
			m.reapOnce(now)
		}
	}
}

func (m *Manager) reapOnce(now time.Time) {
	type candidate struct {
		id     string
		worker ChatWorker
		epoch  uint64
	}
	m.mu.Lock()
	var candidates []candidate
	for id, w := range m.workers {
		reaper, ok := w.(idleReportable)
		if ok && m.operations[id] == 0 && m.pendingSends[id] == 0 && w.Status().State == WorkerStateIdle && reaper.IdleSince(now) > m.idleTTL {
			candidates = append(candidates, candidate{id, w, m.epochs[id]})
		}
	}
	m.mu.Unlock()
	for _, c := range candidates {
		// Never wait for RPC while holding the manager mutex.
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		state, err := c.worker.GetState(ctx)
		cancel()
		if err != nil || state.State != WorkerStateIdle {
			continue
		}
		m.mu.Lock()
		if m.workers[c.id] != c.worker || m.epochs[c.id] != c.epoch || m.operations[c.id] != 0 || m.pendingSends[c.id] != 0 || c.worker.Status().State != WorkerStateIdle || c.worker.(idleReportable).IdleSince(time.Now()) <= m.idleTTL {
			m.mu.Unlock()
			continue
		}
		if fence, ok := c.worker.(interface{ ClaimIdleClose(time.Duration) bool }); ok && !fence.ClaimIdleClose(m.idleTTL) {
			m.mu.Unlock()
			continue
		}
		m.retiring[c.id] = make(chan struct{})
		delete(m.workers, c.id)
		m.mu.Unlock()
		m.finishRetirement(c.id, c.worker, "confirmed_idle_ttl")
	}
}

func (m *Manager) beginOperation(id string) {
	m.mu.Lock()
	m.operations[id]++
	m.epochs[id]++
	m.mu.Unlock()
}

func (m *Manager) endOperation(id string) {
	m.mu.Lock()
	m.operations[id]--
	if m.operations[id] == 0 {
		delete(m.operations, id)
	}
	m.mu.Unlock()
}

func (m *Manager) finishRetirement(id string, w ChatWorker, reason string) {
	pid := 0
	if ins, ok := w.(inspector); ok {
		pid = ins.PID()
	}
	fmt.Fprintf(os.Stderr, "%s worker_retire session=%q pid=%d reason=%s\n", time.Now().UTC().Format(time.RFC3339Nano), id, pid, reason)
	_ = w.Close()
	m.mu.Lock()
	close(m.retiring[id])
	delete(m.retiring, id)
	m.mu.Unlock()
}

func (m *Manager) Send(ctx context.Context, sessionID, sessionPath string, chat chat.Request) error {
	m.beginOperation(sessionID)
	defer m.endOperation(sessionID)
	m.mu.Lock()
	m.pendingSends[sessionID]++
	m.mu.Unlock()
	defer func() {
		// By the time Prompt returns (acked), the worker itself reports
		// Running, so the pending mark and the worker status overlap and the
		// session never observably dips to idle mid-send.
		m.mu.Lock()
		if m.pendingSends[sessionID]--; m.pendingSends[sessionID] <= 0 {
			delete(m.pendingSends, sessionID)
		}
		m.mu.Unlock()
	}()
	worker, err := m.workerFor(ctx, sessionID, sessionPath)
	if err != nil {
		return err
	}
	return worker.Prompt(ctx, chat)
}

// Snapshot returns a point-in-time view of every live worker, for the metrics
// dashboard. It does not spawn workers and never blocks on the workers
// themselves beyond reading their cached status.
func (m *Manager) Snapshot() []WorkerSnapshot {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]WorkerSnapshot, 0, len(m.workers))
	for id, w := range m.workers {
		st := w.Status()
		snap := WorkerSnapshot{
			SessionID: id,
			State:     st.State,
			Model:     st.Model,
			Error:     st.Error,
			CheckedAt: st.CheckedAt,
		}
		if ins, ok := w.(inspector); ok {
			snap.PID = ins.PID()
			if started := ins.StartedAt(); !started.IsZero() {
				snap.UptimeS = now.Sub(started).Seconds()
			}
			snap.IdleForS = ins.IdleSince(now).Seconds()
		}
		out = append(out, snap)
	}
	return out
}

func (m *Manager) Status(sessionID string) WorkerStatus {
	m.mu.Lock()
	pending := m.pendingSends[sessionID] > 0 || m.operations[sessionID] > 0
	worker := m.workers[sessionID]
	m.mu.Unlock()
	if worker == nil {
		if pending {
			return WorkerStatus{State: WorkerStateRunning}
		}
		return WorkerStatus{State: WorkerStateAbsent}
	}
	status := worker.Status()
	if pending && status.State == WorkerStateIdle {
		status.State = WorkerStateRunning
	}
	return status
}

func (m *Manager) SetModel(ctx context.Context, sessionID, sessionPath, provider, modelID string) error {
	m.beginOperation(sessionID)
	defer m.endOperation(sessionID)
	worker, err := m.workerFor(ctx, sessionID, sessionPath)
	if err != nil {
		return err
	}
	return worker.SetModel(ctx, provider, modelID)
}

func (m *Manager) SetThinkingLevel(ctx context.Context, sessionID, sessionPath, level string) error {
	m.beginOperation(sessionID)
	defer m.endOperation(sessionID)
	worker, err := m.workerFor(ctx, sessionID, sessionPath)
	if err != nil {
		return err
	}
	return worker.SetThinkingLevel(ctx, level)
}

func (m *Manager) GetState(ctx context.Context, sessionID string) (WorkerStatus, error) {
	m.mu.Lock()
	worker := m.workers[sessionID]
	m.mu.Unlock()
	if worker == nil {
		return WorkerStatus{State: WorkerStateAbsent}, nil
	}
	return worker.GetState(ctx)
}

// GetCommands peeks at an existing worker for the session and returns its
// slash commands. It never spawns a worker: if none exists yet it returns
// ready=false so callers can decide whether to spawn (via EnsureWorker) before
// retrying.
func (m *Manager) GetCommands(ctx context.Context, sessionID string) (cmds []SlashCommand, ready bool, err error) {
	m.mu.Lock()
	worker := m.workers[sessionID]
	m.mu.Unlock()
	if worker == nil {
		return nil, false, nil
	}
	cmds, err = worker.GetCommands(ctx)
	return cmds, true, err
}

func (m *Manager) Abort(ctx context.Context, sessionID string) error {
	m.beginOperation(sessionID)
	defer m.endOperation(sessionID)
	m.mu.Lock()
	worker := m.workers[sessionID]
	m.mu.Unlock()
	if worker == nil {
		return nil
	}
	return worker.Abort(ctx)
}

func (m *Manager) EnsureWorker(ctx context.Context, sessionID, sessionPath string) error {
	m.beginOperation(sessionID)
	defer m.endOperation(sessionID)
	_, err := m.workerFor(ctx, sessionID, sessionPath)
	return err
}

func (m *Manager) Close() error {
	select {
	case <-m.reaperStop:
		// already closed
	default:
		close(m.reaperStop)
	}
	<-m.reaperDone

	m.mu.Lock()
	workers := make([]ChatWorker, 0, len(m.workers))
	for _, worker := range m.workers {
		workers = append(workers, worker)
	}
	m.workers = make(map[string]ChatWorker)
	m.mu.Unlock()
	var result error
	for _, worker := range workers {
		result = errors.Join(result, worker.Close())
	}
	return result
}

func (m *Manager) workerFor(ctx context.Context, sessionID, sessionPath string) (ChatWorker, error) {
	for {
		m.mu.Lock()
		if done := m.retiring[sessionID]; done != nil {
			m.mu.Unlock()
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if worker := m.workers[sessionID]; worker != nil {
			if worker.Status().State != WorkerStateError {
				m.mu.Unlock()
				return worker, nil
			}
			delete(m.workers, sessionID)
			m.retiring[sessionID] = make(chan struct{})
			m.mu.Unlock()
			m.finishRetirement(sessionID, worker, "worker_error")
			continue
		}
		if call := m.creating[sessionID]; call != nil {
			m.mu.Unlock()
			select {
			case <-call.done:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			if call.err != nil {
				return nil, call.err
			}
			return call.worker, nil
		}
		call := &createCall{done: make(chan struct{})}
		m.creating[sessionID] = call
		m.mu.Unlock()

		worker, err := m.factory(sessionID, sessionPath)

		m.mu.Lock()
		if err == nil {
			if existing := m.workers[sessionID]; existing != nil && existing.Status().State != WorkerStateError {
				_ = worker.Close()
				worker = existing
			} else {
				m.workers[sessionID] = worker
			}
		}
		delete(m.creating, sessionID)
		call.worker = worker
		call.err = err
		close(call.done)
		m.mu.Unlock()
		return worker, err
	}
}

type idleReportable interface {
	IdleSince(now time.Time) time.Duration
}
