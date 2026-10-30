package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"pi-web/internal/workers"
)

const stateCheckTimeout = 2 * time.Second
const stateCheckInterval = 5 * time.Second

var errStateChanged = errors.New("worker changed during state confirmation")

type workerStateQuery struct {
	done       chan struct{}
	generation uint64
	state      workers.WorkerStatus
	err        error
}

func (w *piRPCWorker) setStateLocked(state workers.State, reason string) {
	previous := w.status.State
	w.status.State = state
	w.status.Error = ""
	w.status.CheckedAt = 0
	if state != workers.WorkerStateIdle {
		w.idleSince = time.Time{}
	} else if w.idleSince.IsZero() {
		w.idleSince = time.Now()
	}
	if previous != state {
		fmt.Fprintf(os.Stderr, "%s worker_state session=%q pid=%d from=%s to=%s reason=%s\n", time.Now().UTC().Format(time.RFC3339Nano), filepath.Base(w.sessionPath), w.PID(), previous, state, reason)
	}
}

func (w *piRPCWorker) noteBusy(invalidate bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closing {
		return
	}
	if invalidate || w.status.State != workers.WorkerStateRunning {
		w.generation++
	}
	w.setStateLocked(workers.WorkerStateRunning, "agent_activity")
}

func (w *piRPCWorker) beginMutation(prompt bool) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closing {
		return errors.New("worker is closing or exited")
	}
	w.generation++
	w.mutations++
	if prompt || w.status.State == workers.WorkerStateRunning {
		w.setStateLocked(workers.WorkerStateRunning, "command_started")
	} else {
		w.setStateLocked(workers.WorkerStateUnknown, "command_started")
	}
	return nil
}

func (w *piRPCWorker) endMutation() {
	w.mu.Lock()
	w.mutations--
	w.generation++
	w.mu.Unlock()
	w.requestStateCheck()
}

// ClaimIdleClose is the atomic worker-side retirement fence. The manager also
// reserves all mutating operations and gates replacement until Close finishes.
func (w *piRPCWorker) ClaimIdleClose(ttl time.Duration) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closing || w.mutations != 0 || w.status.State != workers.WorkerStateIdle || w.idleSince.IsZero() || time.Since(w.idleSince) <= ttl {
		return false
	}
	w.closing = true
	w.generation++
	return true
}

func (w *piRPCWorker) requestStateCheck() {
	if w.verifyCh == nil {
		return
	}
	select {
	case w.verifyCh <- struct{}{}:
	default:
	}
}

// Independent of browser subscriptions. Busy or unknown results get a bounded
// low-frequency retry, so a missed settled event cannot strand a worker.
func (w *piRPCWorker) verifyStateLoop() {
	ticker := time.NewTicker(stateCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-w.done:
			return
		case <-w.verifyCh:
		case <-ticker.C:
			if w.Status().State == workers.WorkerStateIdle {
				continue
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), stateCheckTimeout)
		_, _ = w.GetState(ctx)
		cancel()
	}
}

// GetState coalesces concurrent callers. A caller abandoning its HTTP request
// does not cancel the shared check; worker shutdown and the internal deadline do.
func (w *piRPCWorker) GetState(ctx context.Context) (workers.WorkerStatus, error) {
	w.mu.Lock()
	if w.closing {
		s := w.status
		w.mu.Unlock()
		return s, errors.New("worker is closing or exited")
	}
	q := w.stateQuery
	if q == nil {
		q = &workerStateQuery{done: make(chan struct{}), generation: w.generation}
		w.stateQuery = q
		go w.queryState(q)
	}
	w.mu.Unlock()
	select {
	case <-q.done:
		return q.state, q.err
	case <-ctx.Done():
		return w.Status(), ctx.Err()
	}
}

func (w *piRPCWorker) queryState(q *workerStateQuery) {
	ctx, cancel := context.WithTimeout(context.Background(), stateCheckTimeout)
	defer cancel()
	id := w.nextID()
	ch := make(chan response, 1)
	w.mu.Lock()
	w.pending[id] = ch
	w.mu.Unlock()
	defer w.removePending(id)
	err := w.writeCommand(ctx, BuildGetStateCommand(id))
	var state struct {
		IsStreaming   *bool  `json:"isStreaming"`
		IsCompacting  *bool  `json:"isCompacting"`
		ThinkingLevel string `json:"thinkingLevel"`
		Model         model  `json:"model"`
	}
	if err == nil {
		select {
		case res := <-ch:
			if !res.Success {
				err = errors.New("rpc get_state rejected")
			} else if err = json.Unmarshal(res.Data, &state); err == nil && (state.IsStreaming == nil || state.IsCompacting == nil) {
				err = errors.New("rpc get_state missing busy flags")
			}
		case <-ctx.Done():
			err = ctx.Err()
		case <-w.done:
			err = errors.New("worker exited during state confirmation")
		}
	}
	w.mu.Lock()
	if w.closing || q.generation != w.generation || w.mutations != 0 {
		err = errStateChanged
	} else if err != nil {
		w.setStateLocked(workers.WorkerStateUnknown, "state_check_failed")
		w.status.Error = err.Error()
	} else {
		w.currentThinkingLevel = state.ThinkingLevel
		if state.Model.ID != "" {
			w.currentModel = state.Model.ID
			w.currentProvider = state.Model.Provider
			w.status.ModelName = state.Model.Name
		}
		if *state.IsStreaming || *state.IsCompacting {
			w.setStateLocked(workers.WorkerStateRunning, "rpc_busy")
		} else {
			w.setStateLocked(workers.WorkerStateIdle, "rpc_idle_confirmed")
		}
		w.status.CheckedAt = time.Now().UnixMilli()
	}
	q.state = w.status
	q.state.Model = w.currentModel
	q.state.ModelProvider = w.currentProvider
	q.state.ThinkingLevel = w.currentThinkingLevel
	q.err = err
	w.stateQuery = nil
	close(q.done)
	w.mu.Unlock()
}

// Serialize JSONL writes without holding the state mutex. Real subprocess
// pipes support write deadlines; the gate is also cancellable while queued.
func (w *piRPCWorker) writeCommand(ctx context.Context, cmd map[string]any) error {
	w.mu.Lock()
	if w.writeGate == nil {
		w.writeGate = make(chan struct{}, 1)
	}
	gate := w.writeGate
	w.mu.Unlock()
	select {
	case gate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	case <-w.done:
		return errors.New("worker exited")
	}
	defer func() { <-gate }()
	if err := ctx.Err(); err != nil {
		return err
	}
	if w.stdin == nil {
		return errors.New("worker stdin unavailable")
	}
	deadline := time.Now().Add(5 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if pipe, ok := w.stdin.(interface{ SetWriteDeadline(time.Time) error }); ok {
		if err := pipe.SetWriteDeadline(deadline); err != nil {
			return err
		}
		defer pipe.SetWriteDeadline(time.Time{})
	}
	return WriteCommand(w.stdin, cmd)
}
