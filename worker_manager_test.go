package main

import (
	"context"
	"sync"
	"testing"
)

type fakeChatWorker struct {
	mu        sync.Mutex
	streaming bool
	prompts   []map[string]any
}

func (f *fakeChatWorker) Prompt(ctx context.Context, chat ChatRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cmd := buildPromptCommand("test", chat, f.streaming)
	f.prompts = append(f.prompts, cmd)
	f.streaming = true
	return nil
}

func (f *fakeChatWorker) Status() WorkerStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.streaming {
		return WorkerStatus{State: WorkerStateRunning}
	}
	return WorkerStatus{State: WorkerStateIdle}
}

func (f *fakeChatWorker) Close() error { return nil }

func TestWorkerManagerCreatesOneWorkerPerSession(t *testing.T) {
	created := 0
	manager := NewWorkerManager(func(sessionPath string) (ChatWorker, error) {
		created++
		return &fakeChatWorker{}, nil
	})
	ctx := context.Background()
	if err := manager.Send(ctx, "a.jsonl", "/tmp/a.jsonl", ChatRequest{Message: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Send(ctx, "b.jsonl", "/tmp/b.jsonl", ChatRequest{Message: "b"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Send(ctx, "a.jsonl", "/tmp/a.jsonl", ChatRequest{Message: "again"}); err != nil {
		t.Fatal(err)
	}
	if created != 2 {
		t.Fatalf("created workers = %d, want 2", created)
	}
}

func TestWorkerManagerReportsMissingWorkerIdle(t *testing.T) {
	manager := NewWorkerManager(func(sessionPath string) (ChatWorker, error) { return &fakeChatWorker{}, nil })
	status := manager.Status("missing.jsonl")
	if status.State != WorkerStateIdle {
		t.Fatalf("status = %q, want idle", status.State)
	}
}

func TestBusyWorkerUsesSteeringCommand(t *testing.T) {
	worker := &fakeChatWorker{streaming: true}
	manager := NewWorkerManager(func(sessionPath string) (ChatWorker, error) { return worker, nil })
	if err := manager.Send(context.Background(), "a.jsonl", "/tmp/a.jsonl", ChatRequest{Message: "steer"}); err != nil {
		t.Fatal(err)
	}
	worker.mu.Lock()
	defer worker.mu.Unlock()
	if len(worker.prompts) != 1 {
		t.Fatalf("prompts = %d, want 1", len(worker.prompts))
	}
	if worker.prompts[0]["streamingBehavior"] != "steer" {
		t.Fatalf("streamingBehavior = %v, want steer", worker.prompts[0]["streamingBehavior"])
	}
}
