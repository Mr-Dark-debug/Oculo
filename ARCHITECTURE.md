# Oculo Architecture

> Technical documentation for the internal design of Oculo — The Glass Box for AI Agents.

---

## Table of Contents

1. [System Overview](#system-overview)
2. [Data Flow](#data-flow)
3. [Storage Layer](#storage-layer)
4. [Wire Protocol](#wire-protocol)
5. [Ingestion Pipeline](#ingestion-pipeline)
6. [Analysis Engine](#analysis-engine)
7. [TUI Rendering](#tui-rendering)
8. [Design Decisions](#design-decisions)

---

## System Overview

Oculo is a **three-layer system** designed for inspecting AI agent cognition:

```
┌──────────────────────────────────────────────────────────────┐
│                    INTERFACE LAYER                           │
│  ┌──────────┐  ┌──────────┐  ┌──────────────────────────┐    │
│  │ oculo    │  │ oculo-tui│  │ HTTP Metrics             │    │
│  │ (CLI)    │  │ (TUI)    │  │ /metrics, /api/metrics   │    │
│  └────┬─────┘  └────┬─────┘  └──────────────────────────┘    │
│       │              │                                       │
├───────┼──────────────┼───────────────────────────────────────┤
│       │   STORAGE LAYER (SQLite + WAL + FTS5)                │
│       │              │                                       │
│  ┌────▼──────────────▼────────────────────────────────────┐  │
│  │  traces │ spans │ memory_events │ tool_calls │ FTS5    │  │
│  │  pending_writes (crash recovery WAL)                   │  │
│  └────────────────────────▲───────────────────────────────┘  │
│                           │                                  │
├───────────────────────────┼──────────────────────────────────┤
│                    INGESTION LAYER                           │
│  ┌────────────────────────┼───────────────────────────────┐  │
│  │  TCP Listener → Wire Protocol → Batch Buffer → Flush   │  │
│  │  Channel buffers: span[2000], memory[2000], trace[1000]│  │
│  │  Flush: every 500ms OR 1000 items (whichever first)    │  │
│  └────────────────────────▲───────────────────────────────┘  │
│                           │                                  │
└───────────────────────────┼──────────────────────────────────┘
                            │ TCP (length-prefixed JSON)
┌───────────────────────────┼──────────────────────────────────┐
│  PYTHON SDK               │                                  │
│  ┌────────────────────────┘──────────────────────────────┐   │
│  │  OculoTracer → TraceContext → SpanContext             │   │
│  │  MemoryTracker (dict wrapper with mutation detection) │   │
│  │  OculoTransport (async TCP with background flush)     │   │
│  └───────────────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────────────┘
```

---

## Data Flow

### Ingestion Path (Write)

```
Agent Code
    │
    ▼
SpanContext.set_completion()     # Buffer data locally
    │
    ▼
OculoTransport._buffer (Queue)  # Thread-safe queue, never blocks agent
    │
    ▼ (background thread, every 500ms)
OculoTransport._flush()         # Serialize to wire format
    │
    ▼ TCP
DaemonIngester.handleConnection()  # Read wire messages
    │
    ▼
DaemonIngester.processMessage()    # Route by message type
    │
    ├─ MsgTrace  → traceChan     (buffered channel, size 1000)
    ├─ MsgSpan   → spanChan      (buffered channel, size 2000)
    └─ MsgMemory → memEventChan  (buffered channel, size 2000)
    │
    ▼ (flush goroutine, every 500ms OR 1000 items)
DBService.BatchInsertSpans()     # Single transaction
DBService.BatchInsertMemoryEvents()
    │
    ▼
SQLite (WAL mode)                # Concurrent read/write
```

### Query Path (Read)

```
TUI / CLI
    │
    ▼
DBService.QueryTimeline(traceID)        # Indexed on (trace_id, start_time)
DBService.GetMemoryDiffs(spanID)        # Indexed on (span_id, timestamp)
DBService.SearchContent("transformer")  # FTS5 with BM25 ranking
    │
    ▼
BubbleTea Model.Update()               # Render to terminal
```

---

## Storage Layer

### Schema Design

The schema is optimized for three access patterns:

1. **Timeline reconstruction:** Given a trace ID, show all spans ordered by time
2. **Memory diffing:** Given a span, show all memory mutations
3. **Content search:** Find spans by prompt/completion text

#### Core Tables

```sql
traces (trace_id PK, agent_name, start_time, end_time, status, metadata)
    │
    ▼ 1:N
spans (span_id PK, trace_id FK, parent_span_id, operation_type, ...)
    │
    ├─▶ 1:N memory_events (event_id PK, span_id FK, operation, key, old/new_value)
    └─▶ 1:N tool_calls (call_id PK, span_id FK, tool_name, args, result)
```

#### Index Strategy

| Index | Purpose | Query Pattern |
|-------|---------|---------------|
| `idx_spans_trace_time` | Timeline view | `WHERE trace_id = ? ORDER BY start_time` |
| `idx_spans_operation_type` | Type filtering | `WHERE operation_type = 'LLM'` |
| `idx_memory_events_span` | Diff view | `WHERE span_id = ? ORDER BY timestamp` |
| `idx_memory_events_key` | Key timeline | `WHERE key = ? ORDER BY timestamp` |
| `idx_traces_agent_time` | Trace listing | `WHERE agent_name = ? ORDER BY start_time DESC` |

#### FTS5 Configuration

```sql
CREATE VIRTUAL TABLE spans_fts USING fts5(
    span_id UNINDEXED,
    prompt, completion, operation_name,
    content='spans', content_rowid='rowid',
    tokenize='porter unicode61'
);
```

- Uses **Porter stemming** for English-language prompt/completion search
- Synchronized via INSERT/UPDATE/DELETE triggers
- Ranked by **BM25** relevance score

### WAL Mode

SQLite is configured with **Write-Ahead Logging** for concurrent read/write:

```sql
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA cache_size = -64000;  -- 64MB
```

This allows the TUI to read while the daemon writes, which is the primary
use case (daemon writes continuously, TUI reads on demand).

---

## Wire Protocol

Messages between the Python SDK and Go daemon use a length-prefixed binary format:

```
┌───────┬──────────┬─────────────────┐
│ Type  │ Length   │ Payload (JSON)  │
│ 1 byte│ 4 bytes  │ N bytes         │
│       │ big-end. │                 │
└───────┴──────────┴─────────────────┘
```

### Message Types

| Byte | Type | Description |
|------|------|-------------|
| `0x01` | TRACE | Create or update a trace |
| `0x02` | SPAN | Create or update a span |
| `0x03` | MEMORY_EVENT | Record a memory mutation |
| `0x04` | BATCH | Bundle of mixed types |

### ACK Protocol

After each message, the daemon sends a 1-byte ACK:
- `0x00`: Success
- `0x01`: Error

### Safety Limits

- Maximum message size: **10 MB**
- Maximum buffer size: **2000 messages** per type
- Overflow behavior: Direct insert (bypass buffer)

---

## Ingestion Pipeline

### Batch Strategy

The daemon uses a **two-tier buffering** strategy:

1. **Go channels** (in-memory): Fast, lock-free buffering
2. **SQLite transactions**: Batch commits for throughput

```
                   ┌─────────────┐
Incoming ────────▶ │ Go Channel  │ ────▶ Batch Buffer ────▶ SQLite TX
messages           │ (size 2000) │       (size 1000)        (COMMIT)
                   └─────────────┘
                         │
                    overflow?
                         │
                         ▼
                   Direct Insert
                   (bypass buffer)
```

### Flush Triggers

The flush goroutine commits batches when:
1. **Buffer reaches BatchSize** (default: 1000 items)
2. **FlushInterval elapses** (default: 500ms)

This ensures both high throughput under load and low latency under light load.

### Crash Recovery

The `pending_writes` table provides crash safety:

1. Before processing a batch, the raw payload is written to `pending_writes`
2. After successful commit, the pending entry is marked `committed`
3. On startup, the daemon replays any `pending` entries

---

## Analysis Engine

All analysis is **deterministic** — no LLMs involved.

### Token Hotspot Detection

Uses **Z-score** to identify outlier spans:

```
Z = (x - μ) / σ
```

Where:
- `x` = total tokens for a span
- `μ` = mean tokens across all LLM spans in the trace
- `σ` = standard deviation

Severity thresholds:
- Z > 1.5: Low
- Z > 2.0: Medium
- Z > 3.0: High

### Memory Growth Analysis

Uses **ordinary least squares regression** on cumulative key count:

```
y = mx + b

Where:
  y = number of keys in memory
  x = time (seconds since trace start)
  m = growth rate (keys/second)
  b = initial key count
```

**Unbounded growth detection:** If `slope > 0.1` AND `R² > 0.7`, the agent's memory is likely growing without bound.

### Cost Attribution

Uses a static pricing table for major LLM providers:

```go
var modelPricing = map[string][2]float64{
    "gpt-4":           {0.03, 0.06},    // [prompt $/1K, completion $/1K]
    "gpt-4-turbo":     {0.01, 0.03},
    "gpt-4o":          {0.005, 0.015},
    "claude-3-opus":   {0.015, 0.075},
    // ...
}
```

---

## TUI Rendering

### Layout

```
┌─ Title Bar (full width) ──────────────────────────────────────┐
├─ Timeline (40%) ──────────┬─ Detail (60%) ────────────────────┤
│  Span tree with icons     │  Metadata, tokens, prompt/compl.  │
│  and vim navigation       │  Token bar visualization          │
├─ Memory Diff (full width) ────────────────────────────────────┤
│  Unified diff with color  │                                    │
│  Red = deleted            │                                    │  
│  Green = added            │                                    │
│  Amber = updated          │                                    │
├─ Status Bar ──────────────────────────────────────────────────┤
│  Status message           │               Keybinding help     │
└───────────────────────────────────────────────────────────────┘
```

### Span Tree Construction

Spans are organized into a tree via `parent_span_id`:

```go
func buildSpanTree(spans []*Span) []spanNode {
    // 1. Build children lookup map
    // 2. DFS from root spans (parent_span_id = "")
    // 3. Assign depth levels for indentation
    // 4. Fallback: flat list if no root spans found
}
```

### Color Scheme

| Element | Color | Hex |
|---------|-------|-----|
| Primary | Indigo | `#7C3AED` |
| Secondary | Cyan | `#3DC2EC` |
| Added | Green | `#10B981` |
| Deleted | Red | `#EF4444` |
| Updated | Amber | `#F59E0B` |
| Background | Navy | `#0F172A` |
| Panel BG | Slate | `#1E293B` |
| Border | Gray | `#334155` |

---

## Design Decisions

### Why SQLite over PostgreSQL?

1. **Zero setup:** No database server to install
2. **Local-first:** Data never leaves the machine
3. **WAL mode:** Sufficient concurrency for 1 writer + N readers
4. **FTS5:** Built-in full-text search without external dependencies
5. **Portability:** Single file, easy to backup/share

### Why TCP over gRPC?

1. **Simplicity:** Length-prefixed JSON is trivial to implement in any language
2. **No codegen:** Python SDK doesn't need protobuf compilation
3. **Debuggability:** JSON payloads are human-readable
4. **Cross-platform:** TCP works everywhere (UDS doesn't work on Windows)

### Why JSON over Protobuf on the wire?

The `.proto` file defines the **canonical schema** and is used for documentation
and potential future gRPC endpoints. The wire format uses JSON for practical reasons:
- Python's `json` module is standard library (no dependencies)
- Debugging is easier with readable payloads
- The bottleneck is SQLite writes, not serialization

### Why Background Thread in Python SDK?

The SDK must **never block the agent's execution**. The background flush thread:
- Buffers messages in a thread-safe queue
- Flushes asynchronously every 500ms
- Drops messages if the buffer is full (graceful degradation)
- Uses daemon threads so the program can exit cleanly

---

## Performance Targets

| Metric | Target | Implementation |
|--------|--------|----------------|
| Ingestion throughput | >10K spans/sec | Batched transactions |
| Query latency | <100ms | Indexed queries |
| SDK overhead | <5% of agent runtime | Async transport |
| TUI frame rate | <50ms render | Incremental updates |
| Memory usage | <500MB for 10K traces | SQLite disk-backed |

---

## Future Work

1. **Protobuf wire format:** Switch to binary protobuf for 5-10x better serialization performance
2. **Embedding support:** Store vector embeddings in memory_events for semantic similarity
3. **P2P trace sharing:** Encrypted peer-to-peer trace exchange
4. **LangChain/CrewAI integrations:** Auto-instrumentation for popular frameworks
5. **Export formats:** Graphviz, D3.js, OpenTelemetry-compatible export
