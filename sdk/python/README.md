# Oculo Python SDK

Python instrumentation library for [Oculo](https://github.com/Mr-Dark-debug/oculo) — The Glass Box for AI Agents.

## Installation

```bash
pip install -e .
```

## Quick Start

```python
from oculo import OculoTracer

tracer = OculoTracer(agent_name="my-agent")

with tracer.trace() as trace:
    with trace.span("llm_call", operation_type="LLM") as span:
        span.set_prompt("What is AI?")
        result = call_llm(...)
        span.set_completion(result.text,
            prompt_tokens=result.usage.prompt_tokens,
            completion_tokens=result.usage.completion_tokens)

    with trace.span("update_memory", operation_type="MEMORY") as span:
        memory = span.memory_tracker(initial_state=agent.memory)
        memory["new_finding"] = "AI is transformative"
```

## Requirements

- Python 3.8+
- Oculo daemon running (`oculo-daemon`)
