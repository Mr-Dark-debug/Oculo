"""
Oculo Python SDK — Non-blocking instrumentation for AI agents.

The Oculo SDK provides low-overhead tracing for AI agents, capturing
LLM calls, tool invocations, and memory mutations. All data is sent
asynchronously to the local Oculo daemon over TCP.

Quick Start:
    >>> from oculo import OculoTracer
    >>> tracer = OculoTracer(agent_name="my-agent")
    >>> with tracer.trace() as trace:
    ...     with trace.span("llm_call", operation_type="LLM") as span:
    ...         span.set_prompt("What is the capital of France?")
    ...         result = my_llm_call(...)
    ...         span.set_completion(result, prompt_tokens=10, completion_tokens=8)

Memory Tracking:
    >>> tracker = tracer.memory_tracker(initial_state={"key": "value"})
    >>> tracker["key"] = "new_value"  # Automatically generates a MemoryEvent
"""

from oculo.tracer import OculoTracer
from oculo.span import Span, SpanContext
from oculo.memory import MemoryTracker
from oculo.transport import OculoTransport

__version__ = "0.1.0"
__all__ = ["OculoTracer", "Span", "SpanContext", "MemoryTracker", "OculoTransport"]
