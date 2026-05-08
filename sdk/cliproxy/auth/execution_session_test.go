package auth

import (
	"context"
	"sync"
	"testing"
	"time"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/executor"
	coreusage "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/usage"
)


type recordingExecutionSessionLedger struct {
	mu         sync.Mutex
	began      []ExecutionSessionRecord
	finalized  []string
	finishTime []time.Time
}

func (r *recordingExecutionSessionLedger) BeginExecutionSession(_ context.Context, record ExecutionSessionRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.began = append(r.began, record)
	return nil
}

func (r *recordingExecutionSessionLedger) FinalizeExecutionSession(_ context.Context, sessionID string, finishedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finalized = append(r.finalized, sessionID)
	r.finishTime = append(r.finishTime, finishedAt)
	return nil
}

func (r *recordingExecutionSessionLedger) GetExecutionQuotaSummary(context.Context, string) (ExecutionQuotaSnapshot, error) {
	return ExecutionQuotaSnapshot{}, nil
}

// HandleUsage is required by the ExecutionSessionLedger interface.
func (r *recordingExecutionSessionLedger) HandleUsage(context.Context, coreusage.Record) {}

func TestCloseExecutionSessionFinalizesLedger(t *testing.T) {
	manager := NewManager(nil, nil, nil)
	ledger := &recordingExecutionSessionLedger{}
	manager.SetExecutionSessionLedger(ledger)

	manager.CloseExecutionSession("session-1")

	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if len(ledger.finalized) != 1 {
		t.Fatalf("FinalizeExecutionSession call count = %d, want 1", len(ledger.finalized))
	}
	if ledger.finalized[0] != "session-1" {
		t.Fatalf("FinalizeExecutionSession sessionID = %q, want %q", ledger.finalized[0], "session-1")
	}
}

func TestCloseExecutionSessionIgnoresCloseAllMarker(t *testing.T) {
	manager := NewManager(nil, nil, nil)
	ledger := &recordingExecutionSessionLedger{}
	manager.SetExecutionSessionLedger(ledger)

	manager.CloseExecutionSession(CloseAllExecutionSessionsID)

	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if len(ledger.finalized) != 0 {
		t.Fatalf("FinalizeExecutionSession call count = %d, want 0", len(ledger.finalized))
	}
}

func TestSessionIDFromMetadata(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]any
		want     string
	}{
		{name: "string", metadata: map[string]any{cliproxyexecutor.ExecutionSessionMetadataKey: " session-1 "}, want: "session-1"},
		{name: "bytes", metadata: map[string]any{cliproxyexecutor.ExecutionSessionMetadataKey: []byte("session-2")}, want: "session-2"},
		{name: "missing", metadata: map[string]any{}, want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := sessionIDFromMetadata(tc.metadata); got != tc.want {
				t.Fatalf("sessionIDFromMetadata() = %q, want %q", got, tc.want)
			}
		})
	}
}
