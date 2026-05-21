package agent

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nextlevelbuilder/goclaw/internal/pipeline"
	"github.com/nextlevelbuilder/goclaw/internal/providers"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
	"github.com/nextlevelbuilder/goclaw/pkg/protocol"
)

// stubExecutor implements tools.ToolExecutor with a canned successful Result.
// Used to isolate the tool-callback wrappers from real tool registry wiring.
type stubExecutor struct{}

func (s *stubExecutor) ExecuteWithContext(_ context.Context, _ string, _ map[string]any, _, _, _, _ string, _ tools.AsyncCallback) *tools.Result {
	return &tools.Result{ForLLM: "ok", IsError: false}
}
func (s *stubExecutor) TryActivateDeferred(string) bool          { return false }
func (s *stubExecutor) ProviderDefs() []providers.ToolDefinition { return nil }
func (s *stubExecutor) Get(string) (tools.Tool, bool)            { return nil, false }
func (s *stubExecutor) List() []string                           { return nil }
func (s *stubExecutor) Aliases() map[string]string               { return nil }

type metricRecorder struct {
	mu      sync.Mutex
	metrics []store.EvolutionMetric
}

func (m *metricRecorder) RecordMetric(_ context.Context, metric store.EvolutionMetric) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metrics = append(m.metrics, metric)
	return nil
}

func (m *metricRecorder) QueryMetrics(context.Context, uuid.UUID, store.MetricType, time.Time, int) ([]store.EvolutionMetric, error) {
	return nil, nil
}

func (m *metricRecorder) AggregateToolMetrics(context.Context, uuid.UUID, time.Time) ([]store.ToolAggregate, error) {
	return nil, nil
}

func (m *metricRecorder) AggregateRetrievalMetrics(context.Context, uuid.UUID, time.Time) ([]store.RetrievalAggregate, error) {
	return nil, nil
}

func (m *metricRecorder) Cleanup(context.Context, time.Time) (int64, error) { return 0, nil }

func (m *metricRecorder) latest(t *testing.T) store.EvolutionMetric {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		if len(m.metrics) > 0 {
			metric := m.metrics[len(m.metrics)-1]
			m.mu.Unlock()
			return metric
		}
		m.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for metric")
	return store.EvolutionMetric{}
}

// eventCollector buffers AgentEvents for inspection in tests.
// Safe for concurrent appends from parallel goroutines.
type eventCollector struct {
	mu     sync.Mutex
	events []AgentEvent
}

func (c *eventCollector) onEvent(e AgentEvent) {
	c.mu.Lock()
	c.events = append(c.events, e)
	c.mu.Unlock()
}

func (c *eventCollector) filter(typ string) []AgentEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []AgentEvent
	for _, e := range c.events {
		if e.Type == typ {
			out = append(out, e)
		}
	}
	return out
}

// newTestLoopForToolCallbacks builds a minimal Loop instance sufficient to
// exercise makeExecuteToolCall / makeExecuteToolRaw. All optional subsystems
// (tracing, metrics, input guard) are left nil and hit early-return paths.
func newTestLoopForToolCallbacks(onEvent func(AgentEvent)) *Loop {
	return &Loop{
		id:      "test-agent",
		tools:   &stubExecutor{},
		onEvent: onEvent,
	}
}

// TestMakeExecuteToolCall_EmitsToolCallEvent verifies the sequential wrapper
// emits a tool.call event before running tool I/O.
func TestMakeExecuteToolCall_EmitsToolCallEvent(t *testing.T) {
	t.Parallel()
	col := &eventCollector{}
	l := newTestLoopForToolCallbacks(col.onEvent)

	req := &RunRequest{
		RunID:      "run-1",
		SessionKey: "sess-A",
		UserID:     "u-1",
		SenderID:   "sender-1",
		Channel:    "ws",
		RunKind:    "",
	}
	state := &pipeline.RunState{RunID: "run-1"}
	tc := providers.ToolCall{ID: "tc-1", Name: "read_file", Arguments: map[string]any{"path": "/tmp/x"}}

	_, err := l.makeExecuteToolCall(req, &runState{})(context.Background(), state, tc)
	if err != nil {
		t.Fatalf("makeExecuteToolCall returned error: %v", err)
	}

	calls := col.filter(protocol.AgentEventToolCall)
	if len(calls) != 1 {
		t.Fatalf("expected 1 tool.call event, got %d (all events: %+v)", len(calls), col.events)
	}
	assertToolCallPayload(t, calls[0], tc, req)
}

// TestMakeExecuteToolRaw_EmitsToolCallEvent is the PRIMARY regression guard.
// The original bug: parallel path (makeExecuteToolRaw) did not emit tool.call,
// so web UI and desktop UI silently dropped tool info during real-time streaming.
// Mutation-verify: remove emitRun(...) from makeExecuteToolRaw — this test must fail.
func TestMakeExecuteToolRaw_EmitsToolCallEvent(t *testing.T) {
	t.Parallel()
	col := &eventCollector{}
	l := newTestLoopForToolCallbacks(col.onEvent)

	req := &RunRequest{
		RunID:      "run-2",
		SessionKey: "sess-B",
		UserID:     "u-2",
		SenderID:   "sender-2",
		Channel:    "ws",
		RunKind:    "",
	}
	tc := providers.ToolCall{ID: "tc-2", Name: "write_file", Arguments: map[string]any{"path": "/tmp/y"}}

	msg, raw, err := l.makeExecuteToolRaw(req)(context.Background(), tc)
	if err != nil {
		t.Fatalf("makeExecuteToolRaw returned error: %v", err)
	}
	if msg.Role != "tool" || msg.ToolCallID != tc.ID {
		t.Errorf("unexpected tool message: %+v", msg)
	}
	if raw == nil {
		t.Error("expected non-nil raw data (toolRawResult)")
	}

	calls := col.filter(protocol.AgentEventToolCall)
	if len(calls) != 1 {
		t.Fatalf("expected 1 tool.call event, got %d (all events: %+v)", len(calls), col.events)
	}
	assertToolCallPayload(t, calls[0], tc, req)
}

// TestMakeExecuteToolRaw_ConcurrentCallsEmitAllEvents confirms the parallel
// wrapper is safe to invoke from multiple goroutines — mirrors the real
// executeParallel dispatch in pipeline/tool_stage.go.
func TestMakeExecuteToolRaw_ConcurrentCallsEmitAllEvents(t *testing.T) {
	t.Parallel()
	col := &eventCollector{}
	l := newTestLoopForToolCallbacks(col.onEvent)

	req := &RunRequest{RunID: "run-3", SessionKey: "sess-C", UserID: "u-3", SenderID: "sender-3", Channel: "ws"}
	exec := l.makeExecuteToolRaw(req)

	const n = 5
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			tc := providers.ToolCall{ID: "tc-" + string(rune('a'+idx)), Name: "t", Arguments: nil}
			if _, _, err := exec(context.Background(), tc); err != nil {
				t.Errorf("goroutine %d: %v", idx, err)
			}
		}(i)
	}
	wg.Wait()

	calls := col.filter(protocol.AgentEventToolCall)
	if len(calls) != n {
		t.Fatalf("expected %d tool.call events, got %d", n, len(calls))
	}
}

func TestRecordToolMetric_UseSkillTracksConcreteSkill(t *testing.T) {
	recorder := &metricRecorder{}
	agentID := uuid.New()
	l := &Loop{
		id:                    "test-agent",
		agentUUID:             agentID,
		evolutionMetricsStore: recorder,
	}
	tenantID := uuid.New()
	ctx := store.WithTenantID(context.Background(), tenantID)

	l.recordToolMetric(ctx, "session-1", "use_skill", map[string]any{"name": "shipping-doc-processing"}, &tools.Result{ForLLM: "ok"}, 25*time.Millisecond)

	metric := recorder.latest(t)
	if metric.AgentID != agentID {
		t.Fatalf("AgentID = %s, want %s", metric.AgentID, agentID)
	}
	if metric.MetricType != store.MetricTool {
		t.Fatalf("MetricType = %s, want tool", metric.MetricType)
	}
	if metric.MetricKey != "skill:shipping-doc-processing" {
		t.Fatalf("MetricKey = %q, want skill:shipping-doc-processing", metric.MetricKey)
	}
	var value map[string]any
	if err := json.Unmarshal(metric.Value, &value); err != nil {
		t.Fatalf("metric value is not JSON: %v", err)
	}
	if value["tool"] != "use_skill" {
		t.Fatalf("tool = %v, want use_skill", value["tool"])
	}
	if value["skill"] != "shipping-doc-processing" {
		t.Fatalf("skill = %v, want shipping-doc-processing", value["skill"])
	}
	if value["success"] != true {
		t.Fatalf("success = %v, want true", value["success"])
	}
}

// assertToolCallPayload verifies the event carries the expected tc identity
// and routing context from RunRequest.
func assertToolCallPayload(t *testing.T, ev AgentEvent, tc providers.ToolCall, req *RunRequest) {
	t.Helper()
	if ev.AgentID != "test-agent" {
		t.Errorf("AgentID: got %q, want test-agent", ev.AgentID)
	}
	if ev.RunID != req.RunID {
		t.Errorf("RunID: got %q, want %q", ev.RunID, req.RunID)
	}
	if ev.SessionKey != req.SessionKey {
		t.Errorf("SessionKey: got %q, want %q", ev.SessionKey, req.SessionKey)
	}
	if ev.Channel != req.Channel {
		t.Errorf("Channel: got %q, want %q", ev.Channel, req.Channel)
	}
	if ev.UserID != req.UserID {
		t.Errorf("UserID: got %q, want %q", ev.UserID, req.UserID)
	}
	if ev.SenderID != req.SenderID {
		t.Errorf("SenderID: got %q, want %q", ev.SenderID, req.SenderID)
	}
	payload, ok := ev.Payload.(map[string]any)
	if !ok {
		t.Fatalf("Payload is not map[string]any: %T", ev.Payload)
	}
	if payload["id"] != tc.ID {
		t.Errorf("payload.id: got %v, want %q", payload["id"], tc.ID)
	}
	if payload["name"] != tc.Name {
		t.Errorf("payload.name: got %v, want %q", payload["name"], tc.Name)
	}
}
