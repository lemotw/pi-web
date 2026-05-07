package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type WorkerState string

const (
	WorkerStateIdle    WorkerState = "idle"
	WorkerStateRunning WorkerState = "running"
	WorkerStateError   WorkerState = "error"
)

type WorkerStatus struct {
	State         WorkerState `json:"state"`
	Error         string      `json:"error,omitempty"`
	Model         string      `json:"model,omitempty"`
	ModelName     string      `json:"modelName,omitempty"`
	ModelProvider string      `json:"modelProvider,omitempty"`
	ThinkingLevel string      `json:"thinkingLevel,omitempty"`
}

type ChatWorker interface {
	Prompt(ctx context.Context, chat ChatRequest) error
	SetModel(ctx context.Context, provider, modelID string) error
	SetThinkingLevel(ctx context.Context, level string) error
	GetState(ctx context.Context) (WorkerStatus, error)
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

func (m *WorkerManager) SetModel(ctx context.Context, sessionID, sessionPath, provider, modelID string) error {
	worker, err := m.workerFor(sessionID, sessionPath)
	if err != nil {
		return err
	}
	return worker.SetModel(ctx, provider, modelID)
}

func (m *WorkerManager) SetThinkingLevel(ctx context.Context, sessionID, sessionPath, level string) error {
	worker, err := m.workerFor(sessionID, sessionPath)
	if err != nil {
		return err
	}
	return worker.SetThinkingLevel(ctx, level)
}

func (m *WorkerManager) GetState(ctx context.Context, sessionID string) (WorkerStatus, error) {
	m.mu.Lock()
	worker := m.workers[sessionID]
	m.mu.Unlock()
	if worker == nil {
		return WorkerStatus{State: WorkerStateIdle}, nil
	}
	return worker.GetState(ctx)
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
	if worker := m.workers[sessionID]; worker != nil {
		if worker.Status().State != WorkerStateError {
			m.mu.Unlock()
			return worker, nil
		}
		// Worker is in error state — evict and recreate so callers don't get a dead process.
		delete(m.workers, sessionID)
		m.mu.Unlock()
		_ = worker.Close()
	} else {
		m.mu.Unlock()
	}

	worker, err := m.factory(sessionPath)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if existing := m.workers[sessionID]; existing != nil && existing.Status().State != WorkerStateError {
		_ = worker.Close()
		return existing, nil
	}
	m.workers[sessionID] = worker
	return worker, nil
}

type piRPCWorker struct {
	mu                   sync.Mutex
	writeMu              sync.Mutex
	sessionPath          string
	cmd                  *exec.Cmd
	stdin                io.WriteCloser
	status               WorkerStatus
	seq                  atomic.Uint64
	pending              map[string]chan rpcResponse
	currentModel         string
	currentProvider      string
	currentThinkingLevel string
	stderrBuf            *strings.Builder
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
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf
	worker := &piRPCWorker{
		sessionPath: sessionPath,
		cmd:         cmd,
		stdin:       stdin,
		status:      WorkerStatus{State: WorkerStateIdle},
		pending:     make(map[string]chan rpcResponse),
		stderrBuf:   &stderrBuf,
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go worker.consume(stdout)
	go worker.wait()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := worker.switchSession(ctx); err != nil {
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
	id := w.nextID()
	if err := w.sendAndAwait(ctx, buildPromptCommand(id, chat, streaming)); err != nil {
		w.mu.Lock()
		w.status = WorkerStatus{State: WorkerStateError, Error: err.Error()}
		w.mu.Unlock()
		return err
	}
	return nil
}

func (w *piRPCWorker) SetModel(ctx context.Context, provider, modelID string) error {
	id := w.nextID()
	ch := make(chan rpcResponse, 1)
	w.mu.Lock()
	w.pending[id] = ch
	w.mu.Unlock()

	cmd := map[string]any{
		"id":       id,
		"type":     "set_model",
		"provider": provider,
		"modelId":  modelID,
	}
	w.writeMu.Lock()
	err := writeRPCCommand(w.stdin, cmd)
	w.writeMu.Unlock()
	if err != nil {
		w.removePending(id)
		return err
	}

	select {
	case res := <-ch:
		if !res.Success {
			if res.Error != "" {
				return errors.New(res.Error)
			}
			return fmt.Errorf("rpc set_model rejected")
		}
		var m rpcModel
		if err := json.Unmarshal(res.Data, &m); err != nil {
			// Some responses wrap the model in a "model" field
			var wrapper struct {
				Model rpcModel `json:"model"`
			}
			if err2 := json.Unmarshal(res.Data, &wrapper); err2 == nil && wrapper.Model.ID != "" {
				m = wrapper.Model
			}
		}
		if m.ID == "" {
			return fmt.Errorf("set_model returned empty model id")
		}
		if m.Provider != provider || m.ID != modelID {
			return fmt.Errorf("set_model returned unexpected model: %s/%s (wanted %s/%s)", m.Provider, m.ID, provider, modelID)
		}
		w.mu.Lock()
		w.currentModel = m.ID
		w.currentProvider = m.Provider
		w.status = WorkerStatus{State: WorkerStateIdle, Model: m.ID, ModelName: m.Name, ModelProvider: m.Provider}
		w.mu.Unlock()
		// Refresh thinking level after model switch
		go w.refreshThinkingLevel()
		return nil
	case <-ctx.Done():
		w.removePending(id)
		return ctx.Err()
	}
}

func (w *piRPCWorker) SetThinkingLevel(ctx context.Context, level string) error {
	return w.sendAndAwait(ctx, buildSetThinkingLevelCommand(w.nextID(), level))
}

func (w *piRPCWorker) GetState(ctx context.Context) (WorkerStatus, error) {
	id := w.nextID()
	ch := make(chan rpcResponse, 1)
	w.mu.Lock()
	w.pending[id] = ch
	w.mu.Unlock()

	w.writeMu.Lock()
	err := writeRPCCommand(w.stdin, buildGetStateCommand(id))
	w.writeMu.Unlock()
	if err != nil {
		w.removePending(id)
		return WorkerStatus{}, err
	}

	select {
	case res := <-ch:
		if !res.Success {
			if res.Error != "" {
				return WorkerStatus{}, errors.New(res.Error)
			}
			return WorkerStatus{}, fmt.Errorf("rpc get_state rejected")
		}
		var state struct {
			ThinkingLevel string `json:"thinkingLevel"`
		}
		_ = json.Unmarshal(res.Data, &state)
		w.mu.Lock()
		w.currentThinkingLevel = state.ThinkingLevel
		s := w.status
		s.ThinkingLevel = state.ThinkingLevel
		w.mu.Unlock()
		return s, nil
	case <-ctx.Done():
		w.removePending(id)
		return WorkerStatus{}, ctx.Err()
	}
}

func (w *piRPCWorker) refreshThinkingLevel() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _ = w.GetState(ctx)
}

func (w *piRPCWorker) Status() WorkerStatus {
	w.mu.Lock()
	defer w.mu.Unlock()
	s := w.status
	s.Model = w.currentModel
	s.ModelProvider = w.currentProvider
	s.ThinkingLevel = w.currentThinkingLevel
	return s
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
	return w.sendAndAwait(ctx, buildSwitchSessionCommand(w.nextID(), w.sessionPath))
}

func (w *piRPCWorker) sendAndAwait(ctx context.Context, cmd map[string]any) error {
	id, _ := cmd["id"].(string)
	if id == "" {
		return errors.New("rpc command missing id")
	}
	ch := make(chan rpcResponse, 1)
	w.mu.Lock()
	w.pending[id] = ch
	w.mu.Unlock()

	w.writeMu.Lock()
	err := writeRPCCommand(w.stdin, cmd)
	w.writeMu.Unlock()
	if err != nil {
		w.removePending(id)
		return err
	}

	select {
	case res := <-ch:
		if !res.Success {
			if res.Error != "" {
				return errors.New(res.Error)
			}
			return fmt.Errorf("rpc %s rejected", res.Command)
		}
		return nil
	case <-ctx.Done():
		w.removePending(id)
		return ctx.Err()
	}
}

func (w *piRPCWorker) removePending(id string) {
	w.mu.Lock()
	delete(w.pending, id)
	w.mu.Unlock()
}

func (w *piRPCWorker) consume(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		w.handleRPCLine(line)
	}
	if err := scanner.Err(); err != nil {
		w.setError(err)
	}
}

func (w *piRPCWorker) handleRPCLine(line string) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return
	}
	if raw["type"] == "response" {
		var res rpcResponse
		if err := json.Unmarshal([]byte(line), &res); err != nil {
			return
		}
		w.mu.Lock()
		ch := w.pending[res.ID]
		delete(w.pending, res.ID)
		w.mu.Unlock()
		if ch != nil {
			ch <- res
		}
		return
	}
	if raw["type"] == "agent_end" {
		w.mu.Lock()
		w.status = WorkerStatus{State: WorkerStateIdle}
		w.mu.Unlock()
	}
	if raw["type"] == "thinking_level_changed" {
		if level, ok := raw["level"].(string); ok && level != "" {
			w.mu.Lock()
			w.currentThinkingLevel = level
			w.mu.Unlock()
		}
	}
}

func (w *piRPCWorker) wait() {
	if err := w.cmd.Wait(); err != nil {
		w.setError(err)
	}
}

func (w *piRPCWorker) setError(err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.status.State != WorkerStateError {
		w.status = WorkerStatus{State: WorkerStateError, Error: err.Error()}
	}
	for id, ch := range w.pending {
		delete(w.pending, id)
		ch <- rpcResponse{ID: id, Type: "response", Success: false, Error: err.Error()}
	}
}
