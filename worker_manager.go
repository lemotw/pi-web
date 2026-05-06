package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
)

type WorkerState string

const (
	WorkerStateIdle    WorkerState = "idle"
	WorkerStateRunning WorkerState = "running"
	WorkerStateError   WorkerState = "error"
)

type WorkerStatus struct {
	State WorkerState `json:"state"`
	Error string      `json:"error,omitempty"`
}

type ChatWorker interface {
	Prompt(ctx context.Context, chat ChatRequest) error
	Status() WorkerStatus
	Close() error
}

type WorkerFactory func(sessionPath string) (ChatWorker, error)

type WorkerManager struct {
	mu      sync.Mutex
	workers map[string]ChatWorker
	factory WorkerFactory
}

func NewWorkerManager(factory WorkerFactory) *WorkerManager {
	return &WorkerManager{workers: make(map[string]ChatWorker), factory: factory}
}

func (m *WorkerManager) Send(ctx context.Context, sessionID, sessionPath string, chat ChatRequest) error {
	worker, err := m.workerFor(sessionID, sessionPath)
	if err != nil {
		return err
	}
	return worker.Prompt(ctx, chat)
}

func (m *WorkerManager) Status(sessionID string) WorkerStatus {
	m.mu.Lock()
	worker := m.workers[sessionID]
	m.mu.Unlock()
	if worker == nil {
		return WorkerStatus{State: WorkerStateIdle}
	}
	return worker.Status()
}

func (m *WorkerManager) Close() error {
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

func (m *WorkerManager) workerFor(sessionID, sessionPath string) (ChatWorker, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if worker := m.workers[sessionID]; worker != nil {
		return worker, nil
	}
	worker, err := m.factory(sessionPath)
	if err != nil {
		return nil, err
	}
	m.workers[sessionID] = worker
	return worker, nil
}

type piRPCWorker struct {
	mu          sync.Mutex
	sessionPath string
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	status      WorkerStatus
	seq         atomic.Uint64
}

func newPiRPCWorker(sessionPath string) (ChatWorker, error) {
	if _, err := exec.LookPath("pi"); err != nil {
		return nil, fmt.Errorf("pi executable not found: %w", err)
	}
	cmd := exec.Command("pi", "--mode", "rpc")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	worker := &piRPCWorker{sessionPath: sessionPath, cmd: cmd, stdin: stdin, status: WorkerStatus{State: WorkerStateIdle}}
	go worker.consume(stdout)
	if err := worker.switchSession(context.Background()); err != nil {
		_ = worker.Close()
		return nil, err
	}
	return worker, nil
}

func (w *piRPCWorker) Prompt(ctx context.Context, chat ChatRequest) error {
	w.mu.Lock()
	streaming := w.status.State == WorkerStateRunning
	w.status = WorkerStatus{State: WorkerStateRunning}
	w.mu.Unlock()
	return writeRPCCommand(w.stdin, buildPromptCommand(w.nextID(), chat, streaming))
}

func (w *piRPCWorker) Status() WorkerStatus {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.status
}

func (w *piRPCWorker) Close() error {
	if w.stdin != nil {
		_ = w.stdin.Close()
	}
	if w.cmd != nil && w.cmd.Process != nil {
		_ = w.cmd.Process.Kill()
	}
	return nil
}

func (w *piRPCWorker) nextID() string {
	return fmt.Sprintf("req-%d", w.seq.Add(1))
}

func (w *piRPCWorker) switchSession(ctx context.Context) error {
	return writeRPCCommand(w.stdin, buildSwitchSessionCommand(w.nextID(), w.sessionPath))
}

func (w *piRPCWorker) consume(r io.Reader) {
	lines, err := readJSONLLines(r)
	w.mu.Lock()
	defer w.mu.Unlock()
	if err != nil {
		w.status = WorkerStatus{State: WorkerStateError, Error: err.Error()}
		return
	}
	for _, line := range lines {
		var event map[string]any
		if json.Unmarshal([]byte(line), &event) == nil && event["type"] == "agent_end" {
			w.status = WorkerStatus{State: WorkerStateIdle}
		}
	}
}
