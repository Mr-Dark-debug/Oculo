![Oculo Banner](banner.svg)

<p align="center">
  <strong>Runtime debugging for AI agents.</strong><br>
  <sub>Traces. Memory diffs. Token analysis. All local.</sub>
</p>

<p align="center">
  <a href="#install">Install</a> &middot;
  <a href="#quick-start">Quick Start</a> &middot;
  <a href="#sdk-reference">SDK</a> &middot;
  <a href="#architecture">Architecture</a>
</p>

---

## Install

**One-line install** (Linux / macOS):

```bash
curl -fsSL https://raw.githubusercontent.com/Mr-Dark-debug/Oculo/main/install.sh | sh
```

**With Go** (requires Go 1.21+ and a C compiler for CGo):

```bash
go install -tags fts5 github.com/Mr-Dark-debug/oculo/cmd/oculo@latest
go install -tags fts5 github.com/Mr-Dark-debug/oculo/cmd/oculo-daemon@latest
go install -tags fts5 github.com/Mr-Dark-debug/oculo/cmd/oculo-tui@latest
```

**From source:**

```bash
git clone https://github.com/Mr-Dark-debug/Oculo.git
cd Oculo
make build
```

**Python SDK:**

```bash
pip install oculo-sdk
```

---

## Quick Start

**1. Start the daemon:**

```bash
oculo-daemon
```

The daemon listens on `127.0.0.1:9876` and stores traces in `~/.oculo/oculo.db`.

**2. Instrument your agent:**

```python
from oculo import OculoTracer

tracer = OculoTracer(agent_name="my-agent")

with tracer.trace() as t:
    with t.span("llm_call") as s:
        s.set_prompt("What is the capital of France?")
        # ... call your LLM ...
        s.set_completion("The capital of France is Paris.")
        s.set_tokens(prompt=12, completion=8)
        s.set_model("gpt-4")
```

**3. Open the debugger:**

```bash
oculo-tui
```

**4. Run analysis:**

```bash
oculo analyze <trace-id>
oculo analyze <trace-id> --format markdown
```

---

## What You Get

| Feature | Description |
|---|---|
| **Span Timeline** | Tree view of every operation your agent performs |
| **Memory Diffs** | Unified diff view of memory mutations across spans |
| **Token Analysis** | Per-span and trace-level token usage with visual bars |
| **Anomaly Detection** | Z-score based token hotspot detection |
| **Memory Growth** | Linear regression to detect unbounded state accumulation |
| **Cost Attribution** | Per-model cost breakdown using configurable pricing |
| **Full-Text Search** | FTS5-powered search over prompts and completions |
| **Crash Recovery** | WAL journal with pending write recovery |

---

## SDK Reference

### Tracer

```python
from oculo import OculoTracer

tracer = OculoTracer(
    agent_name="research-bot",
    host="127.0.0.1",        # daemon host
    port=9876,                # daemon port
    buffer_size=1000,         # max buffered events
    flush_interval=0.5,       # seconds between flushes
)
```

### Spans

```python
with tracer.trace() as t:
    with t.span("llm_call") as s:
        s.set_prompt("...")
        s.set_completion("...")
        s.set_tokens(prompt=100, completion=50)
        s.set_model("gpt-4")

        # Tool calls
        s.record_tool_call(
            tool_name="search",
            arguments={"query": "climate change"},
            result={"hits": 42},
            success=True,
            latency_ms=120,
        )
```

### Memory Tracking

```python
from oculo import OculoTracer, compute_memory_diff

tracer = OculoTracer(agent_name="agent")

with tracer.trace() as t:
    with t.span("reasoning") as s:
        before = {"goal": "research", "steps": 3}
        after  = {"goal": "research", "steps": 5, "status": "active"}

        diff = compute_memory_diff(before, after)
        for op, key, old_val, new_val in diff:
            s.record_memory_mutation(
                key=key, operation=op,
                old_value=old_val, new_value=new_val,
            )
```

---

## CLI Commands

```
oculo analyze <trace-id>            Semantic analysis with anomaly detection
oculo analyze <trace-id> -f md      Markdown formatted report
oculo query traces                  List recent traces
oculo query timeline <trace-id>     Show span timeline
oculo status                        Check daemon connectivity
oculo version                       Print version info
```

---

## TUI Keyboard Shortcuts

| Key | Action |
|---|---|
| `↑` `↓` / `j` `k` | Navigate spans / traces |
| `Tab` / `Shift+Tab` | Switch panes |
| `Enter` | Select trace / expand |
| `/` | Search |
| `d` | Toggle diff view |
| `Esc` | Back to trace list |
| `q` | Quit |

---

## Project Structure

```
oculo/
├── cmd/
│   ├── oculo/            CLI tool (analyze, query, status)
│   ├── oculo-daemon/     Ingestion daemon (TCP server)
│   └── oculo-tui/        Terminal debugger (BubbleTea)
├── internal/
│   ├── analysis/         Z-score, regression, cost analysis
│   ├── database/         SQLite + WAL + FTS5 storage
│   ├── ingestion/        TCP server + batch pipeline
│   ├── protocol/         Wire protocol definitions
│   └── tui/              Component-based UI
│       ├── model.go      Root model + Update logic
│       ├── theme.go      Centralized colors + styles
│       ├── header.go     Header bar + footer
│       ├── timeline.go   Span tree component
│       ├── detail.go     Span detail + token bars
│       ├── diffview.go   Memory diff viewer
│       ├── tracelist.go  Trace selector
│       └── helpers.go    Tree building, utilities
├── pkg/
│   ├── jsonutil/         JSON diffing + helpers
│   └── timeutil/         Time formatting
├── sdk/python/oculo/     Python SDK
├── examples/             Sample instrumented agent
├── install.sh            One-line installer
├── banner.svg            README banner
├── Makefile              Build system
└── ARCHITECTURE.md       Technical design document
```

---

## Configuration

| Variable | Default | Description |
|---|---|---|
| `--listen` | `127.0.0.1:9876` | Daemon listen address |
| `--db` | `~/.oculo/oculo.db` | SQLite database path |
| `--metrics` | `127.0.0.1:9877` | Prometheus metrics endpoint |
| `--batch` | `1000` | Batch flush size |
| `OCULO_INSTALL_DIR` | `~/.local/bin` | Installer target directory |
| `OCULO_VERSION` | `latest` | Version for installer |

---

## Build from Source

```bash
# Prerequisites: Go 1.21+, GCC (for CGo/SQLite)

make build          # Build all binaries → bin/
make test           # Run all tests
make bench          # Run benchmarks
make lint           # Run go vet + linters
make install        # Install to $GOPATH/bin
make install-sdk    # Install Python SDK
make clean          # Remove build artifacts
```

> **Note:** The `-tags fts5` build flag is required for SQLite full-text search.
> The Makefile handles this automatically.

---

## License

MIT

---

<p align="center">
  <sub>Built for engineers who debug AI agents, not decorate terminals.</sub>
</p>