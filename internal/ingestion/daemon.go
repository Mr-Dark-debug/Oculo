// Package ingestion implements the high-throughput, crash-safe ingestion
// service for Oculo. It receives trace data over a TCP connection (or
// named pipe on Windows), buffers incoming data, and batches writes
// to the SQLite database for optimal throughput.
//
// Architecture:
//   Client (Python SDK) → TCP/Named Pipe → Ingester → Batch Buffer → DBService
//
// The ingester uses a buffered channel and periodic flush to batch writes,
// committing every 500ms or 1000 records (whichever comes first).
// A write-ahead log in SQLite ensures crash safety.
package ingestion

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Mr-Dark-debug/oculo/internal/database"
)

// Ingester defines the interface for the ingestion service.
// This abstraction allows for mocking in integration tests.
type Ingester interface {
	// Start begins listening for incoming trace data.
	Start(ctx context.Context) error
	// Stop gracefully shuts down the ingester, flushing remaining data.
	Stop() error
	// Metrics returns the current ingestion metrics.
	Metrics() IngestionMetrics
}

// IngestionMetrics tracks throughput and error rates.
type IngestionMetrics struct {
	TracesIngested   int64 `json:"traces_ingested"`
	SpansIngested    int64 `json:"spans_ingested"`
	MemoryEvents     int64 `json:"memory_events"`
	ErrorCount       int64 `json:"error_count"`
	BatchesCommitted int64 `json:"batches_committed"`
	Uptime           int64 `json:"uptime_seconds"`
}

// Config holds configuration for the ingestion daemon.
type Config struct {
	// ListenAddr is the TCP address or named pipe path to listen on.
	// On Unix: use a path like "/tmp/oculo.sock" for UDS
	// On Windows: use "127.0.0.1:9876" for TCP
	ListenAddr string `json:"listen_addr"`

	// DBPath is the path to the SQLite database file.
	DBPath string `json:"db_path"`

	// MetricsAddr is the HTTP address for Prometheus metrics.
	// Empty string disables the metrics server.
	MetricsAddr string `json:"metrics_addr"`

	// BatchSize is the maximum number of items to batch before flushing.
	BatchSize int `json:"batch_size"`

	// FlushInterval is the maximum time between batch flushes.
	FlushInterval time.Duration `json:"flush_interval"`
}

// DefaultConfig returns sensible defaults for the ingestion daemon.
func DefaultConfig() Config {
	listenAddr := "127.0.0.1:9876"
	if runtime.GOOS != "windows" {
		listenAddr = "/tmp/oculo.sock"
	}

	homeDir, _ := os.UserHomeDir()
	dbPath := filepath.Join(homeDir, ".oculo", "oculo.db")

	return Config{
		ListenAddr:    listenAddr,
		DBPath:        dbPath,
		MetricsAddr:   "127.0.0.1:9877",
		BatchSize:     1000,
		FlushInterval: 500 * time.Millisecond,
	}
}

// ============================================================
// Wire Protocol
// ============================================================

// MessageType discriminates the kind of payload in the wire protocol.
type MessageType byte

const (
	MsgTrace       MessageType = 0x01
	MsgSpan        MessageType = 0x02
	MsgMemoryEvent MessageType = 0x03
	MsgBatch       MessageType = 0x04
)

// WireMessage is the envelope for data sent over the socket.
// Format: [1 byte type][4 bytes length (big-endian)][payload JSON]
type WireMessage struct {
	Type    MessageType `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// BatchMessage contains multiple items of different types.
type BatchMessage struct {
	Traces       []*database.Trace       `json:"traces,omitempty"`
	Spans        []*database.Span        `json:"spans,omitempty"`
	MemoryEvents []*database.MemoryEvent `json:"memory_events,omitempty"`
	ToolCalls    []*database.ToolCall    `json:"tool_calls,omitempty"`
}

// ============================================================
// DaemonIngester Implementation
// ============================================================

// DaemonIngester is the production implementation of the Ingester interface.
// It manages the network listener, batch buffer, and flush goroutine.
type DaemonIngester struct {
	config  Config
	store   database.Store
	metrics IngestionMetrics

	// Channels for buffered ingestion
	spanChan        chan *database.Span
	memoryEventChan chan *database.MemoryEvent
	traceChan       chan *database.Trace

	listener net.Listener
	mu       sync.RWMutex
	wg       sync.WaitGroup
	started  time.Time

	cancel context.CancelFunc
	done   chan struct{}
}

// NewDaemonIngester creates a new ingestion daemon with the given configuration.
func NewDaemonIngester(config Config, store database.Store) *DaemonIngester {
	return &DaemonIngester{
		config:          config,
		store:           store,
		spanChan:        make(chan *database.Span, config.BatchSize*2),
		memoryEventChan: make(chan *database.MemoryEvent, config.BatchSize*2),
		traceChan:       make(chan *database.Trace, config.BatchSize),
		done:            make(chan struct{}),
	}
}

// Start begins listening for incoming connections and starts the batch
// flush goroutine. It also replays any pending writes from a previous crash.
func (d *DaemonIngester) Start(ctx context.Context) error {
	d.started = time.Now()

	// Replay pending writes from crash recovery
	if err := d.replayPending(); err != nil {
		log.Printf("[WARN] Failed to replay pending writes: %v", err)
	}

	// Determine network type based on platform
	network := "tcp"
	if runtime.GOOS != "windows" {
		network = "unix"
		// Remove stale socket file
		os.Remove(d.config.ListenAddr)
	}

	listener, err := net.Listen(network, d.config.ListenAddr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", d.config.ListenAddr, err)
	}
	d.listener = listener

	ctx, d.cancel = context.WithCancel(ctx)

	// Start batch flush goroutine
	d.wg.Add(1)
	go d.flushLoop(ctx)

	// Start metrics server if configured
	if d.config.MetricsAddr != "" {
		d.wg.Add(1)
		go d.serveMetrics(ctx)
	}

	// Accept connections
	d.wg.Add(1)
	go d.acceptLoop(ctx)

	log.Printf("[INFO] Oculo daemon listening on %s (network: %s)", d.config.ListenAddr, network)
	return nil
}

// Stop gracefully shuts down the ingester, flushing remaining buffered data.
func (d *DaemonIngester) Stop() error {
	log.Println("[INFO] Shutting down Oculo daemon...")

	if d.cancel != nil {
		d.cancel()
	}

	if d.listener != nil {
		d.listener.Close()
	}

	// Close channels to signal flush goroutine
	close(d.spanChan)
	close(d.memoryEventChan)
	close(d.traceChan)

	d.wg.Wait()
	close(d.done)

	log.Println("[INFO] Oculo daemon stopped.")
	return nil
}

// Metrics returns a snapshot of the current ingestion metrics.
func (d *DaemonIngester) Metrics() IngestionMetrics {
	return IngestionMetrics{
		TracesIngested:   atomic.LoadInt64(&d.metrics.TracesIngested),
		SpansIngested:    atomic.LoadInt64(&d.metrics.SpansIngested),
		MemoryEvents:     atomic.LoadInt64(&d.metrics.MemoryEvents),
		ErrorCount:       atomic.LoadInt64(&d.metrics.ErrorCount),
		BatchesCommitted: atomic.LoadInt64(&d.metrics.BatchesCommitted),
		Uptime:           int64(time.Since(d.started).Seconds()),
	}
}

// acceptLoop handles incoming connections.
func (d *DaemonIngester) acceptLoop(ctx context.Context) {
	defer d.wg.Done()

	for {
		conn, err := d.listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				log.Printf("[ERROR] Accept failed: %v", err)
				continue
			}
		}

		d.wg.Add(1)
		go d.handleConnection(ctx, conn)
	}
}

// handleConnection reads wire messages from a single client connection.
// Messages use a length-prefixed JSON format:
//   [1 byte type][4 bytes length][JSON payload]
func (d *DaemonIngester) handleConnection(ctx context.Context, conn net.Conn) {
	defer d.wg.Done()
	defer conn.Close()

	log.Printf("[DEBUG] New connection from %s", conn.RemoteAddr())

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Read message type (1 byte)
		typeBuf := make([]byte, 1)
		if _, err := io.ReadFull(conn, typeBuf); err != nil {
			if err != io.EOF {
				log.Printf("[DEBUG] Connection read error: %v", err)
			}
			return
		}
		msgType := MessageType(typeBuf[0])

		// Read payload length (4 bytes, big-endian)
		lenBuf := make([]byte, 4)
		if _, err := io.ReadFull(conn, lenBuf); err != nil {
			log.Printf("[ERROR] Failed to read message length: %v", err)
			atomic.AddInt64(&d.metrics.ErrorCount, 1)
			return
		}
		payloadLen := binary.BigEndian.Uint32(lenBuf)

		// Safety check: reject messages larger than 10MB
		if payloadLen > 10*1024*1024 {
			log.Printf("[ERROR] Message too large: %d bytes", payloadLen)
			atomic.AddInt64(&d.metrics.ErrorCount, 1)
			return
		}

		// Read payload
		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(conn, payload); err != nil {
			log.Printf("[ERROR] Failed to read payload: %v", err)
			atomic.AddInt64(&d.metrics.ErrorCount, 1)
			return
		}

		// Process the message
		if err := d.processMessage(msgType, payload); err != nil {
			log.Printf("[ERROR] Processing message: %v", err)
			atomic.AddInt64(&d.metrics.ErrorCount, 1)
		}

		// Send ACK (1 byte: 0x00 = success, 0x01 = error)
		conn.Write([]byte{0x00})
	}
}

// processMessage deserializes and routes a wire message to the appropriate
// channel for batched insertion.
func (d *DaemonIngester) processMessage(msgType MessageType, payload []byte) error {
	switch msgType {
	case MsgTrace:
		var trace database.Trace
		if err := json.Unmarshal(payload, &trace); err != nil {
			return fmt.Errorf("unmarshaling trace: %w", err)
		}
		select {
		case d.traceChan <- &trace:
			atomic.AddInt64(&d.metrics.TracesIngested, 1)
		default:
			// Channel full — insert directly to avoid data loss
			if err := d.store.InsertTrace(&trace); err != nil {
				return fmt.Errorf("direct trace insert: %w", err)
			}
			atomic.AddInt64(&d.metrics.TracesIngested, 1)
		}

	case MsgSpan:
		var span database.Span
		if err := json.Unmarshal(payload, &span); err != nil {
			return fmt.Errorf("unmarshaling span: %w", err)
		}
		select {
		case d.spanChan <- &span:
			atomic.AddInt64(&d.metrics.SpansIngested, 1)
		default:
			if err := d.store.InsertSpan(&span); err != nil {
				return fmt.Errorf("direct span insert: %w", err)
			}
			atomic.AddInt64(&d.metrics.SpansIngested, 1)
		}

	case MsgMemoryEvent:
		var event database.MemoryEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			return fmt.Errorf("unmarshaling memory event: %w", err)
		}
		select {
		case d.memoryEventChan <- &event:
			atomic.AddInt64(&d.metrics.MemoryEvents, 1)
		default:
			if err := d.store.InsertMemoryEvent(&event); err != nil {
				return fmt.Errorf("direct memory event insert: %w", err)
			}
			atomic.AddInt64(&d.metrics.MemoryEvents, 1)
		}

	case MsgBatch:
		var batch BatchMessage
		if err := json.Unmarshal(payload, &batch); err != nil {
			return fmt.Errorf("unmarshaling batch: %w", err)
		}
		return d.processBatch(&batch)

	default:
		return fmt.Errorf("unknown message type: 0x%02x", msgType)
	}

	return nil
}

// processBatch handles a batch message containing mixed types.
func (d *DaemonIngester) processBatch(batch *BatchMessage) error {
	for _, t := range batch.Traces {
		if err := d.store.InsertTrace(t); err != nil {
			return fmt.Errorf("batch trace insert: %w", err)
		}
		atomic.AddInt64(&d.metrics.TracesIngested, 1)
	}

	if len(batch.Spans) > 0 {
		if err := d.store.BatchInsertSpans(batch.Spans); err != nil {
			return fmt.Errorf("batch span insert: %w", err)
		}
		atomic.AddInt64(&d.metrics.SpansIngested, int64(len(batch.Spans)))
	}

	if len(batch.MemoryEvents) > 0 {
		if err := d.store.BatchInsertMemoryEvents(batch.MemoryEvents); err != nil {
			return fmt.Errorf("batch memory event insert: %w", err)
		}
		atomic.AddInt64(&d.metrics.MemoryEvents, int64(len(batch.MemoryEvents)))
	}

	for _, tc := range batch.ToolCalls {
		if err := d.store.InsertToolCall(tc); err != nil {
			return fmt.Errorf("batch tool call insert: %w", err)
		}
	}

	atomic.AddInt64(&d.metrics.BatchesCommitted, 1)
	return nil
}

// flushLoop periodically flushes buffered items to the database.
// It commits when either BatchSize items accumulate or FlushInterval elapses.
func (d *DaemonIngester) flushLoop(ctx context.Context) {
	defer d.wg.Done()

	ticker := time.NewTicker(d.config.FlushInterval)
	defer ticker.Stop()

	spanBuf := make([]*database.Span, 0, d.config.BatchSize)
	memBuf := make([]*database.MemoryEvent, 0, d.config.BatchSize)

	flush := func() {
		if len(spanBuf) > 0 {
			if err := d.store.BatchInsertSpans(spanBuf); err != nil {
				log.Printf("[ERROR] Flushing span batch: %v", err)
				atomic.AddInt64(&d.metrics.ErrorCount, 1)
			} else {
				atomic.AddInt64(&d.metrics.BatchesCommitted, 1)
			}
			spanBuf = spanBuf[:0]
		}
		if len(memBuf) > 0 {
			if err := d.store.BatchInsertMemoryEvents(memBuf); err != nil {
				log.Printf("[ERROR] Flushing memory event batch: %v", err)
				atomic.AddInt64(&d.metrics.ErrorCount, 1)
			} else {
				atomic.AddInt64(&d.metrics.BatchesCommitted, 1)
			}
			memBuf = memBuf[:0]
		}
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return

		case trace, ok := <-d.traceChan:
			if !ok {
				flush()
				return
			}
			// Traces are inserted immediately (low volume, high importance)
			if err := d.store.InsertTrace(trace); err != nil {
				log.Printf("[ERROR] Inserting trace: %v", err)
				atomic.AddInt64(&d.metrics.ErrorCount, 1)
			}

		case span, ok := <-d.spanChan:
			if !ok {
				flush()
				return
			}
			spanBuf = append(spanBuf, span)
			if len(spanBuf) >= d.config.BatchSize {
				flush()
			}

		case event, ok := <-d.memoryEventChan:
			if !ok {
				flush()
				return
			}
			memBuf = append(memBuf, event)
			if len(memBuf) >= d.config.BatchSize {
				flush()
			}

		case <-ticker.C:
			flush()
		}
	}
}

// replayPending replays any pending writes from a previous crash.
func (d *DaemonIngester) replayPending() error {
	pending, err := d.store.GetPendingPayloads()
	if err != nil {
		return fmt.Errorf("getting pending payloads: %w", err)
	}

	if len(pending) == 0 {
		return nil
	}

	log.Printf("[INFO] Replaying %d pending writes from crash recovery", len(pending))

	for _, pw := range pending {
		var batch BatchMessage
		if err := json.Unmarshal(pw.Payload, &batch); err != nil {
			log.Printf("[WARN] Skipping corrupt pending write %d: %v", pw.WriteID, err)
			continue
		}

		if err := d.processBatch(&batch); err != nil {
			log.Printf("[ERROR] Failed to replay pending write %d: %v", pw.WriteID, err)
			continue
		}

		if err := d.store.CommitPendingPayload(pw.WriteID); err != nil {
			log.Printf("[ERROR] Failed to commit pending write %d: %v", pw.WriteID, err)
		}
	}

	return nil
}

// serveMetrics starts an HTTP server exposing ingestion metrics.
func (d *DaemonIngester) serveMetrics(ctx context.Context) {
	defer d.wg.Done()

	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Metrics endpoint (Prometheus-compatible text format)
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		m := d.Metrics()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "# HELP oculo_traces_ingested_total Total traces ingested\n")
		fmt.Fprintf(w, "# TYPE oculo_traces_ingested_total counter\n")
		fmt.Fprintf(w, "oculo_traces_ingested_total %d\n", m.TracesIngested)
		fmt.Fprintf(w, "# HELP oculo_spans_ingested_total Total spans ingested\n")
		fmt.Fprintf(w, "# TYPE oculo_spans_ingested_total counter\n")
		fmt.Fprintf(w, "oculo_spans_ingested_total %d\n", m.SpansIngested)
		fmt.Fprintf(w, "# HELP oculo_memory_events_total Total memory events\n")
		fmt.Fprintf(w, "# TYPE oculo_memory_events_total counter\n")
		fmt.Fprintf(w, "oculo_memory_events_total %d\n", m.MemoryEvents)
		fmt.Fprintf(w, "# HELP oculo_errors_total Total errors\n")
		fmt.Fprintf(w, "# TYPE oculo_errors_total counter\n")
		fmt.Fprintf(w, "oculo_errors_total %d\n", m.ErrorCount)
		fmt.Fprintf(w, "# HELP oculo_batches_committed_total Total batches committed\n")
		fmt.Fprintf(w, "# TYPE oculo_batches_committed_total counter\n")
		fmt.Fprintf(w, "oculo_batches_committed_total %d\n", m.BatchesCommitted)
		fmt.Fprintf(w, "# HELP oculo_uptime_seconds Uptime in seconds\n")
		fmt.Fprintf(w, "# TYPE oculo_uptime_seconds gauge\n")
		fmt.Fprintf(w, "oculo_uptime_seconds %d\n", m.Uptime)
	})

	// JSON metrics for programmatic access
	mux.HandleFunc("/api/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(d.Metrics())
	})

	server := &http.Server{
		Addr:    d.config.MetricsAddr,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		server.Shutdown(context.Background())
	}()

	log.Printf("[INFO] Metrics server listening on http://%s/metrics", d.config.MetricsAddr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Printf("[ERROR] Metrics server: %v", err)
	}
}
