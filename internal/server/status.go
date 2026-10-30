package server

import (
	"context"
	"encoding/json"

	"pi-web/internal/workers"
)

// computeRunningStatus is the single source of truth for "is this session
// running right now". Both the HTTP handler (handleWorkerStatus) and the SSE
// broadcaster (recomputeAndBroadcastStatus) call this; that is what keeps
// terminal sessions, chat workers, and the recent-activity fallback from
// drifting apart.
//
// Order matches the historical behaviour of handleWorkerStatus:
//  1. session-status/<id> file (terminal sessions)
//  2. in-process chat worker status
//  3. recent jsonl mtime within recentSessionActivityWindow
func (s *Server) computeWorkerStatus(sessionID string) workers.WorkerStatus {
	status := workers.WorkerStatus{State: workers.WorkerStateIdle}
	if sessionID == "" {
		return status
	}
	if s.chatSender != nil {
		status = s.chatSender.Status(sessionID)
	}
	if terminal := s.readSessionStatus(sessionID); terminal != nil {
		status.State = workers.WorkerStateRunning
		return status
	}
	// Never turn a failed/unknown managed-worker state into idle via mtime.
	if status.State == workers.WorkerStateRunning || status.State == workers.WorkerStateUnknown || status.State == workers.WorkerStateError || status.CheckedAt != 0 || status.Model != "" {
		return status
	}
	if s.hasRecentSessionActivity(sessionID) {
		status.State = workers.WorkerStateRunning
	}
	return status
}

func (s *Server) computeRunningStatus(sessionID string) bool {
	status := s.computeWorkerStatus(sessionID)
	if status.State == workers.WorkerStateUnknown {
		s.lastKnownMu.Lock()
		_, running := s.lastKnown[sessionID]
		s.lastKnownMu.Unlock()
		return running
	}
	return status.State == workers.WorkerStateRunning
}

func (s *Server) runningStatusPayload(sessionID string, running bool) map[string]any {
	payload := map[string]any{"id": sessionID, "running": running}
	if s.cache != nil {
		if project, ok := s.cache.ProjectForID(sessionID); ok && project != "" {
			payload["project"] = project
		}
	}
	if !running || s.chatSender == nil {
		return payload
	}
	status := s.chatSender.Status(sessionID)
	if status.Model != "" {
		payload["model"] = status.Model
	}
	if status.ModelName != "" {
		payload["modelName"] = status.ModelName
	}
	if status.ModelProvider != "" {
		payload["modelProvider"] = status.ModelProvider
	}
	return payload
}

// recomputeAndBroadcastStatus recomputes the running state for sessionID and,
// if it changed since the last broadcast, sends a status-delta SSE event to
// every __all__ subscriber.
//
// `lastKnown` is the set of session ids currently broadcast as running.
// Absence == idle. We only emit when (now == running) != (id ∈ lastKnown).
// First-touch idle is therefore silent (no spurious running:false flood when
// the sweeper rescans).
func (s *Server) recomputeAndBroadcastStatus(sessionID string) {
	if sessionID == "" {
		return
	}
	status := s.computeWorkerStatus(sessionID)
	if status.State == workers.WorkerStateUnknown {
		return
	}
	now := status.State == workers.WorkerStateRunning
	confirmedIdle := status.State == workers.WorkerStateIdle

	s.lastKnownMu.Lock()
	_, was := s.lastKnown[sessionID]
	if now == was {
		s.lastKnownMu.Unlock()
		return
	}
	if now {
		s.lastKnown[sessionID] = struct{}{}
	} else {
		delete(s.lastKnown, sessionID)
	}
	s.lastKnownMu.Unlock()

	data, _ := json.Marshal(s.runningStatusPayload(sessionID, now))
	s.broadcast(globalSessID, "event: status-delta\ndata: "+string(data))

	// Transition running → idle: fire a push notification so subscribed
	// clients learn the response is ready even when the tab is closed
	// or the device is locked. Scheduled runs get a schedule-specific push
	// (shown even in the foreground) instead of the generic one.
	if was && confirmedIdle && s.push != nil && !s.disableBackgroundJobs {
		if name, ok := s.scheduleNameForSession(sessionID); ok {
			s.startTask(func(context.Context) {
				s.push.NotifyScheduleDone(name, sessionID)
			})
		} else {
			s.startTask(func(context.Context) {
				s.push.NotifyDone(sessionID)
			})
		}
	}

	// Transition running → idle is also the cue for the autonomous queue
	// drainer: if items are waiting, dispatch the next one now instead of
	// waiting for the 5-second tick.
	if was && confirmedIdle && s.queueDrainer != nil {
		s.queueDrainer.kick(sessionID)
	}
}
