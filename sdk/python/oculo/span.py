"""
Span — represents a single unit of work within an agent trace.

SpanContext is the object yielded by TraceContext.span() and provides
methods for recording LLM-specific data (prompt, completion, tokens)
and memory mutations within the span's scope.
"""

import uuid
import time
import json
import logging
from typing import Any, Dict, List, Optional

from oculo.transport import OculoTransport, MessageType

logger = logging.getLogger("oculo")


class Span:
    """
    Data class representing a completed span.
    
    This is used internally for constructing the span payload.
    """

    def __init__(
        self,
        span_id: str,
        trace_id: str,
        parent_span_id: Optional[str],
        operation_type: str,
        operation_name: str,
        start_time: int,
        duration_ms: int,
    ):
        self.span_id = span_id
        self.trace_id = trace_id
        self.parent_span_id = parent_span_id
        self.operation_type = operation_type
        self.operation_name = operation_name
        self.start_time = start_time
        self.duration_ms = duration_ms


class SpanContext:
    """
    Context for recording data within an active span.
    
    This is the object yielded by `TraceContext.span()` and provides
    a fluent API for setting LLM-specific fields and recording
    memory mutations.
    
    Attributes are buffered locally and sent to the daemon when
    the span's context manager exits.
    """

    def __init__(
        self,
        transport: OculoTransport,
        trace_id: str,
        span_id: str,
        parent_span_id: Optional[str],
        operation_name: str,
        operation_type: str,
        start_time: int,
        metadata: Optional[Dict[str, Any]] = None,
    ):
        self._transport = transport
        self.trace_id = trace_id
        self.span_id = span_id
        self.parent_span_id = parent_span_id
        self.operation_name = operation_name
        self.operation_type = operation_type
        self.start_time = start_time

        # LLM-specific fields
        self._prompt: Optional[str] = None
        self._completion: Optional[str] = None
        self._prompt_tokens: int = 0
        self._completion_tokens: int = 0
        self._model: Optional[str] = None
        self._temperature: Optional[float] = None

        # Status
        self._status: str = "ok"
        self._error_message: Optional[str] = None

        # Metadata
        self._metadata_json: Optional[str] = None
        if metadata:
            try:
                self._metadata_json = json.dumps(metadata, default=str)
            except (TypeError, ValueError):
                logger.warning("Failed to serialize span metadata")

        # Accumulated memory events
        self._memory_events: List[Dict[str, Any]] = []

    def set_prompt(self, prompt: str) -> "SpanContext":
        """
        Record the prompt sent to the LLM.
        
        Args:
            prompt: The full prompt text
        
        Returns:
            self for method chaining
        """
        self._prompt = prompt
        return self

    def set_completion(
        self,
        completion: str,
        prompt_tokens: int = 0,
        completion_tokens: int = 0,
    ) -> "SpanContext":
        """
        Record the LLM's completion and token usage.
        
        Args:
            completion: The full completion text
            prompt_tokens: Number of tokens in the prompt
            completion_tokens: Number of tokens in the completion
        
        Returns:
            self for method chaining
        """
        self._completion = completion
        self._prompt_tokens = prompt_tokens
        self._completion_tokens = completion_tokens
        return self

    def set_model(self, model: str, temperature: Optional[float] = None) -> "SpanContext":
        """
        Record the model and parameters used.
        
        Args:
            model: Model identifier (e.g., "gpt-4-turbo")
            temperature: Sampling temperature
        
        Returns:
            self for method chaining
        """
        self._model = model
        self._temperature = temperature
        return self

    def set_error(self, error: str) -> "SpanContext":
        """
        Mark this span as failed with an error message.
        
        Args:
            error: Error description
        
        Returns:
            self for method chaining
        """
        self._status = "error"
        self._error_message = error
        return self

    def add_tool_call(
        self,
        tool_name: str,
        arguments: Any = None,
        result: Any = None,
        success: bool = True,
        latency_ms: int = 0,
    ) -> "SpanContext":
        """
        Record a tool call made during this span.
        
        Args:
            tool_name: Name of the tool (e.g., "search_web")
            arguments: Tool arguments (will be JSON-serialized)
            result: Tool result (will be JSON-serialized)
            success: Whether the tool call succeeded
            latency_ms: Duration of the tool call in milliseconds
        
        Returns:
            self for method chaining
        """
        # Tool calls are recorded as metadata for now
        tool_data = {
            "tool_name": tool_name,
            "arguments_json": json.dumps(arguments, default=str) if arguments else None,
            "result_json": json.dumps(result, default=str) if result else None,
            "success": success,
            "latency_ms": latency_ms,
        }
        
        # Append to metadata
        if self._metadata_json:
            meta = json.loads(self._metadata_json)
        else:
            meta = {}
        
        if "tool_calls" not in meta:
            meta["tool_calls"] = []
        meta["tool_calls"].append(tool_data)
        self._metadata_json = json.dumps(meta, default=str)
        
        return self

    def record_memory_event(
        self,
        operation: str,
        key: str,
        old_value: Optional[str] = None,
        new_value: Optional[str] = None,
        namespace: str = "default",
    ) -> "SpanContext":
        """
        Record a single memory mutation event.
        
        This is the low-level API. For automatic diff tracking,
        use memory_tracker() instead.
        
        Args:
            operation: "ADD", "UPDATE", or "DELETE"
            key: The memory key being mutated
            old_value: Previous value (for UPDATE/DELETE)
            new_value: New value (for ADD/UPDATE)
            namespace: Memory namespace
        
        Returns:
            self for method chaining
        """
        event = {
            "event_id": str(uuid.uuid4()),
            "span_id": self.span_id,
            "timestamp": time.time_ns(),
            "operation": operation,
            "key": key,
            "old_value": old_value,
            "new_value": new_value,
            "namespace": namespace,
        }
        self._memory_events.append(event)
        return self

    def memory_tracker(
        self,
        initial_state: Optional[Dict[str, Any]] = None,
        namespace: str = "default",
    ) -> "MemoryTrackerBridge":
        """
        Create a memory tracker that automatically generates MemoryEvents.
        
        The tracker wraps a dictionary and intercepts all mutations,
        generating appropriate ADD/UPDATE/DELETE events.
        
        Args:
            initial_state: Starting state of the memory
            namespace: Memory namespace for grouping
        
        Returns:
            MemoryTrackerBridge that behaves like a dict
        """
        return MemoryTrackerBridge(
            span_context=self,
            initial_state=initial_state or {},
            namespace=namespace,
        )


class MemoryTrackerBridge:
    """
    Dictionary-like wrapper that generates MemoryEvents on mutation.
    
    This is the bridge between the dict interface and span memory events.
    It compares values before and after mutations to generate appropriate
    ADD, UPDATE, or DELETE events automatically.
    """

    def __init__(
        self,
        span_context: SpanContext,
        initial_state: Dict[str, Any],
        namespace: str = "default",
    ):
        self._ctx = span_context
        self._state = dict(initial_state)
        self._namespace = namespace

    def __getitem__(self, key: str) -> Any:
        return self._state[key]

    def __setitem__(self, key: str, value: Any) -> None:
        old_value = self._state.get(key)
        self._state[key] = value

        new_str = json.dumps(value, default=str) if not isinstance(value, str) else value

        if old_value is None:
            self._ctx.record_memory_event(
                operation="ADD",
                key=key,
                new_value=new_str,
                namespace=self._namespace,
            )
        else:
            old_str = json.dumps(old_value, default=str) if not isinstance(old_value, str) else old_value
            if old_str != new_str:
                self._ctx.record_memory_event(
                    operation="UPDATE",
                    key=key,
                    old_value=old_str,
                    new_value=new_str,
                    namespace=self._namespace,
                )

    def __delitem__(self, key: str) -> None:
        old_value = self._state.pop(key, None)
        old_str = json.dumps(old_value, default=str) if old_value is not None and not isinstance(old_value, str) else str(old_value) if old_value is not None else None
        
        self._ctx.record_memory_event(
            operation="DELETE",
            key=key,
            old_value=old_str,
            namespace=self._namespace,
        )

    def __contains__(self, key: str) -> bool:
        return key in self._state

    def __len__(self) -> int:
        return len(self._state)

    def get(self, key: str, default: Any = None) -> Any:
        return self._state.get(key, default)

    def keys(self):
        return self._state.keys()

    def values(self):
        return self._state.values()

    def items(self):
        return self._state.items()

    def update(self, data: Dict[str, Any]) -> None:
        """Update multiple keys, generating events for each change."""
        for key, value in data.items():
            self[key] = value

    def to_dict(self) -> Dict[str, Any]:
        """Return a plain dict copy of the current state."""
        return dict(self._state)
