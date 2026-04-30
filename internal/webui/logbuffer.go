package webui

import (
	"sync"
	"time"
)

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Raw       string    `json:"raw"`
}

// LogBuffer is a thread-safe ring buffer for log entries
type LogBuffer struct {
	entries []LogEntry
	size    int
	head    int
	count   int
	mu      sync.RWMutex
}

// NewLogBuffer creates a new log buffer with the specified capacity
func NewLogBuffer(size int) *LogBuffer {
	return &LogBuffer{
		entries: make([]LogEntry, size),
		size:    size,
	}
}

// Write implements io.Writer for capturing log output
func (lb *LogBuffer) Write(p []byte) (n int, err error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	entry := LogEntry{
		Timestamp: time.Now(),
		Raw:       string(p),
	}

	// Parse level from JSON if possible (zerolog format)
	raw := string(p)
	entry.Level = parseLevel(raw)
	entry.Message = parseMessage(raw)

	lb.entries[lb.head] = entry
	lb.head = (lb.head + 1) % lb.size
	if lb.count < lb.size {
		lb.count++
	}

	return len(p), nil
}

// GetEntries returns all log entries in chronological order
func (lb *LogBuffer) GetEntries() []LogEntry {
	lb.mu.RLock()
	defer lb.mu.RUnlock()

	result := make([]LogEntry, lb.count)
	if lb.count == 0 {
		return result
	}

	start := 0
	if lb.count == lb.size {
		start = lb.head
	}

	for i := 0; i < lb.count; i++ {
		idx := (start + i) % lb.size
		result[i] = lb.entries[idx]
	}

	return result
}

// GetRecentEntries returns the most recent n entries
func (lb *LogBuffer) GetRecentEntries(n int) []LogEntry {
	entries := lb.GetEntries()
	if len(entries) <= n {
		return entries
	}
	return entries[len(entries)-n:]
}

// Clear clears all log entries
func (lb *LogBuffer) Clear() {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	lb.head = 0
	lb.count = 0
}

// parseLevel extracts the log level from a zerolog JSON line
func parseLevel(raw string) string {
	// Simple parsing for zerolog JSON format
	levels := []string{"debug", "info", "warn", "error", "fatal"}
	for _, level := range levels {
		if contains(raw, `"level":"`+level+`"`) {
			return level
		}
	}
	return "info"
}

// parseMessage extracts the message from a zerolog JSON line
func parseMessage(raw string) string {
	// Look for "msg":"..." pattern
	start := indexOf(raw, `"msg":"`)
	if start == -1 {
		return raw
	}
	start += 7 // len(`"msg":"`)
	end := start
	for end < len(raw) && raw[end] != '"' {
		if raw[end] == '\\' && end+1 < len(raw) {
			end += 2
			continue
		}
		end++
	}
	if end > start {
		return raw[start:end]
	}
	return raw
}

func contains(s, substr string) bool {
	return indexOf(s, substr) != -1
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
