package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"pi-web/internal/chat"
	"pi-web/internal/workers"
)

type stateTestPipe struct{ commands chan map[string]any }

func (p *stateTestPipe) Write(b []byte) (int, error) {
	var cmd map[string]any
	if err := json.Unmarshal(b, &cmd); err != nil {
		return 0, err
	}
	p.commands <- cmd
	return len(b), nil
}
func (*stateTestPipe) Close() error { return nil }

func newStateTestWorker(t *testing.T) (*piRPCWorker, *stateTestPipe) {
	t.Helper()
	p := &stateTestPipe{commands: make(chan map[string]any, 16)}
	w := &piRPCWorker{stdin: p, status: workers.WorkerStatus{State: workers.WorkerStateUnknown}, pending: make(map[string]chan response), done: make(chan struct{})}
	t.Cleanup(func() { _ = w.Close() })
	return w, p
}
func stateCommand(t *testing.T, p *stateTestPipe) map[string]any {
	t.Helper()
	select {
	case cmd := <-p.commands:
		return cmd
	case <-time.After(3 * time.Second):
		t.Fatal("missing RPC command")
		return nil
	}
}
func replyState(w *piRPCWorker, cmd map[string]any, data string) {
	w.handleRPCLine(`{"type":"response","id":"` + cmd["id"].(string) + `","success":true,"data":` + data + `}`)
}

type stateResult struct {
	state workers.WorkerStatus
	err   error
}

func startStateCheck(w *piRPCWorker) <-chan stateResult {
	done := make(chan stateResult, 1)
	go func() { state, err := w.GetState(context.Background()); done <- stateResult{state, err} }()
	return done
}
func checkedState(t *testing.T, w *piRPCWorker, p *stateTestPipe, data string) stateResult {
	t.Helper()
	done := startStateCheck(w)
	cmd := stateCommand(t, p)
	if cmd["type"] != "get_state" {
		t.Fatalf("command=%v", cmd)
	}
	replyState(w, cmd, data)
	select {
	case result := <-done:
		return result
	case <-time.After(3 * time.Second):
		t.Fatal("state check did not finish")
		return stateResult{}
	}
}

func TestRetryDoesNotBecomeIdle(t *testing.T) {
	w := &piRPCWorker{status: workers.WorkerStatus{State: workers.WorkerStateRunning}, pending: make(map[string]chan response)}
	for _, event := range []string{
		`{"type":"agent_end","willRetry":true}`, `{"type":"auto_retry_start","attempt":1}`, `{"type":"agent_start"}`,
		`{"type":"message_update","assistantMessageEvent":{"type":"text_delta","delta":"working"}}`,
		`{"type":"turn_end"}`, `{"type":"compaction_start"}`, `{"type":"compaction_end"}`, `{"type":"agent_settled"}`,
	} {
		w.handleRPCLine(event)
		if got := w.Status().State; got != workers.WorkerStateRunning {
			t.Fatalf("after %s: %s, want running until confirmation", event, got)
		}
		if idle := w.IdleSince(time.Now().Add(time.Hour)); idle != 0 {
			t.Fatalf("active worker idle duration=%v", idle)
		}
	}
}

func TestGetStateReadsBusyFlags(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		want       workers.State
	}{
		{"streaming", `{"isStreaming":true,"isCompacting":false}`, workers.WorkerStateRunning},
		{"compacting", `{"isStreaming":false,"isCompacting":true}`, workers.WorkerStateRunning},
		{"idle", `{"isStreaming":false,"isCompacting":false}`, workers.WorkerStateIdle},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, p := newStateTestWorker(t)
			result := checkedState(t, w, p, tc.data)
			if result.err != nil || result.state.State != tc.want || result.state.CheckedAt == 0 {
				t.Fatalf("result=%+v", result)
			}
		})
	}
}

func TestStateConfirmationRejectsMissingOrInvalidFlags(t *testing.T) {
	for _, data := range []string{`{}`, `null`, `{"isStreaming":false}`, `{"isStreaming":null,"isCompacting":false}`, `{"isStreaming":"false","isCompacting":false}`} {
		t.Run(data, func(t *testing.T) {
			w, p := newStateTestWorker(t)
			w.noteBusy(true)
			result := checkedState(t, w, p, data)
			if result.err == nil || result.state.State != workers.WorkerStateUnknown || result.state.CheckedAt != 0 {
				t.Fatalf("result=%+v", result)
			}
			if w.ClaimIdleClose(time.Nanosecond) {
				t.Fatal("unknown worker can be reaped")
			}
		})
	}
}

func TestOldStateReplyCannotOverrideNewActivity(t *testing.T) {
	for _, mutation := range []bool{false, true} {
		t.Run(map[bool]string{false: "agent event", true: "new command"}[mutation], func(t *testing.T) {
			w, p := newStateTestWorker(t)
			done := startStateCheck(w)
			cmd := stateCommand(t, p)
			if mutation {
				if err := w.beginMutation(true); err != nil {
					t.Fatal(err)
				}
				defer w.endMutation()
			} else {
				w.handleRPCLine(`{"type":"agent_start"}`)
			}
			replyState(w, cmd, `{"isStreaming":false,"isCompacting":false}`)
			result := <-done
			if !errors.Is(result.err, errStateChanged) || w.Status().State != workers.WorkerStateRunning {
				t.Fatalf("stale result=%+v, cached=%+v", result, w.Status())
			}
		})
	}
}

func TestIdleCountdownStartsAtConfirmationAndPollingDoesNotResetIt(t *testing.T) {
	w, p := newStateTestWorker(t)
	w.noteBusy(true)
	if got := w.IdleSince(time.Now().Add(20 * time.Minute)); got != 0 {
		t.Fatalf("running idle=%v", got)
	}
	idle := `{"isStreaming":false,"isCompacting":false}`
	if r := checkedState(t, w, p, idle); r.err != nil {
		t.Fatal(r.err)
	}
	w.mu.Lock()
	since := time.Now().Add(-11 * time.Minute)
	w.idleSince = since
	w.mu.Unlock()
	if r := checkedState(t, w, p, idle); r.err != nil {
		t.Fatal(r.err)
	}
	w.mu.Lock()
	after := w.idleSince
	w.mu.Unlock()
	if !after.Equal(since) {
		t.Fatal("passive query reset idle countdown")
	}
	w.noteBusy(true)
	if r := checkedState(t, w, p, idle); r.err != nil {
		t.Fatal(r.err)
	}
	if got := w.IdleSince(time.Now()); got > time.Second {
		t.Fatalf("new idle retained old timestamp: %v", got)
	}
}

func TestConcurrentStateChecksShareQueryAndCallerCancellation(t *testing.T) {
	w, p := newStateTestWorker(t)
	first := startStateCheck(w)
	cmd := stateCommand(t, p)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := w.GetState(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error=%v", err)
	}
	if len(p.commands) != 0 {
		t.Fatal("second caller started another RPC")
	}
	replyState(w, cmd, `{"isStreaming":false,"isCompacting":false}`)
	if r := <-first; r.err != nil || r.state.State != workers.WorkerStateIdle {
		t.Fatalf("shared check canceled: %+v", r)
	}
}

func TestStateCheckTimeoutIsUnknown(t *testing.T) {
	w, p := newStateTestWorker(t)
	done := startStateCheck(w)
	stateCommand(t, p)
	result := <-done
	if !errors.Is(result.err, context.DeadlineExceeded) || result.state.State != workers.WorkerStateUnknown {
		t.Fatalf("result=%+v", result)
	}
	if w.IdleSince(time.Now().Add(time.Hour)) != 0 {
		t.Fatal("timeout started idle countdown")
	}
}

func TestExitFailsPendingConfirmationWithoutChangingErrorToIdle(t *testing.T) {
	w, p := newStateTestWorker(t)
	done := startStateCheck(w)
	stateCommand(t, p)
	w.setError(io.ErrUnexpectedEOF)
	select {
	case r := <-done:
		if r.err == nil || w.Status().State != workers.WorkerStateError {
			t.Fatalf("result=%+v", r)
		}
	case <-time.After(time.Second):
		t.Fatal("exit stranded query")
	}
}

func TestClaimIdleCloseRejectsNewActivityAndFencesCommands(t *testing.T) {
	w, p := newStateTestWorker(t)
	checkedState(t, w, p, `{"isStreaming":false,"isCompacting":false}`)
	w.mu.Lock()
	w.idleSince = time.Now().Add(-time.Hour)
	w.mu.Unlock()
	w.noteBusy(true)
	if w.ClaimIdleClose(time.Minute) {
		t.Fatal("reaped new run")
	}
	checkedState(t, w, p, `{"isStreaming":false,"isCompacting":false}`)
	w.mu.Lock()
	w.idleSince = time.Now().Add(-time.Hour)
	w.mu.Unlock()
	if !w.ClaimIdleClose(time.Minute) {
		t.Fatal("could not claim idle worker")
	}
	if w.beginMutation(true) == nil {
		t.Fatal("command entered retired worker")
	}
}

func TestConcurrentPromptsReserveRunningBeforeWriting(t *testing.T) {
	w, p := newStateTestWorker(t)
	checkedState(t, w, p, `{"isStreaming":false,"isCompacting":false}`)
	first := make(chan error, 1)
	go func() { first <- w.Prompt(context.Background(), chat.Request{Message: "first"}) }()
	c1 := stateCommand(t, p)
	second := make(chan error, 1)
	go func() { second <- w.Prompt(context.Background(), chat.Request{Message: "second"}) }()
	c2 := stateCommand(t, p)
	if c1["streamingBehavior"] != nil || c2["streamingBehavior"] != "steer" {
		t.Fatalf("commands=%v %v", c1, c2)
	}
	replyState(w, c1, `{}`)
	replyState(w, c2, `{}`)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := <-second; err != nil {
		t.Fatal(err)
	}
	if w.Status().State != workers.WorkerStateRunning {
		t.Fatal("ack ended active run")
	}
}

func TestAbortAcknowledgementStillRequiresIdleConfirmation(t *testing.T) {
	w, p := newStateTestWorker(t)
	w.noteBusy(true)
	done := make(chan error, 1)
	go func() { done <- w.Abort(context.Background()) }()
	replyState(w, stateCommand(t, p), `{}`)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if w.Status().State != workers.WorkerStateRunning {
		t.Fatal("abort ack committed idle")
	}
	if r := checkedState(t, w, p, `{"isStreaming":false,"isCompacting":false}`); r.state.State != workers.WorkerStateIdle {
		t.Fatalf("state=%+v", r)
	}
}

func TestSetModelDoesNotEndActiveRun(t *testing.T) {
	w, p := newStateTestWorker(t)
	w.noteBusy(true)
	done := make(chan error, 1)
	go func() { done <- w.SetModel(context.Background(), "provider", "new-model") }()
	replyState(w, stateCommand(t, p), `{"id":"new-model","provider":"provider","name":"New"}`)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if w.Status().State != workers.WorkerStateRunning {
		t.Fatal("model switch committed idle")
	}
	replyState(w, stateCommand(t, p), `{"isStreaming":true,"isCompacting":false}`)
}

func TestVerifierConfirmsSettledWithoutBrowserPolling(t *testing.T) {
	w, p := newStateTestWorker(t)
	w.verifyCh = make(chan struct{}, 1)
	w.noteBusy(true)
	stopped := make(chan struct{})
	go func() { w.verifyStateLoop(); close(stopped) }()
	w.handleRPCLine(`{"type":"agent_settled"}`)
	replyState(w, stateCommand(t, p), `{"isStreaming":false,"isCompacting":false}`)
	deadline := time.Now().Add(time.Second)
	for w.Status().State != workers.WorkerStateIdle && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if w.Status().State != workers.WorkerStateIdle {
		t.Fatal("background confirmation did not commit idle")
	}
	w.Close()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("verifier survived worker close")
	}
}

func TestWriteCommandDeadlineIncludesBlockedPipe(t *testing.T) {
	r, p, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer p.Close()
	w := &piRPCWorker{stdin: p}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	if err := w.writeCommand(ctx, map[string]any{"type": "prompt", "message": strings.Repeat("x", 1<<20)}); err == nil {
		t.Fatal("expected blocked write timeout")
	}
	if time.Since(started) > time.Second {
		t.Fatal("write exceeded deadline")
	}
}
