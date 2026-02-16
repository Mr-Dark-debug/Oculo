package database

import (
	"fmt"
	"testing"
	"time"
)

// TestNewDBService verifies that the database initializes correctly
// with the embedded schema using an in-memory SQLite instance.
func TestNewDBService(t *testing.T) {
	svc, err := NewDBService(":memory:")
	if err != nil {
		t.Fatalf("NewDBService(:memory:) failed: %v", err)
	}
	defer svc.Close()
}

// TestInsertAndQueryTrace verifies the full trace lifecycle:
// insert → query → verify fields match.
func TestInsertAndQueryTrace(t *testing.T) {
	svc, err := NewDBService(":memory:")
	if err != nil {
		t.Fatalf("NewDBService failed: %v", err)
	}
	defer svc.Close()

	now := time.Now().UnixNano()
	trace := &Trace{
		TraceID:   "trace-001",
		AgentName: "test-agent",
		StartTime: now,
		Status:    "running",
		Metadata:  map[string]string{"env": "test"},
	}

	if err := svc.InsertTrace(trace); err != nil {
		t.Fatalf("InsertTrace failed: %v", err)
	}

	// Query back
	traces, err := svc.QueryTraces(TraceFilter{Limit: 10})
	if err != nil {
		t.Fatalf("QueryTraces failed: %v", err)
	}
	if len(traces) != 1 {
		t.Fatalf("expected 1 trace, got %d", len(traces))
	}
	if traces[0].TraceID != "trace-001" {
		t.Errorf("expected trace_id=trace-001, got %s", traces[0].TraceID)
	}
	if traces[0].AgentName != "test-agent" {
		t.Errorf("expected agent_name=test-agent, got %s", traces[0].AgentName)
	}
	if traces[0].Metadata["env"] != "test" {
		t.Errorf("expected metadata env=test, got %v", traces[0].Metadata)
	}
}

// TestInsertSpanAndQueryTimeline verifies span insertion and
// timeline ordering within a trace.
func TestInsertSpanAndQueryTimeline(t *testing.T) {
	svc, err := NewDBService(":memory:")
	if err != nil {
		t.Fatalf("NewDBService failed: %v", err)
	}
	defer svc.Close()

	now := time.Now().UnixNano()
	trace := &Trace{
		TraceID:   "trace-002",
		AgentName: "timeline-agent",
		StartTime: now,
		Status:    "completed",
	}
	if err := svc.InsertTrace(trace); err != nil {
		t.Fatalf("InsertTrace failed: %v", err)
	}

	// Insert spans with different start times
	prompt := "What is the capital of France?"
	completion := "The capital of France is Paris."
	model := "gpt-4"
	temp := 0.7
	spans := []*Span{
		{
			SpanID: "span-001", TraceID: "trace-002",
			OperationType: "LLM", OperationName: "gpt-4-call",
			StartTime: now, DurationMs: 1200,
			Prompt: &prompt, Completion: &completion,
			PromptTokens: 10, CompletionTokens: 8,
			Model: &model, Temperature: &temp,
			Status: "ok",
		},
		{
			SpanID: "span-002", TraceID: "trace-002",
			OperationType: "TOOL", OperationName: "search_web",
			StartTime: now + 1000000, DurationMs: 500,
			Status: "ok",
		},
		{
			SpanID: "span-003", TraceID: "trace-002",
			OperationType: "MEMORY", OperationName: "update_knowledge",
			StartTime: now + 2000000, DurationMs: 10,
			Status: "ok",
		},
	}

	for _, sp := range spans {
		if err := svc.InsertSpan(sp); err != nil {
			t.Fatalf("InsertSpan(%s) failed: %v", sp.SpanID, err)
		}
	}

	timeline, err := svc.QueryTimeline("trace-002")
	if err != nil {
		t.Fatalf("QueryTimeline failed: %v", err)
	}
	if len(timeline) != 3 {
		t.Fatalf("expected 3 spans in timeline, got %d", len(timeline))
	}

	// Verify ordering
	for i := 1; i < len(timeline); i++ {
		if timeline[i].StartTime < timeline[i-1].StartTime {
			t.Errorf("timeline not ordered: span %d start_time < span %d start_time",
				i, i-1)
		}
	}
}

// TestMemoryDiffs verifies the core feature: memory mutation tracking.
func TestMemoryDiffs(t *testing.T) {
	svc, err := NewDBService(":memory:")
	if err != nil {
		t.Fatalf("NewDBService failed: %v", err)
	}
	defer svc.Close()

	now := time.Now().UnixNano()

	// Setup trace and span
	svc.InsertTrace(&Trace{
		TraceID: "trace-003", AgentName: "memory-agent",
		StartTime: now, Status: "completed",
	})
	svc.InsertSpan(&Span{
		SpanID: "span-010", TraceID: "trace-003",
		OperationType: "MEMORY", OperationName: "update",
		StartTime: now, DurationMs: 5, Status: "ok",
	})

	// Insert memory events
	oldVal := "Paris"
	newVal := "Berlin"
	events := []*MemoryEvent{
		{
			EventID: "evt-001", SpanID: "span-010",
			Timestamp: now, Operation: "ADD",
			Key: "capital", NewValue: &newVal,
			Namespace: "geography",
		},
		{
			EventID: "evt-002", SpanID: "span-010",
			Timestamp: now + 1000, Operation: "UPDATE",
			Key: "capital", OldValue: &oldVal, NewValue: &newVal,
			Namespace: "geography",
		},
		{
			EventID: "evt-003", SpanID: "span-010",
			Timestamp: now + 2000, Operation: "DELETE",
			Key: "outdated", OldValue: &oldVal,
			Namespace: "geography",
		},
	}

	for _, ev := range events {
		if err := svc.InsertMemoryEvent(ev); err != nil {
			t.Fatalf("InsertMemoryEvent(%s) failed: %v", ev.EventID, err)
		}
	}

	diffs, err := svc.GetMemoryDiffs("span-010")
	if err != nil {
		t.Fatalf("GetMemoryDiffs failed: %v", err)
	}
	if len(diffs) != 3 {
		t.Fatalf("expected 3 memory diffs, got %d", len(diffs))
	}

	// Verify operations
	ops := []string{"ADD", "UPDATE", "DELETE"}
	for i, d := range diffs {
		if d.Operation != ops[i] {
			t.Errorf("event %d: expected op=%s, got %s", i, ops[i], d.Operation)
		}
	}

	// Test memory timeline for a specific key
	timeline, err := svc.GetMemoryTimeline("capital", "geography")
	if err != nil {
		t.Fatalf("GetMemoryTimeline failed: %v", err)
	}
	if len(timeline) != 2 {
		t.Fatalf("expected 2 events for key 'capital', got %d", len(timeline))
	}
}

// TestBatchInsertSpans verifies that batch insertion works correctly.
func TestBatchInsertSpans(t *testing.T) {
	svc, err := NewDBService(":memory:")
	if err != nil {
		t.Fatalf("NewDBService failed: %v", err)
	}
	defer svc.Close()

	now := time.Now().UnixNano()
	svc.InsertTrace(&Trace{
		TraceID: "trace-batch", AgentName: "batch-agent",
		StartTime: now, Status: "completed",
	})

	spans := make([]*Span, 100)
	for i := 0; i < 100; i++ {
		spans[i] = &Span{
			SpanID:        fmt.Sprintf("batch-span-%03d", i),
			TraceID:       "trace-batch",
			OperationType: "LLM",
			OperationName: fmt.Sprintf("call-%d", i),
			StartTime:     now + int64(i*1000000),
			DurationMs:    100,
			PromptTokens:  50,
			CompletionTokens: 30,
			Status:        "ok",
		}
	}

	if err := svc.BatchInsertSpans(spans); err != nil {
		t.Fatalf("BatchInsertSpans failed: %v", err)
	}

	timeline, err := svc.QueryTimeline("trace-batch")
	if err != nil {
		t.Fatalf("QueryTimeline after batch failed: %v", err)
	}
	if len(timeline) != 100 {
		t.Errorf("expected 100 spans, got %d", len(timeline))
	}
}

// TestSearchContent verifies full-text search over prompt/completion content.
func TestSearchContent(t *testing.T) {
	svc, err := NewDBService(":memory:")
	if err != nil {
		t.Fatalf("NewDBService failed: %v", err)
	}
	defer svc.Close()

	now := time.Now().UnixNano()
	svc.InsertTrace(&Trace{
		TraceID: "trace-fts", AgentName: "search-agent",
		StartTime: now, Status: "completed",
	})

	prompt1 := "Analyze the transformer architecture in this research paper"
	completion1 := "The paper discusses attention mechanisms and self-attention layers"
	prompt2 := "What is the weather forecast for tomorrow?"
	completion2 := "Tomorrow will be sunny with a high of 75 degrees"

	svc.InsertSpan(&Span{
		SpanID: "fts-001", TraceID: "trace-fts",
		OperationType: "LLM", OperationName: "analysis",
		StartTime: now, DurationMs: 500,
		Prompt: &prompt1, Completion: &completion1,
		Status: "ok",
	})
	svc.InsertSpan(&Span{
		SpanID: "fts-002", TraceID: "trace-fts",
		OperationType: "LLM", OperationName: "weather",
		StartTime: now + 1000000, DurationMs: 300,
		Prompt: &prompt2, Completion: &completion2,
		Status: "ok",
	})

	// Search for "transformer"
	results, err := svc.SearchContent("transformer", 10)
	if err != nil {
		t.Fatalf("SearchContent('transformer') failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for 'transformer', got %d", len(results))
	}
	if results[0].SpanID != "fts-001" {
		t.Errorf("expected span fts-001, got %s", results[0].SpanID)
	}
}

// TestGetTraceStats verifies aggregated statistics computation.
func TestGetTraceStats(t *testing.T) {
	svc, err := NewDBService(":memory:")
	if err != nil {
		t.Fatalf("NewDBService failed: %v", err)
	}
	defer svc.Close()

	now := time.Now().UnixNano()
	svc.InsertTrace(&Trace{
		TraceID: "trace-stats", AgentName: "stats-agent",
		StartTime: now, Status: "completed",
	})

	// Insert 2 LLM spans, 1 TOOL span, 1 MEMORY span
	svc.InsertSpan(&Span{
		SpanID: "ss-001", TraceID: "trace-stats",
		OperationType: "LLM", StartTime: now, DurationMs: 1000,
		PromptTokens: 100, CompletionTokens: 50, Status: "ok",
	})
	svc.InsertSpan(&Span{
		SpanID: "ss-002", TraceID: "trace-stats",
		OperationType: "LLM", StartTime: now + 1000, DurationMs: 800,
		PromptTokens: 200, CompletionTokens: 100, Status: "ok",
	})
	svc.InsertSpan(&Span{
		SpanID: "ss-003", TraceID: "trace-stats",
		OperationType: "TOOL", StartTime: now + 2000, DurationMs: 500,
		Status: "ok",
	})
	svc.InsertSpan(&Span{
		SpanID: "ss-004", TraceID: "trace-stats",
		OperationType: "MEMORY", StartTime: now + 3000, DurationMs: 10,
		Status: "ok",
	})

	stats, err := svc.GetTraceStats("trace-stats")
	if err != nil {
		t.Fatalf("GetTraceStats failed: %v", err)
	}

	if stats.TotalSpans != 4 {
		t.Errorf("expected 4 total spans, got %d", stats.TotalSpans)
	}
	if stats.LLMCalls != 2 {
		t.Errorf("expected 2 LLM calls, got %d", stats.LLMCalls)
	}
	if stats.ToolCalls != 1 {
		t.Errorf("expected 1 tool call, got %d", stats.ToolCalls)
	}
	if stats.MemoryOps != 1 {
		t.Errorf("expected 1 memory op, got %d", stats.MemoryOps)
	}
	if stats.TotalPromptTokens != 300 {
		t.Errorf("expected 300 prompt tokens, got %d", stats.TotalPromptTokens)
	}
	if stats.TotalCompletionTokens != 150 {
		t.Errorf("expected 150 completion tokens, got %d", stats.TotalCompletionTokens)
	}
}

// TestPendingWrites verifies the crash recovery mechanism.
func TestPendingWrites(t *testing.T) {
	svc, err := NewDBService(":memory:")
	if err != nil {
		t.Fatalf("NewDBService failed: %v", err)
	}
	defer svc.Close()

	payload := []byte(`{"test": "data"}`)

	writeID, err := svc.WritePendingPayload(payload)
	if err != nil {
		t.Fatalf("WritePendingPayload failed: %v", err)
	}

	pending, err := svc.GetPendingPayloads()
	if err != nil {
		t.Fatalf("GetPendingPayloads failed: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending write, got %d", len(pending))
	}

	if err := svc.CommitPendingPayload(writeID); err != nil {
		t.Fatalf("CommitPendingPayload failed: %v", err)
	}

	pending, err = svc.GetPendingPayloads()
	if err != nil {
		t.Fatalf("GetPendingPayloads after commit failed: %v", err)
	}
	if len(pending) != 0 {
		t.Errorf("expected 0 pending writes after commit, got %d", len(pending))
	}
}

// TestTraceFilterByAgent verifies filtering traces by agent name.
func TestTraceFilterByAgent(t *testing.T) {
	svc, err := NewDBService(":memory:")
	if err != nil {
		t.Fatalf("NewDBService failed: %v", err)
	}
	defer svc.Close()

	now := time.Now().UnixNano()
	agents := []string{"alpha-agent", "beta-agent", "alpha-agent"}
	for i, name := range agents {
		svc.InsertTrace(&Trace{
			TraceID:   fmt.Sprintf("filter-%d", i),
			AgentName: name,
			StartTime: now + int64(i*1000000),
			Status:    "completed",
		})
	}

	agentName := "alpha-agent"
	results, err := svc.QueryTraces(TraceFilter{AgentName: &agentName, Limit: 10})
	if err != nil {
		t.Fatalf("QueryTraces with filter failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 traces for alpha-agent, got %d", len(results))
	}
}

// BenchmarkBatchInsert measures the throughput of batch span insertion.
func BenchmarkBatchInsert(b *testing.B) {
	svc, err := NewDBService(":memory:")
	if err != nil {
		b.Fatalf("NewDBService failed: %v", err)
	}
	defer svc.Close()

	now := time.Now().UnixNano()
	svc.InsertTrace(&Trace{
		TraceID: "bench-trace", AgentName: "bench-agent",
		StartTime: now, Status: "running",
	})

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		spans := make([]*Span, 1000)
		for i := 0; i < 1000; i++ {
			spans[i] = &Span{
				SpanID:        fmt.Sprintf("bench-%d-%d", n, i),
				TraceID:       "bench-trace",
				OperationType: "LLM",
				StartTime:     now + int64(i),
				DurationMs:    100,
				Status:        "ok",
			}
		}
		if err := svc.BatchInsertSpans(spans); err != nil {
			b.Fatalf("BatchInsertSpans failed: %v", err)
		}
	}
}
