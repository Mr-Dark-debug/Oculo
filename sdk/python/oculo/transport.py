"""
Transport layer for communicating with the Oculo daemon.

Uses a non-blocking, buffered TCP connection to send trace data
to the Go daemon. Messages use a length-prefixed JSON wire format:
  [1 byte type][4 bytes length (big-endian)][JSON payload]

The transport runs a background thread that flushes buffered data
at regular intervals, ensuring that the agent's execution thread
is never blocked by I/O operations.
"""

import json
import queue
import socket
import struct
import threading
import logging
import time
from typing import Any, Dict, Optional

logger = logging.getLogger("oculo")


class MessageType:
    """Wire protocol message type identifiers."""
    TRACE = 0x01
    SPAN = 0x02
    MEMORY_EVENT = 0x03
    BATCH = 0x04


class OculoTransport:
    """
    Non-blocking transport to the Oculo daemon.
    
    Buffers messages in a thread-safe queue and flushes them
    to the daemon asynchronously via a background thread.
    
    Args:
        host: Daemon TCP host (default: "127.0.0.1")
        port: Daemon TCP port (default: 9876)
        flush_interval: Seconds between automatic flushes (default: 0.5)
        max_buffer_size: Maximum messages to buffer before force-flush (default: 1000)
        connect_timeout: Socket connection timeout in seconds (default: 5.0)
    """

    def __init__(
        self,
        host: str = "127.0.0.1",
        port: int = 9876,
        flush_interval: float = 0.5,
        max_buffer_size: int = 1000,
        connect_timeout: float = 5.0,
    ):
        self.host = host
        self.port = port
        self.flush_interval = flush_interval
        self.max_buffer_size = max_buffer_size
        self.connect_timeout = connect_timeout

        self._buffer: queue.Queue = queue.Queue(maxsize=max_buffer_size * 2)
        self._socket: Optional[socket.socket] = None
        self._lock = threading.Lock()
        self._running = False
        self._flush_thread: Optional[threading.Thread] = None
        self._connected = False

        # Metrics
        self.messages_sent = 0
        self.messages_dropped = 0
        self.errors = 0

    def start(self) -> None:
        """Start the background flush thread and connect to the daemon."""
        if self._running:
            return

        self._running = True
        self._connect()

        self._flush_thread = threading.Thread(
            target=self._flush_loop,
            name="oculo-transport",
            daemon=True,  # Don't prevent program exit
        )
        self._flush_thread.start()
        logger.info("Oculo transport started (target: %s:%d)", self.host, self.port)

    def stop(self) -> None:
        """Stop the transport, flushing remaining data."""
        self._running = False
        
        # Final flush
        self._flush()
        
        if self._flush_thread:
            self._flush_thread.join(timeout=2.0)

        self._disconnect()
        logger.info(
            "Oculo transport stopped (sent: %d, dropped: %d, errors: %d)",
            self.messages_sent, self.messages_dropped, self.errors,
        )

    def send(self, msg_type: int, data: Dict[str, Any]) -> None:
        """
        Queue a message for async delivery to the daemon.
        
        This method is designed to NEVER block the caller's thread.
        If the buffer is full, the message is dropped with a warning.
        
        Args:
            msg_type: MessageType constant
            data: JSON-serializable dictionary
        """
        try:
            self._buffer.put_nowait((msg_type, data))
        except queue.Full:
            self.messages_dropped += 1
            logger.warning("Oculo buffer full — dropping message (total dropped: %d)", self.messages_dropped)

    def send_batch(self, traces=None, spans=None, memory_events=None, tool_calls=None) -> None:
        """
        Queue a batch message containing multiple items.
        
        Args:
            traces: List of trace dicts
            spans: List of span dicts
            memory_events: List of memory event dicts
            tool_calls: List of tool call dicts
        """
        batch = {}
        if traces:
            batch["traces"] = traces
        if spans:
            batch["spans"] = spans
        if memory_events:
            batch["memory_events"] = memory_events
        if tool_calls:
            batch["tool_calls"] = tool_calls

        self.send(MessageType.BATCH, batch)

    def _connect(self) -> bool:
        """Establish TCP connection to the daemon."""
        with self._lock:
            if self._connected:
                return True

            try:
                self._socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
                self._socket.settimeout(self.connect_timeout)
                self._socket.connect((self.host, self.port))
                self._connected = True
                logger.debug("Connected to Oculo daemon at %s:%d", self.host, self.port)
                return True
            except (socket.error, OSError) as e:
                logger.warning("Cannot connect to Oculo daemon: %s", e)
                self._connected = False
                self._socket = None
                return False

    def _disconnect(self) -> None:
        """Close the socket connection."""
        with self._lock:
            if self._socket:
                try:
                    self._socket.close()
                except Exception:
                    pass
                self._socket = None
                self._connected = False

    def _flush_loop(self) -> None:
        """Background thread that periodically flushes the buffer."""
        while self._running:
            time.sleep(self.flush_interval)
            self._flush()

    def _flush(self) -> None:
        """Send all buffered messages to the daemon."""
        messages = []
        while not self._buffer.empty():
            try:
                messages.append(self._buffer.get_nowait())
            except queue.Empty:
                break

        if not messages:
            return

        # Ensure we have a connection
        if not self._connected:
            if not self._connect():
                # Can't connect — re-queue messages (up to limit)
                for msg in messages[:self.max_buffer_size]:
                    try:
                        self._buffer.put_nowait(msg)
                    except queue.Full:
                        self.messages_dropped += 1
                return

        for msg_type, data in messages:
            try:
                self._send_wire_message(msg_type, data)
                self.messages_sent += 1
            except Exception as e:
                self.errors += 1
                logger.debug("Failed to send message: %s", e)
                # Reconnect on next flush
                self._disconnect()
                break

    def _send_wire_message(self, msg_type: int, data: Dict[str, Any]) -> None:
        """
        Send a single wire message over the socket.
        
        Wire format: [1 byte type][4 bytes length (big-endian)][JSON payload]
        """
        payload = json.dumps(data, default=str).encode("utf-8")
        header = struct.pack(">BI", msg_type, len(payload))

        with self._lock:
            if not self._socket:
                raise ConnectionError("Not connected to daemon")

            self._socket.sendall(header + payload)

            # Read ACK (1 byte)
            ack = self._socket.recv(1)
            if not ack or ack[0] != 0x00:
                raise RuntimeError(f"Daemon returned error ACK: {ack}")

    @property
    def is_connected(self) -> bool:
        """Whether the transport is currently connected to the daemon."""
        return self._connected
