"""
OculoTracer — Main entry point for instrumenting AI agents.

The tracer manages trace lifecycle and provides context managers
for creating spans with automatic timing and memory tracking.

Usage:
    tracer = OculoTracer(agent_name="research-agent")
    with tracer.trace() as trace:
        with trace.span("llm_call", operation_type="LLM") as span:
            span.set_prompt("Analyze this paper...")
            result = call_llm(...)
            span.set_completion(result)
        
        with trace.span("memory_update", operation_type="MEMORY") as span:
            tracker = span.memory_tracker(current_state)
            tracker["findings"] = "New discovery"
"""

import uuid
import time
import logging
from typing import Any, Dict, Optional
from contextlib import contextmanager

from oculo.transport import OculoTransport, MessageType
from oculo.span import Span, SpanContext
from oculo.memory import MemoryTracker

logger = logging.getLogger("oculo")


class OculoTracer:
    """
    Main tracer class for instrumenting AI agents with Oculo.
    
    Creates traces and spans, manages the transport connection,
    and provides helpers for memory tracking and LLM call instrumentation.
    
    Args:
        agent_name: Name identifying this agent (e.g., "research-agent-v2")
        host: Oculo daemon TCP host (default: "127.0.0.1")
        port: Oculo daemon TCP port (default: 9876)
        auto_start: Whether to start the transport immediately (default: True)
        metadata: Additional metadata to attach to all traces
    """

    def __init__(
        self,
        agent_name: str,
        host: str = "127.0.0.1",
        port: int = 9876,
        auto_start: bool = True,
        metadata: Optional[Dict[str, str]] = None,
    ):
        self.agent_name = agent_name
        self.metadata = metadata or {}
        self.transport = OculoTransport(host=host, port=port)

        if auto_start:
            self.transport.start()

    def close(self) -> None:
        """Flush remaining data and close the transport."""
        self.transport.stop()

    def __enter__(self):
        return self

    def __exit__(self, *args):
        self.close()

    @contextmanager
    def trace(
        self,
        trace_id: Optional[str] = None,
        metadata: Optional[Dict[str, str]] = None,
    ):
        """
        Create a new trace context.
        
        A trace represents a complete execution of the agent, from
        initial prompt to final output. All spans created within
        this context are associated with this trace.
        
        Args:
            trace_id: Optional explicit trace ID (auto-generated if not provided)
            metadata: Additional metadata for this trace
        
        Yields:
            TraceContext for creating spans within this trace.
        
        Example:
            with tracer.trace() as t:
                with t.span("step_1", operation_type="LLM") as s:
                    ...
        """
        tid = trace_id or str(uuid.uuid4())
        start_time = time.time_ns()
        
        merged_metadata = {**self.metadata, **(metadata or {})}

        # Send trace start
        trace_data = {
            "trace_id": tid,
            "agent_name": self.agent_name,
            "start_time": start_time,
            "status": "running",
            "metadata": merged_metadata,
        }
        self.transport.send(MessageType.TRACE, trace_data)

        ctx = TraceContext(
            tracer=self,
            trace_id=tid,
            metadata=merged_metadata,
        )

        try:
            yield ctx
            status = "completed"
        except Exception as e:
            status = "failed"
            logger.error("Trace %s failed: %s", tid, e)
            raise
        finally:
            # Send trace end
            end_data = {
                "trace_id": tid,
                "agent_name": self.agent_name,
                "start_time": start_time,
                "end_time": time.time_ns(),
                "status": status,
                "metadata": merged_metadata,
            }
            self.transport.send(MessageType.TRACE, end_data)

    def memory_tracker(
        self,
        initial_state: Optional[Dict[str, Any]] = None,
        namespace: str = "default",
    ) -> MemoryTracker:
        """
        Create a standalone memory tracker for monitoring dict changes.
        
        Args:
            initial_state: Starting state of the agent's memory
            namespace: Memory namespace for grouping related keys
        
        Returns:
            MemoryTracker that automatically logs mutations.
        """
        return MemoryTracker(
            transport=self.transport,
            initial_state=initial_state,
            namespace=namespace,
        )


class TraceContext:
    """
    Context for creating spans within a trace.
    
    TraceContext is yielded by OculoTracer.trace() and provides
    methods for creating child spans with automatic timing.
    """

    def __init__(self, tracer: OculoTracer, trace_id: str, metadata: Dict[str, str]):
        self.tracer = tracer
        self.trace_id = trace_id
        self.metadata = metadata
        self._span_stack: list = []

    @contextmanager
    def span(
        self,
        operation_name: str,
        operation_type: str = "LLM",
        parent_span_id: Optional[str] = None,
        metadata: Optional[Dict[str, Any]] = None,
    ):
        """
        Create a new span within this trace.
        
        Spans represent individual operations like LLM calls, tool
        invocations, or memory mutations. They can be nested to
        show causal relationships.
        
        Args:
            operation_name: Human-readable name (e.g., "search_web")
            operation_type: One of "LLM", "TOOL", "MEMORY", "PLANNING", "RETRIEVAL"
            parent_span_id: Optional parent span for nesting
            metadata: Additional metadata JSON
        
        Yields:
            SpanContext with methods for setting prompt, completion, etc.
        
        Example:
            with trace.span("gpt4_call", operation_type="LLM") as s:
                s.set_prompt("Tell me about transformers")
                result = call_gpt4(...)
                s.set_completion(result, prompt_tokens=100, completion_tokens=50)
        """
        span_id = str(uuid.uuid4())
        start_time = time.time_ns()

        # Auto-detect parent from span stack
        if parent_span_id is None and self._span_stack:
            parent_span_id = self._span_stack[-1]

        ctx = SpanContext(
            transport=self.tracer.transport,
            trace_id=self.trace_id,
            span_id=span_id,
            parent_span_id=parent_span_id,
            operation_name=operation_name,
            operation_type=operation_type,
            start_time=start_time,
            metadata=metadata,
        )

        self._span_stack.append(span_id)

        try:
            yield ctx
            ctx._status = "ok"
        except Exception as e:
            ctx._status = "error"
            ctx._error_message = str(e)
            raise
        finally:
            self._span_stack.pop()
            
            duration_ms = (time.time_ns() - start_time) // 1_000_000

            span_data = {
                "span_id": span_id,
                "trace_id": self.trace_id,
                "parent_span_id": parent_span_id,
                "operation_type": operation_type,
                "operation_name": operation_name,
                "start_time": start_time,
                "duration_ms": duration_ms,
                "prompt": ctx._prompt,
                "completion": ctx._completion,
                "prompt_tokens": ctx._prompt_tokens,
                "completion_tokens": ctx._completion_tokens,
                "model": ctx._model,
                "temperature": ctx._temperature,
                "metadata": ctx._metadata_json,
                "status": ctx._status,
                "error_message": ctx._error_message,
            }
            self.tracer.transport.send(MessageType.SPAN, span_data)

            # Send any accumulated memory events
            for event in ctx._memory_events:
                self.tracer.transport.send(MessageType.MEMORY_EVENT, event)
