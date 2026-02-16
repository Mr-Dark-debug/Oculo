"""
MemoryTracker — Automatic memory mutation detection for AI agents.

The MemoryTracker wraps a dictionary and intercepts all mutations,
computing diffs and generating MemoryEvents that are sent to the
Oculo daemon for visualization in the Glass Box TUI.

This module provides two approaches:
1. MemoryTracker: Standalone tracker that sends events directly via transport
2. SpanContext.memory_tracker(): Tracker that accumulates events within a span

The diffing function compares memory state snapshots before and after
operations to automatically generate ADD/UPDATE/DELETE events.
"""

import json
import uuid
import time
import copy
import logging
from typing import Any, Dict, List, Optional

from oculo.transport import OculoTransport, MessageType

logger = logging.getLogger("oculo")


class MemoryTracker:
    """
    Standalone memory tracker that sends mutations directly to the daemon.
    
    Use this when you want to track memory changes outside of a span context,
    or when the agent has a global memory store that persists across traces.
    
    Args:
        transport: Active OculoTransport for sending events
        initial_state: Starting state of the agent's memory
        namespace: Memory namespace for grouping related keys
        span_id: Optional span ID to associate events with
    
    Example:
        tracker = MemoryTracker(transport, initial_state={"goal": "research"})
        tracker["findings"] = "New discovery about transformers"
        tracker["goal"] = "publish results"  # Generates UPDATE event
        del tracker["old_data"]  # Generates DELETE event
    """

    def __init__(
        self,
        transport: OculoTransport,
        initial_state: Optional[Dict[str, Any]] = None,
        namespace: str = "default",
        span_id: Optional[str] = None,
    ):
        self._transport = transport
        self._state: Dict[str, Any] = dict(initial_state or {})
        self._namespace = namespace
        self._span_id = span_id or "standalone"
        self._event_count = 0

    def __getitem__(self, key: str) -> Any:
        return self._state[key]

    def __setitem__(self, key: str, value: Any) -> None:
        old_value = self._state.get(key)
        self._state[key] = value

        new_str = self._serialize(value)

        if old_value is None:
            self._emit_event("ADD", key, new_value=new_str)
        else:
            old_str = self._serialize(old_value)
            if old_str != new_str:
                self._emit_event("UPDATE", key, old_value=old_str, new_value=new_str)

    def __delitem__(self, key: str) -> None:
        if key not in self._state:
            raise KeyError(key)

        old_value = self._state.pop(key)
        old_str = self._serialize(old_value)
        self._emit_event("DELETE", key, old_value=old_str)

    def __contains__(self, key: str) -> bool:
        return key in self._state

    def __len__(self) -> int:
        return len(self._state)

    def __repr__(self) -> str:
        return f"MemoryTracker(namespace={self._namespace!r}, keys={len(self._state)}, events={self._event_count})"

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

    def clear(self) -> None:
        """Clear all keys, generating DELETE events for each."""
        for key in list(self._state.keys()):
            del self[key]

    def snapshot(self) -> Dict[str, Any]:
        """Return a deep copy of the current memory state."""
        return copy.deepcopy(self._state)

    def to_dict(self) -> Dict[str, Any]:
        """Return a shallow copy of the current state."""
        return dict(self._state)

    def _emit_event(
        self,
        operation: str,
        key: str,
        old_value: Optional[str] = None,
        new_value: Optional[str] = None,
    ) -> None:
        """Send a memory event to the daemon via the transport."""
        event = {
            "event_id": str(uuid.uuid4()),
            "span_id": self._span_id,
            "timestamp": time.time_ns(),
            "operation": operation,
            "key": key,
            "old_value": old_value,
            "new_value": new_value,
            "namespace": self._namespace,
        }
        self._transport.send(MessageType.MEMORY_EVENT, event)
        self._event_count += 1

        logger.debug(
            "Memory %s: %s.%s %s",
            operation, self._namespace, key,
            f"({old_value[:30]}... → {new_value[:30]}...)" if operation == "UPDATE" and old_value and new_value
            else f"= {new_value[:50]}..." if new_value
            else f"(was: {old_value[:50]}...)" if old_value
            else "",
        )

    @staticmethod
    def _serialize(value: Any) -> str:
        """Serialize a value to a string for storage."""
        if isinstance(value, str):
            return value
        try:
            return json.dumps(value, default=str, ensure_ascii=False)
        except (TypeError, ValueError):
            return str(value)


def compute_memory_diff(
    before: Dict[str, Any],
    after: Dict[str, Any],
    namespace: str = "default",
    span_id: Optional[str] = None,
) -> List[Dict[str, Any]]:
    """
    Compute the diff between two memory state snapshots.
    
    This is the core diffing function used to automatically generate
    MemoryEvents when comparing the agent's memory before and after
    an operation (e.g., LLM call, tool use).
    
    Args:
        before: Memory state before the operation
        after: Memory state after the operation
        namespace: Memory namespace
        span_id: Span to associate events with
    
    Returns:
        List of MemoryEvent dicts ready for transport
    
    Example:
        before = {"goal": "research", "findings": []}
        after = {"goal": "research", "findings": ["transformer paper"]}
        events = compute_memory_diff(before, after)
        # Returns: [{"operation": "UPDATE", "key": "findings", ...}]
    """
    events = []
    sid = span_id or "diff"
    all_keys = set(list(before.keys()) + list(after.keys()))

    for key in sorted(all_keys):
        before_val = before.get(key)
        after_val = after.get(key)

        before_str = MemoryTracker._serialize(before_val) if before_val is not None else None
        after_str = MemoryTracker._serialize(after_val) if after_val is not None else None

        if before_val is None and after_val is not None:
            events.append({
                "event_id": str(uuid.uuid4()),
                "span_id": sid,
                "timestamp": time.time_ns(),
                "operation": "ADD",
                "key": key,
                "old_value": None,
                "new_value": after_str,
                "namespace": namespace,
            })
        elif before_val is not None and after_val is None:
            events.append({
                "event_id": str(uuid.uuid4()),
                "span_id": sid,
                "timestamp": time.time_ns(),
                "operation": "DELETE",
                "key": key,
                "old_value": before_str,
                "new_value": None,
                "namespace": namespace,
            })
        elif before_str != after_str:
            events.append({
                "event_id": str(uuid.uuid4()),
                "span_id": sid,
                "timestamp": time.time_ns(),
                "operation": "UPDATE",
                "key": key,
                "old_value": before_str,
                "new_value": after_str,
                "namespace": namespace,
            })

    return events
