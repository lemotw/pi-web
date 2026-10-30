package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"pi-web/internal/workers"
)

func TestWorkerStatusPreservesUnknownErrorAndAbsentWithoutRPC(t *testing.T) {
	for _, state := range []workers.State{workers.WorkerStateUnknown, workers.WorkerStateError, workers.WorkerStateAbsent} {
		t.Run(string(state), func(t *testing.T) {
			sender := &fakeSender{status: workers.WorkerStatus{State: state, Error: "diagnostic", Model: "model"}}
			s := &Server{sessionsDir: t.TempDir(), chatSender: sender, now: time.Now}
			r := httptest.NewRequest(http.MethodGet, "/api/worker-status?id=session.jsonl", nil)
			out := httptest.NewRecorder()
			s.handleWorkerStatus(out, r)
			var got workers.WorkerStatus
			if err := json.Unmarshal(out.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.State != state || got.Error != "diagnostic" {
				t.Fatalf("status=%+v", got)
			}
			if sender.stateCalls() != 0 {
				t.Fatal("HTTP polling performed RPC")
			}
		})
	}
}

func TestUnknownDoesNotBroadcastCompletionOrKickQueue(t *testing.T) {
	s := &Server{sessionsDir: t.TempDir(), chatSender: &fakeSender{status: workers.WorkerStatus{State: workers.WorkerStateUnknown}}, lastKnown: map[string]struct{}{"s": {}}, now: time.Now}
	d := newQueueDrainer(s)
	s.queueDrainer = d
	c := s.addClient(globalSessID)
	defer s.removeClient(c)
	s.recomputeAndBroadcastStatus("s")
	if !s.computeRunningStatus("s") {
		t.Fatal("unknown dropped last known activity")
	}
	select {
	case msg := <-c.ch:
		t.Fatalf("false completion broadcast: %s", msg)
	default:
	}
	select {
	case <-d.kickCh:
		t.Fatal("unknown kicked queue")
	default:
	}
}

func TestTerminalErrorOrAbsenceDoesNotKickCompletionQueue(t *testing.T) {
	for _, state := range []workers.State{workers.WorkerStateError, workers.WorkerStateAbsent} {
		s := &Server{sessionsDir: t.TempDir(), chatSender: &fakeSender{status: workers.WorkerStatus{State: state}}, lastKnown: map[string]struct{}{"s": {}}, now: time.Now}
		d := newQueueDrainer(s)
		s.queueDrainer = d
		s.recomputeAndBroadcastStatus("s")
		select {
		case <-d.kickCh:
			t.Fatalf("%s treated as successful idle", state)
		default:
		}
	}
}

func TestUnknownLeavesQueueItemUntouched(t *testing.T) {
	sender := &fakeSender{status: workers.WorkerStatus{State: workers.WorkerStateUnknown}, sendCh: make(chan struct{}, 1)}
	s, d, id := newDrainerServer(t, sender)
	if _, err := s.chatQueue.Add(id, "keep queued", "keep queued"); err != nil {
		t.Fatal(err)
	}
	d.drainSession(id)
	snapshot, err := s.chatQueue.List(id)
	if err != nil || len(snapshot.Items) != 1 {
		t.Fatalf("queue=%+v err=%v", snapshot, err)
	}
	select {
	case <-sender.sendCh:
		t.Fatal("unknown dispatched a prompt")
	default:
	}
}

func TestConfirmedIdleWithoutModelOverridesRecentMtime(t *testing.T) {
	now := time.Now()
	s := &Server{sessionsDir: t.TempDir(), chatSender: &fakeSender{status: workers.WorkerStatus{State: workers.WorkerStateIdle, CheckedAt: now.UnixMilli()}}, fileMod: map[string]time.Time{"s": now}, now: time.Now}
	if s.computeRunningStatus("s") {
		t.Fatal("mtime overrode confirmed idle")
	}
}
