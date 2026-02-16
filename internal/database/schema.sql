-- Oculo Database Schema
-- Optimized for time-series trace data with AI-native semantics.
-- Uses WAL mode for concurrent read/write access and FTS5 for
-- full-text search over prompt/completion content.

-- Enable WAL mode for high-concurrency reads during TUI queries
-- while the daemon is writing ingested traces.
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA cache_size = -64000;  -- 64MB cache
PRAGMA foreign_keys = ON;
PRAGMA temp_store = MEMORY;

-- =============================================================
-- Core Tables
-- =============================================================

-- traces: Top-level execution trace of an AI agent run.
-- Each trace represents a single agent invocation from start to finish.
CREATE TABLE IF NOT EXISTS traces (
    trace_id     TEXT PRIMARY KEY,
    agent_name   TEXT NOT NULL,
    start_time   INTEGER NOT NULL,  -- Unix nanoseconds
    end_time     INTEGER,           -- NULL if still running
    status       TEXT NOT NULL DEFAULT 'running' CHECK(status IN ('running', 'completed', 'failed')),
    metadata     TEXT,              -- JSON blob for extensibility
    created_at   INTEGER NOT NULL DEFAULT (strftime('%s','now') * 1000000000)
);

-- spans: Individual operations within a trace.
-- Forms a tree via parent_span_id for causal chain visualization.
CREATE TABLE IF NOT EXISTS spans (
    span_id          TEXT PRIMARY KEY,
    trace_id         TEXT NOT NULL REFERENCES traces(trace_id) ON DELETE CASCADE,
    parent_span_id   TEXT,          -- NULL for root spans
    operation_type   TEXT NOT NULL CHECK(operation_type IN ('LLM', 'TOOL', 'MEMORY', 'PLANNING', 'RETRIEVAL')),
    operation_name   TEXT NOT NULL DEFAULT '',
    start_time       INTEGER NOT NULL,  -- Unix nanoseconds
    duration_ms      INTEGER NOT NULL DEFAULT 0,
    
    -- AI-specific columns for LLM spans
    prompt           TEXT,
    completion       TEXT,
    prompt_tokens    INTEGER DEFAULT 0,
    completion_tokens INTEGER DEFAULT 0,
    model            TEXT,
    temperature      REAL,
    
    -- Generic metadata
    metadata         TEXT,          -- JSON blob
    status           TEXT NOT NULL DEFAULT 'ok' CHECK(status IN ('ok', 'error')),
    error_message    TEXT,
    
    created_at       INTEGER NOT NULL DEFAULT (strftime('%s','now') * 1000000000)
);

-- memory_events: The heart of Oculo.
-- Captures every mutation to the agent's memory/knowledge store.
-- This is what powers the "Glass Box" diff visualization.
CREATE TABLE IF NOT EXISTS memory_events (
    event_id     TEXT PRIMARY KEY,
    span_id      TEXT NOT NULL REFERENCES spans(span_id) ON DELETE CASCADE,
    timestamp    INTEGER NOT NULL,  -- Unix nanoseconds
    operation    TEXT NOT NULL CHECK(operation IN ('ADD', 'UPDATE', 'DELETE')),
    key          TEXT NOT NULL,
    old_value    TEXT,              -- NULL for ADD operations
    new_value    TEXT,              -- NULL for DELETE operations
    namespace    TEXT DEFAULT 'default',
    created_at   INTEGER NOT NULL DEFAULT (strftime('%s','now') * 1000000000)
);

-- tool_calls: Captures external tool invocations within a span.
CREATE TABLE IF NOT EXISTS tool_calls (
    call_id        INTEGER PRIMARY KEY AUTOINCREMENT,
    span_id        TEXT NOT NULL REFERENCES spans(span_id) ON DELETE CASCADE,
    tool_name      TEXT NOT NULL,
    arguments_json TEXT,
    result_json    TEXT,
    success        INTEGER NOT NULL DEFAULT 1,  -- Boolean
    latency_ms     INTEGER DEFAULT 0,
    created_at     INTEGER NOT NULL DEFAULT (strftime('%s','now') * 1000000000)
);

-- =============================================================
-- Indexes: Optimized for the most common query patterns
-- =============================================================

-- Timeline queries: "Show me all spans for this trace, ordered by time"
CREATE INDEX IF NOT EXISTS idx_spans_trace_time ON spans(trace_id, start_time);

-- Type filtering: "Show me all LLM calls" or "Show me all MEMORY operations"
CREATE INDEX IF NOT EXISTS idx_spans_operation_type ON spans(operation_type);

-- Memory diff timeline: "Show me how this key changed over time"
CREATE INDEX IF NOT EXISTS idx_memory_events_span ON memory_events(span_id, timestamp);
CREATE INDEX IF NOT EXISTS idx_memory_events_key ON memory_events(key, timestamp);
CREATE INDEX IF NOT EXISTS idx_memory_events_namespace ON memory_events(namespace, timestamp);

-- Trace listing: "Show me recent traces for this agent"
CREATE INDEX IF NOT EXISTS idx_traces_agent_time ON traces(agent_name, start_time DESC);
CREATE INDEX IF NOT EXISTS idx_traces_status ON traces(status);

-- Tool call lookup
CREATE INDEX IF NOT EXISTS idx_tool_calls_span ON tool_calls(span_id);

-- =============================================================
-- Full-Text Search: Semantic search over prompt/completion content
-- =============================================================

-- FTS5 virtual table for searching within prompts and completions.
-- Enables queries like: "Find all spans where the agent discussed 'transformer architecture'"
CREATE VIRTUAL TABLE IF NOT EXISTS spans_fts USING fts5(
    span_id UNINDEXED,
    prompt,
    completion,
    operation_name,
    content='spans',
    content_rowid='rowid',
    tokenize='porter unicode61'
);

-- Triggers to keep FTS index synchronized with the spans table
CREATE TRIGGER IF NOT EXISTS spans_ai AFTER INSERT ON spans BEGIN
    INSERT INTO spans_fts(rowid, span_id, prompt, completion, operation_name)
    VALUES (new.rowid, new.span_id, new.prompt, new.completion, new.operation_name);
END;

CREATE TRIGGER IF NOT EXISTS spans_ad AFTER DELETE ON spans BEGIN
    INSERT INTO spans_fts(spans_fts, rowid, span_id, prompt, completion, operation_name)
    VALUES ('delete', old.rowid, old.span_id, old.prompt, old.completion, old.operation_name);
END;

CREATE TRIGGER IF NOT EXISTS spans_au AFTER UPDATE ON spans BEGIN
    INSERT INTO spans_fts(spans_fts, rowid, span_id, prompt, completion, operation_name)
    VALUES ('delete', old.rowid, old.span_id, old.prompt, old.completion, old.operation_name);
    INSERT INTO spans_fts(rowid, span_id, prompt, completion, operation_name)
    VALUES (new.rowid, new.span_id, new.prompt, new.completion, new.operation_name);
END;

-- =============================================================
-- WAL Management Table: For crash recovery of the ingestion daemon
-- =============================================================

-- pending_writes: Write-ahead log for crash-safe ingestion.
-- If the daemon crashes mid-batch, unprocessed entries are replayed on startup.
CREATE TABLE IF NOT EXISTS pending_writes (
    write_id   INTEGER PRIMARY KEY AUTOINCREMENT,
    payload    BLOB NOT NULL,       -- Serialized protobuf
    status     TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending', 'committed', 'failed')),
    created_at INTEGER NOT NULL DEFAULT (strftime('%s','now') * 1000000000),
    committed_at INTEGER
);

CREATE INDEX IF NOT EXISTS idx_pending_writes_status ON pending_writes(status);
