package workers

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"pi-web/internal/chat"
)

type confirmingWorker struct {
	fakeChatWorker
	probe        func(context.Context) (WorkerStatus, error)
	closed       atomic.Bool
	closeStarted chan struct{}
	closeRelease chan struct{}
}

func (*confirmingWorker) IdleSince(time.Time) time.Duration                    { return time.Hour }
func (w *confirmingWorker) GetState(ctx context.Context) (WorkerStatus, error) { return w.probe(ctx) }
func (w *confirmingWorker) Close() error {
	w.closed.Store(true)
	if w.closeStarted != nil {
		close(w.closeStarted)
		<-w.closeRelease
	}
	return nil
}

func TestReaperRequiresSuccessfulIdleConfirmation(t *testing.T) {
	for _, tc := range []struct {
		name       string
		state      State
		err        error
		wantClosed bool
	}{
		{"idle", WorkerStateIdle, nil, true},
		{"busy", WorkerStateRunning, nil, false},
		{"unknown", WorkerStateUnknown, nil, false},
		{"failure", WorkerStateIdle, errors.New("probe failed"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := &confirmingWorker{probe: func(context.Context) (WorkerStatus, error) { return WorkerStatus{State: tc.state}, tc.err }}
			m := NewManagerWithTTL(func(string, string) (ChatWorker, error) { return w, nil }, time.Minute)
			defer m.Close()
			if err := m.EnsureWorker(context.Background(), "s", "unused"); err != nil {
				t.Fatal(err)
			}
			m.reapOnce(time.Now())
			if w.closed.Load() != tc.wantClosed {
				t.Fatalf("closed=%v", w.closed.Load())
			}
			if (len(m.Snapshot()) == 0) != tc.wantClosed {
				t.Fatal("snapshot disagrees with retirement")
			}
		})
	}
}

func TestReaperDoesNotHoldManagerLockAndRejectsOldReplyAfterSend(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	w := &confirmingWorker{probe: func(context.Context) (WorkerStatus, error) {
		close(started)
		<-release
		return WorkerStatus{State: WorkerStateIdle}, nil
	}}
	m := NewManagerWithTTL(func(string, string) (ChatWorker, error) { return w, nil }, time.Minute)
	defer m.Close()
	if err := m.EnsureWorker(context.Background(), "s", "unused"); err != nil {
		t.Fatal(err)
	}
	reaped := make(chan struct{})
	go func() { m.reapOnce(time.Now()); close(reaped) }()
	<-started
	sent := make(chan error, 1)
	go func() { sent <- m.Send(context.Background(), "s", "unused", chat.Request{Message: "new run"}) }()
	select {
	case err := <-sent:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		close(release)
		t.Fatal("RPC held manager mutex")
	}
	close(release)
	<-reaped
	if w.closed.Load() || m.Status("s").State != WorkerStateRunning {
		t.Fatal("old idle reply retired new run")
	}
}

func TestReplacementWaitsUntilOldWorkerCloseCompletes(t *testing.T) {
	w := &confirmingWorker{probe: func(context.Context) (WorkerStatus, error) { return WorkerStatus{State: WorkerStateIdle}, nil }, closeStarted: make(chan struct{}), closeRelease: make(chan struct{})}
	var creates atomic.Int32
	replacement := make(chan struct{}, 1)
	m := NewManagerWithTTL(func(string, string) (ChatWorker, error) {
		if creates.Add(1) == 1 {
			return w, nil
		}
		replacement <- struct{}{}
		return &fakeChatWorker{}, nil
	}, time.Minute)
	defer m.Close()
	m.EnsureWorker(context.Background(), "s", "unused")
	reaped := make(chan struct{})
	go func() { m.reapOnce(time.Now()); close(reaped) }()
	<-w.closeStarted
	sent := make(chan error, 1)
	go func() { sent <- m.Send(context.Background(), "s", "unused", chat.Request{Message: "next"}) }()
	select {
	case <-replacement:
		close(w.closeRelease)
		t.Fatal("replacement overlapped old worker")
	case <-time.After(40 * time.Millisecond):
	}
	close(w.closeRelease)
	<-reaped
	select {
	case err := <-sent:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("send stayed blocked after retirement")
	}
	if creates.Load() != 2 {
		t.Fatalf("created=%d", creates.Load())
	}
}
