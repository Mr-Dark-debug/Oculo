# Oculo Python SDK

![Oculo Banner](https://raw.githubusercontent.com/Mr-Dark-debug/oculo/main/banner.svg)

<p align="center">
  <a href="https://pypi.org/project/oculo-sdk/">
    <img src="https://img.shields.io/pypi/v/oculo-sdk.svg" alt="PyPI version">
  </a>
</p>

**Oculo** is a "Glass Box" for AI agents—a comprehensive observability tool that lets you see inside your agent's cognition. This SDK provides the Python instrumentation to capture traces, spans, and memory mutations and send them to the Oculo daemon.

> **Note**: This SDK requires the `oculo-daemon` to be running locally. Please refer to the [main Oculo repository](https://github.com/Mr-Dark-debug/oculo) for daemon installation instructions.

## 📦 Installation

You can install the Oculo SDK directly from PyPI:

```bash
pip install oculo-sdk
```

Or install the latest development version from source:

```bash
pip install git+https://github.com/Mr-Dark-debug/oculo.git#subdirectory=sdk/python
```

Or, if developing locally:

```bash
cd sdk/python
pip install -e .
```

## 🚀 Quick Start

Instrumenting your agent is simple. Wrap your agent's execution in a `trace`, and wrap individual steps (like LLM calls or tool usage) in a `span`.

```python
from oculo import OculoTracer

# 1. Initialize the tracer
tracer = OculoTracer(agent_name="research-agent-v1")

# 2. Start a trace
with tracer.trace() as t:
    
    # 3. Create a span for an LLM call
    with t.span("step_1_planning", operation_type="LLM") as s:
        prompt = "Plan a research trip to Mars."
        s.set_prompt(prompt)
        
        # ... call your LLM ...
        response = my_llm_function(prompt)
        
        s.set_completion(
            response.text, 
            prompt_tokens=response.usage.prompt_tokens, 
            completion_tokens=response.usage.completion_tokens
        )
        
    # 4. Track memory changes
    with t.span("update_memory", operation_type="MEMORY") as s:
        # Create a tracker wrapper around your agent's memory dict
        memory = s.memory_tracker(initial_state=agent.memory)
        
        # Mutations are automatically detected and logged!
        memory["current_goal"] = "Calculate trajectory"
        memory["constraints"].append("Fuel limit")
```

## 📚 Core Concepts

### Traces (`tracer.trace()`)
A **Trace** represents a complete execution workflow of your agent, from the initial user prompt to the final answer. It groups all subsequent operations together.

### Spans (`trace.span()`)
A **Span** represents a single unit of work. Spans can be nested to show causal relationships (e.g., a "CoT" span containing multiple "LLM" spans).

Supported `operation_type` values:
- `LLM`: Large Language Model calls
- `TOOL`: External tool invocations (search, calculator, etc.)
- `MEMORY`: Memory read/write operations
- `PLANNING`: Planning or reasoning steps
- `RETRIEVAL`: RAG or database lookups

### Memory Tracking (`span.memory_tracker()`)
Oculo excels at visualizing how your agent's state changes over time. The `MemoryTracker` wraps a standard Python dictionary and automatically records `ADD`, `UPDATE`, and `DELETE` events whenever you modify it.

```python
# Automatic diffing
tracker = span.memory_tracker(agent.state)
tracker["status"] = "thinking"  # -> Generates an UPDATE event
```

## 🔧 Configuration

The `OculoTracer` can be configured with the following parameters:

```python
tracer = OculoTracer(
    agent_name="my-agent",
    host="127.0.0.1",       # Daemon host
    port=9876,              # Daemon port
    auto_start=True,        # Connect immediately
    metadata={"env": "prod"} # Global tags
)
```

## 🤝 Contributing

We welcome contributions! Please see the [main repository](https://github.com/Mr-Dark-debug/oculo) for contribution guidelines.

## 📄 License

MIT License. See [LICENSE](https://github.com/Mr-Dark-debug/oculo/blob/main/LICENSE) for details.
