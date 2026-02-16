// Package database provides the storage layer for Oculo.
//
// It implements the Store interface using SQLite with WAL mode,
// FTS5 full-text search, and optimized indexes for time-series
// trace data. The DBService struct is the primary entry point
// for all database operations.
package database

import (
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed schema.sql
var schemaFS embed.FS

// Store defines the interface for trace data persistence.
// This abstraction allows for mocking in tests and potential
// future backends beyond SQLite.
type Store interface {
	// InsertTrace persists a new trace record.
	InsertTrace(trace *Trace) error
	// InsertSpan persists a new span within an existing trace.
	InsertSpan(span *Span) error
	// InsertMemoryEvent persists a memory mutation event.
	InsertMemoryEvent(event *MemoryEvent) error
	// InsertToolCall persists a tool call record.
	InsertToolCall(call *ToolCall) error

	// BatchInsertSpans inserts multiple spans in a single transaction.
	BatchInsertSpans(spans []*Span) error
	// BatchInsertMemoryEvents inserts multiple memory events in a single transaction.
	BatchInsertMemoryEvents(events []*MemoryEvent) error

	// QueryTraces returns traces matching the given filter, ordered by start_time DESC.
	QueryTraces(filter TraceFilter) ([]*Trace, error)
	// QueryTimeline returns all spans for a trace, ordered by start_time.
	QueryTimeline(traceID string) ([]*Span, error)
	// GetMemoryDiffs returns all memory events for a span, ordered by timestamp.
	GetMemoryDiffs(spanID string) ([]*MemoryEvent, error)
	// GetMemoryTimeline returns the full mutation history for a memory key.
	GetMemoryTimeline(key string, namespace string) ([]*MemoryEvent, error)
	// SearchContent performs full-text search over prompt/completion content.
	SearchContent(query string, limit int) ([]*Span, error)
	// GetTraceStats returns aggregated statistics for a trace.
	GetTraceStats(traceID string) (*TraceStats, error)

	// WritePendingPayload stores a raw payload for crash recovery.
	WritePendingPayload(payload []byte) (int64, error)
	// CommitPendingPayload marks a pending write as committed.
	CommitPendingPayload(writeID int64) error
	// GetPendingPayloads returns all payloads that haven't been committed.
	GetPendingPayloads() ([]PendingWrite, error)

	// Close gracefully shuts down the database connection.
	Close() error
}

// ============================================================
// Domain Models
// ============================================================

// Trace represents a complete execution trace of an AI agent.
type Trace struct {
	TraceID   string            `json:"trace_id"`
	AgentName string            `json:"agent_name"`
	StartTime int64             `json:"start_time"`
	EndTime   *int64            `json:"end_time,omitempty"`
	Status    string            `json:"status"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// Span represents a single operation within a trace.
type Span struct {
	SpanID           string  `json:"span_id"`
	TraceID          string  `json:"trace_id"`
	ParentSpanID     *string `json:"parent_span_id,omitempty"`
	OperationType    string  `json:"operation_type"`
	OperationName    string  `json:"operation_name"`
	StartTime        int64   `json:"start_time"`
	DurationMs       int64   `json:"duration_ms"`
	Prompt           *string `json:"prompt,omitempty"`
	Completion       *string `json:"completion,omitempty"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	Model            *string `json:"model,omitempty"`
	Temperature      *float64 `json:"temperature,omitempty"`
	Metadata         *string `json:"metadata,omitempty"`
	Status           string  `json:"status"`
	ErrorMessage     *string `json:"error_message,omitempty"`
}

// MemoryEvent captures a single mutation to the agent's memory.
type MemoryEvent struct {
	EventID   string `json:"event_id"`
	SpanID    string `json:"span_id"`
	Timestamp int64  `json:"timestamp"`
	Operation string `json:"operation"`
	Key       string `json:"key"`
	OldValue  *string `json:"old_value,omitempty"`
	NewValue  *string `json:"new_value,omitempty"`
	Namespace string `json:"namespace"`
}

// ToolCall captures an external tool invocation.
type ToolCall struct {
	CallID        int64  `json:"call_id"`
	SpanID        string `json:"span_id"`
	ToolName      string `json:"tool_name"`
	ArgumentsJSON *string `json:"arguments_json,omitempty"`
	ResultJSON    *string `json:"result_json,omitempty"`
	Success       bool   `json:"success"`
	LatencyMs     int64  `json:"latency_ms"`
}

// TraceFilter defines query parameters for trace listing.
type TraceFilter struct {
	AgentName *string `json:"agent_name,omitempty"`
	Status    *string `json:"status,omitempty"`
	Since     *int64  `json:"since,omitempty"` // Unix nanoseconds
	Until     *int64  `json:"until,omitempty"` // Unix nanoseconds
	Limit     int     `json:"limit"`
	Offset    int     `json:"offset"`
}

// TraceStats holds aggregated statistics for a single trace.
type TraceStats struct {
	TraceID          string `json:"trace_id"`
	TotalSpans       int    `json:"total_spans"`
	LLMCalls         int    `json:"llm_calls"`
	ToolCalls        int    `json:"tool_calls"`
	MemoryOps        int    `json:"memory_ops"`
	TotalPromptTokens    int `json:"total_prompt_tokens"`
	TotalCompletionTokens int `json:"total_completion_tokens"`
	TotalDurationMs  int64  `json:"total_duration_ms"`
	MemoryEventCount int    `json:"memory_event_count"`
}

// PendingWrite represents an uncommitted ingestion payload.
type PendingWrite struct {
	WriteID   int64  `json:"write_id"`
	Payload   []byte `json:"payload"`
	Status    string `json:"status"`
	CreatedAt int64  `json:"created_at"`
}

// ============================================================
// DBService Implementation
// ============================================================

// DBService implements the Store interface using SQLite.
// It manages the database connection pool, prepared statements,
// and ensures thread-safe access through a read-write mutex.
type DBService struct {
	db   *sql.DB
	mu   sync.RWMutex
	path string

	// Prepared statements for hot-path operations
	stmtInsertTrace       *sql.Stmt
	stmtInsertSpan        *sql.Stmt
	stmtInsertMemoryEvent *sql.Stmt
	stmtInsertToolCall    *sql.Stmt
	stmtInsertPending     *sql.Stmt
	stmtCommitPending     *sql.Stmt
}

// NewDBService creates a new database service, initializes the schema,
// and prepares frequently-used statements.
//
// The path parameter specifies the SQLite database file location.
// Use ":memory:" for in-memory databases (useful for testing).
func NewDBService(path string) (*DBService, error) {
	// Enable WAL mode, foreign keys, and other optimizations via DSN
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_synchronous=NORMAL&_foreign_keys=ON&_cache_size=-64000", path)
	
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database at %s: %w", path, err)
	}

	// Set connection pool parameters for SQLite
	// SQLite handles concurrency through WAL mode, so we limit writers
	db.SetMaxOpenConns(1) // SQLite only supports one writer at a time
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0) // Keep connection alive

	svc := &DBService{
		db:   db,
		path: path,
	}

	if err := svc.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("initializing schema: %w", err)
	}

	if err := svc.prepareStatements(); err != nil {
		db.Close()
		return nil, fmt.Errorf("preparing statements: %w", err)
	}

	return svc, nil
}

// initSchema reads the embedded schema.sql and executes it to create
// all tables, indexes, triggers, and FTS5 virtual tables.
func (s *DBService) initSchema() error {
	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return fmt.Errorf("reading embedded schema: %w", err)
	}

	if _, err := s.db.Exec(string(schema)); err != nil {
		return fmt.Errorf("executing schema: %w", err)
	}

	return nil
}

// prepareStatements creates prepared statements for frequently-used
// insert and update operations to minimize parsing overhead.
func (s *DBService) prepareStatements() error {
	var err error

	s.stmtInsertTrace, err = s.db.Prepare(`
		INSERT INTO traces (trace_id, agent_name, start_time, end_time, status, metadata)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(trace_id) DO UPDATE SET
			end_time = COALESCE(excluded.end_time, traces.end_time),
			status = excluded.status,
			metadata = COALESCE(excluded.metadata, traces.metadata)
	`)
	if err != nil {
		return fmt.Errorf("preparing InsertTrace: %w", err)
	}

	s.stmtInsertSpan, err = s.db.Prepare(`
		INSERT INTO spans (span_id, trace_id, parent_span_id, operation_type, operation_name,
			start_time, duration_ms, prompt, completion, prompt_tokens, completion_tokens,
			model, temperature, metadata, status, error_message)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(span_id) DO UPDATE SET
			duration_ms = excluded.duration_ms,
			completion = COALESCE(excluded.completion, spans.completion),
			completion_tokens = excluded.completion_tokens,
			status = excluded.status,
			error_message = excluded.error_message
	`)
	if err != nil {
		return fmt.Errorf("preparing InsertSpan: %w", err)
	}

	s.stmtInsertMemoryEvent, err = s.db.Prepare(`
		INSERT INTO memory_events (event_id, span_id, timestamp, operation, key, old_value, new_value, namespace)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("preparing InsertMemoryEvent: %w", err)
	}

	s.stmtInsertToolCall, err = s.db.Prepare(`
		INSERT INTO tool_calls (span_id, tool_name, arguments_json, result_json, success, latency_ms)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("preparing InsertToolCall: %w", err)
	}

	s.stmtInsertPending, err = s.db.Prepare(`
		INSERT INTO pending_writes (payload, status) VALUES (?, 'pending')
	`)
	if err != nil {
		return fmt.Errorf("preparing InsertPending: %w", err)
	}

	s.stmtCommitPending, err = s.db.Prepare(`
		UPDATE pending_writes SET status = 'committed', committed_at = ? WHERE write_id = ?
	`)
	if err != nil {
		return fmt.Errorf("preparing CommitPending: %w", err)
	}

	return nil
}

// InsertTrace persists a new trace record. If a trace with the same ID
// already exists, it updates the end_time, status, and metadata.
func (s *DBService) InsertTrace(trace *Trace) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var metadataJSON *string
	if trace.Metadata != nil {
		b, err := json.Marshal(trace.Metadata)
		if err != nil {
			return fmt.Errorf("marshaling trace metadata: %w", err)
		}
		str := string(b)
		metadataJSON = &str
	}

	_, err := s.stmtInsertTrace.Exec(
		trace.TraceID, trace.AgentName, trace.StartTime, trace.EndTime,
		trace.Status, metadataJSON,
	)
	if err != nil {
		return fmt.Errorf("inserting trace %s: %w", trace.TraceID, err)
	}
	return nil
}

// InsertSpan persists a new span within an existing trace.
// If a span with the same ID already exists, it updates
// duration, completion, tokens, and status.
func (s *DBService) InsertSpan(span *Span) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.stmtInsertSpan.Exec(
		span.SpanID, span.TraceID, span.ParentSpanID, span.OperationType,
		span.OperationName, span.StartTime, span.DurationMs,
		span.Prompt, span.Completion, span.PromptTokens, span.CompletionTokens,
		span.Model, span.Temperature, span.Metadata,
		span.Status, span.ErrorMessage,
	)
	if err != nil {
		return fmt.Errorf("inserting span %s: %w", span.SpanID, err)
	}
	return nil
}

// InsertMemoryEvent persists a memory mutation event.
func (s *DBService) InsertMemoryEvent(event *MemoryEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.stmtInsertMemoryEvent.Exec(
		event.EventID, event.SpanID, event.Timestamp,
		event.Operation, event.Key, event.OldValue, event.NewValue,
		event.Namespace,
	)
	if err != nil {
		return fmt.Errorf("inserting memory event %s: %w", event.EventID, err)
	}
	return nil
}

// InsertToolCall persists a tool call record.
func (s *DBService) InsertToolCall(call *ToolCall) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.stmtInsertToolCall.Exec(
		call.SpanID, call.ToolName, call.ArgumentsJSON,
		call.ResultJSON, call.Success, call.LatencyMs,
	)
	if err != nil {
		return fmt.Errorf("inserting tool call for span %s: %w", call.SpanID, err)
	}
	return nil
}

// BatchInsertSpans inserts multiple spans within a single transaction
// for improved throughput during batch ingestion.
func (s *DBService) BatchInsertSpans(spans []*Span) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning batch span transaction: %w", err)
	}
	defer tx.Rollback() // No-op if committed

	stmt := tx.Stmt(s.stmtInsertSpan)
	for _, span := range spans {
		_, err := stmt.Exec(
			span.SpanID, span.TraceID, span.ParentSpanID, span.OperationType,
			span.OperationName, span.StartTime, span.DurationMs,
			span.Prompt, span.Completion, span.PromptTokens, span.CompletionTokens,
			span.Model, span.Temperature, span.Metadata,
			span.Status, span.ErrorMessage,
		)
		if err != nil {
			return fmt.Errorf("batch inserting span %s: %w", span.SpanID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing batch span transaction: %w", err)
	}
	return nil
}

// BatchInsertMemoryEvents inserts multiple memory events within a single
// transaction for improved throughput.
func (s *DBService) BatchInsertMemoryEvents(events []*MemoryEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning batch memory event transaction: %w", err)
	}
	defer tx.Rollback()

	stmt := tx.Stmt(s.stmtInsertMemoryEvent)
	for _, event := range events {
		_, err := stmt.Exec(
			event.EventID, event.SpanID, event.Timestamp,
			event.Operation, event.Key, event.OldValue, event.NewValue,
			event.Namespace,
		)
		if err != nil {
			return fmt.Errorf("batch inserting memory event %s: %w", event.EventID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing batch memory event transaction: %w", err)
	}
	return nil
}

// QueryTraces returns traces matching the given filter criteria.
// Results are ordered by start_time descending (most recent first).
func (s *DBService) QueryTraces(filter TraceFilter) ([]*Trace, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT trace_id, agent_name, start_time, end_time, status, metadata FROM traces WHERE 1=1`
	args := make([]interface{}, 0)

	if filter.AgentName != nil {
		query += ` AND agent_name = ?`
		args = append(args, *filter.AgentName)
	}
	if filter.Status != nil {
		query += ` AND status = ?`
		args = append(args, *filter.Status)
	}
	if filter.Since != nil {
		query += ` AND start_time >= ?`
		args = append(args, *filter.Since)
	}
	if filter.Until != nil {
		query += ` AND start_time <= ?`
		args = append(args, *filter.Until)
	}

	query += ` ORDER BY start_time DESC`

	if filter.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, filter.Limit)
	} else {
		query += ` LIMIT 100`
	}
	if filter.Offset > 0 {
		query += ` OFFSET ?`
		args = append(args, filter.Offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying traces: %w", err)
	}
	defer rows.Close()

	var traces []*Trace
	for rows.Next() {
		t := &Trace{}
		var metadataStr *string
		if err := rows.Scan(&t.TraceID, &t.AgentName, &t.StartTime, &t.EndTime, &t.Status, &metadataStr); err != nil {
			return nil, fmt.Errorf("scanning trace row: %w", err)
		}
		if metadataStr != nil {
			t.Metadata = make(map[string]string)
			if err := json.Unmarshal([]byte(*metadataStr), &t.Metadata); err != nil {
				// Non-fatal: metadata is supplementary
				t.Metadata = map[string]string{"_raw": *metadataStr}
			}
		}
		traces = append(traces, t)
	}
	return traces, rows.Err()
}

// QueryTimeline returns all spans for a given trace, ordered by start_time.
// This is the primary query for the TUI timeline view.
func (s *DBService) QueryTimeline(traceID string) ([]*Span, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT span_id, trace_id, parent_span_id, operation_type, operation_name,
			start_time, duration_ms, prompt, completion, prompt_tokens, completion_tokens,
			model, temperature, metadata, status, error_message
		FROM spans
		WHERE trace_id = ?
		ORDER BY start_time ASC
	`, traceID)
	if err != nil {
		return nil, fmt.Errorf("querying timeline for trace %s: %w", traceID, err)
	}
	defer rows.Close()

	return scanSpans(rows)
}

// GetMemoryDiffs returns all memory events for a given span,
// ordered by timestamp. This powers the bottom diff pane in the TUI.
func (s *DBService) GetMemoryDiffs(spanID string) ([]*MemoryEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT event_id, span_id, timestamp, operation, key, old_value, new_value, namespace
		FROM memory_events
		WHERE span_id = ?
		ORDER BY timestamp ASC
	`, spanID)
	if err != nil {
		return nil, fmt.Errorf("querying memory diffs for span %s: %w", spanID, err)
	}
	defer rows.Close()

	return scanMemoryEvents(rows)
}

// GetMemoryTimeline returns the full mutation history for a specific
// memory key within a namespace. This lets users answer:
// "When did the agent start believing X?"
func (s *DBService) GetMemoryTimeline(key string, namespace string) ([]*MemoryEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT event_id, span_id, timestamp, operation, key, old_value, new_value, namespace
		FROM memory_events
		WHERE key = ? AND namespace = ?
		ORDER BY timestamp ASC
	`, key, namespace)
	if err != nil {
		return nil, fmt.Errorf("querying memory timeline for key %s: %w", key, err)
	}
	defer rows.Close()

	return scanMemoryEvents(rows)
}

// SearchContent performs full-text search over prompt and completion content
// using the FTS5 index. Returns matching spans with BM25 relevance ranking.
func (s *DBService) SearchContent(query string, limit int) ([]*Span, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 20
	}

	rows, err := s.db.Query(`
		SELECT s.span_id, s.trace_id, s.parent_span_id, s.operation_type, s.operation_name,
			s.start_time, s.duration_ms, s.prompt, s.completion, s.prompt_tokens, s.completion_tokens,
			s.model, s.temperature, s.metadata, s.status, s.error_message
		FROM spans s
		INNER JOIN spans_fts f ON s.span_id = f.span_id
		WHERE spans_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("searching content for %q: %w", query, err)
	}
	defer rows.Close()

	return scanSpans(rows)
}

// GetTraceStats returns aggregated statistics for a trace.
// Used by the TUI detail pane and the analysis engine.
func (s *DBService) GetTraceStats(traceID string) (*TraceStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := &TraceStats{TraceID: traceID}

	err := s.db.QueryRow(`
		SELECT
			COUNT(*) as total_spans,
			COALESCE(SUM(CASE WHEN operation_type = 'LLM' THEN 1 ELSE 0 END), 0) as llm_calls,
			COALESCE(SUM(CASE WHEN operation_type = 'TOOL' THEN 1 ELSE 0 END), 0) as tool_calls,
			COALESCE(SUM(CASE WHEN operation_type = 'MEMORY' THEN 1 ELSE 0 END), 0) as memory_ops,
			COALESCE(SUM(prompt_tokens), 0) as total_prompt_tokens,
			COALESCE(SUM(completion_tokens), 0) as total_completion_tokens,
			COALESCE(SUM(duration_ms), 0) as total_duration_ms
		FROM spans
		WHERE trace_id = ?
	`, traceID).Scan(
		&stats.TotalSpans, &stats.LLMCalls, &stats.ToolCalls, &stats.MemoryOps,
		&stats.TotalPromptTokens, &stats.TotalCompletionTokens, &stats.TotalDurationMs,
	)
	if err != nil {
		return nil, fmt.Errorf("querying trace stats for %s: %w", traceID, err)
	}

	err = s.db.QueryRow(`
		SELECT COUNT(*) FROM memory_events me
		INNER JOIN spans s ON me.span_id = s.span_id
		WHERE s.trace_id = ?
	`, traceID).Scan(&stats.MemoryEventCount)
	if err != nil {
		return nil, fmt.Errorf("counting memory events for trace %s: %w", traceID, err)
	}

	return stats, nil
}

// WritePendingPayload stores a raw payload in the pending_writes table
// for crash recovery. Returns the write ID for later commitment.
func (s *DBService) WritePendingPayload(payload []byte) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.stmtInsertPending.Exec(payload)
	if err != nil {
		return 0, fmt.Errorf("writing pending payload: %w", err)
	}
	return result.LastInsertId()
}

// CommitPendingPayload marks a pending write as committed.
func (s *DBService) CommitPendingPayload(writeID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UnixNano()
	_, err := s.stmtCommitPending.Exec(now, writeID)
	if err != nil {
		return fmt.Errorf("committing pending payload %d: %w", writeID, err)
	}
	return nil
}

// GetPendingPayloads returns all uncommitted payloads for crash recovery.
func (s *DBService) GetPendingPayloads() ([]PendingWrite, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT write_id, payload, status, created_at
		FROM pending_writes
		WHERE status = 'pending'
		ORDER BY write_id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("querying pending payloads: %w", err)
	}
	defer rows.Close()

	var writes []PendingWrite
	for rows.Next() {
		var w PendingWrite
		if err := rows.Scan(&w.WriteID, &w.Payload, &w.Status, &w.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning pending write: %w", err)
		}
		writes = append(writes, w)
	}
	return writes, rows.Err()
}

// Close gracefully shuts down the database, closing all prepared statements
// and the underlying connection pool.
func (s *DBService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	stmts := []*sql.Stmt{
		s.stmtInsertTrace, s.stmtInsertSpan, s.stmtInsertMemoryEvent,
		s.stmtInsertToolCall, s.stmtInsertPending, s.stmtCommitPending,
	}
	for _, stmt := range stmts {
		if stmt != nil {
			stmt.Close()
		}
	}

	return s.db.Close()
}

// ============================================================
// Scan Helpers
// ============================================================

func scanSpans(rows *sql.Rows) ([]*Span, error) {
	var spans []*Span
	for rows.Next() {
		sp := &Span{}
		if err := rows.Scan(
			&sp.SpanID, &sp.TraceID, &sp.ParentSpanID, &sp.OperationType,
			&sp.OperationName, &sp.StartTime, &sp.DurationMs,
			&sp.Prompt, &sp.Completion, &sp.PromptTokens, &sp.CompletionTokens,
			&sp.Model, &sp.Temperature, &sp.Metadata,
			&sp.Status, &sp.ErrorMessage,
		); err != nil {
			return nil, fmt.Errorf("scanning span row: %w", err)
		}
		spans = append(spans, sp)
	}
	return spans, rows.Err()
}

func scanMemoryEvents(rows *sql.Rows) ([]*MemoryEvent, error) {
	var events []*MemoryEvent
	for rows.Next() {
		ev := &MemoryEvent{}
		if err := rows.Scan(
			&ev.EventID, &ev.SpanID, &ev.Timestamp,
			&ev.Operation, &ev.Key, &ev.OldValue, &ev.NewValue,
			&ev.Namespace,
		); err != nil {
			return nil, fmt.Errorf("scanning memory event row: %w", err)
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}
