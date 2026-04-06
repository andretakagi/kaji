// Log entry types and paginated querying. Reads the log file backwards
// so the most recent entries come first.
package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

type LogEntry struct {
	Level   string      `json:"level"`
	Ts      float64     `json:"ts"`
	Logger  string      `json:"logger"`
	Msg     string      `json:"msg"`
	Request *LogRequest `json:"request,omitempty"`

	Duration    float64             `json:"duration,omitempty"`
	Status      int                 `json:"status,omitempty"`
	Size        int                 `json:"size,omitempty"`
	RespHeaders map[string][]string `json:"resp_headers,omitempty"`

	Extra map[string]any `json:"extra,omitempty"`
}

// Known top-level keys that are already mapped to struct fields.
var knownKeys = map[string]bool{
	"level": true, "ts": true, "logger": true, "msg": true,
	"request": true, "duration": true, "status": true,
	"size": true, "resp_headers": true,
}

func (e *LogEntry) UnmarshalJSON(data []byte) error {
	// Unmarshal known fields via an alias to avoid infinite recursion.
	type Alias LogEntry
	var alias Alias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	*e = LogEntry(alias)

	// Collect any remaining fields into Extra.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for k, v := range raw {
		if knownKeys[k] {
			continue
		}
		if e.Extra == nil {
			e.Extra = make(map[string]any)
		}
		var val any
		if err := json.Unmarshal(v, &val); err != nil {
			e.Extra[k] = string(v)
		} else {
			e.Extra[k] = val
		}
	}
	return nil
}

type LogRequest struct {
	RemoteAddr string              `json:"remote_addr"`
	Proto      string              `json:"proto"`
	Method     string              `json:"method"`
	Host       string              `json:"host"`
	URI        string              `json:"uri"`
	Headers    map[string][]string `json:"headers,omitempty"`
	TLS        *LogTLS             `json:"tls,omitempty"`
}

type LogTLS struct {
	Resumed     bool   `json:"resumed"`
	Version     int    `json:"version"`
	CipherSuite int    `json:"cipher_suite"`
	Proto       string `json:"proto"`
	ServerName  string `json:"server_name"`
}

type QueryParams struct {
	Limit     int
	Offset    int
	Level     string
	Host      string
	StatusMin int
	StatusMax int
	Since     time.Time
	Until     time.Time
}

type QueryResult struct {
	Entries []LogEntry `json:"entries"`
	HasMore bool       `json:"has_more"`
}

func QueryLogs(path string, params QueryParams) (*QueryResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening log file %s: %w", path, err)
	}
	defer f.Close()

	rs, err := newReverseScanner(f)
	if err != nil {
		return nil, fmt.Errorf("initializing reverse scanner: %w", err)
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 100
	}

	var entries []LogEntry
	skipped := 0

	for rs.Scan() {
		line := rs.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry LogEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if !matchesFilter(entry, params) {
			continue
		}

		if skipped < params.Offset {
			skipped++
			continue
		}

		entries = append(entries, entry)
		if len(entries) > limit {
			break
		}
	}

	if err := rs.Err(); err != nil {
		return nil, fmt.Errorf("scanning log file: %w", err)
	}

	hasMore := len(entries) > limit
	if hasMore {
		entries = entries[:limit]
	}

	if entries == nil {
		entries = []LogEntry{}
	}

	return &QueryResult{Entries: entries, HasMore: hasMore}, nil
}

func matchesFilter(entry LogEntry, params QueryParams) bool {
	if params.Level != "" && entry.Level != params.Level {
		return false
	}

	if params.Host != "" {
		if entry.Request == nil || entry.Request.Host != params.Host {
			return false
		}
	}

	if params.StatusMin > 0 && entry.Status < params.StatusMin {
		return false
	}
	if params.StatusMax > 0 && entry.Status > params.StatusMax {
		return false
	}

	if !params.Since.IsZero() || !params.Until.IsZero() {
		secs := int64(entry.Ts)
		entryTime := time.Unix(secs, int64((entry.Ts-float64(secs))*1e9))
		if !params.Since.IsZero() && entryTime.Before(params.Since) {
			return false
		}
		if !params.Until.IsZero() && entryTime.After(params.Until) {
			return false
		}
	}

	return true
}

const chunkSize = 32 * 1024

type reverseScanner struct {
	f      *os.File
	offset int64  // read cursor, moves backwards from end
	carry  []byte // leftover bytes from the front of the last chunk
	lines  [][]byte
	err    error
}

func newReverseScanner(f *os.File) (*reverseScanner, error) {
	size, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, fmt.Errorf("seeking to end of file: %w", err)
	}
	return &reverseScanner{f: f, offset: size}, nil
}

func (rs *reverseScanner) Scan() bool {
	for len(rs.lines) == 0 {
		if rs.offset <= 0 && len(rs.carry) == 0 {
			return false
		}
		if rs.offset <= 0 {
			// No more file to read, but we have leftover carry bytes -
			// that's the very first line in the file.
			rs.lines = [][]byte{rs.carry}
			rs.carry = nil
			return true
		}
		if err := rs.readChunk(); err != nil {
			rs.err = err
			return false
		}
	}
	return true
}

func (rs *reverseScanner) Err() error {
	return rs.err
}

func (rs *reverseScanner) Bytes() []byte {
	n := len(rs.lines)
	line := rs.lines[n-1]
	rs.lines = rs.lines[:n-1]
	return line
}

func (rs *reverseScanner) readChunk() error {
	readSize := int64(chunkSize)
	if readSize > rs.offset {
		readSize = rs.offset
	}
	rs.offset -= readSize

	buf := make([]byte, readSize)
	if _, err := rs.f.ReadAt(buf, rs.offset); err != nil && err != io.EOF {
		return fmt.Errorf("reading chunk at offset %d: %w", rs.offset, err)
	}

	if len(rs.carry) > 0 {
		buf = append(buf, rs.carry...)
		rs.carry = nil
	}

	// Split into lines. The first segment may be a partial line if we're
	// not at the start of the file, so stash it as carry.
	var lines [][]byte
	start := 0
	for i := 0; i < len(buf); i++ {
		if buf[i] == '\n' {
			lines = append(lines, buf[start:i])
			start = i + 1
		}
	}

	// Anything after the last newline
	if start < len(buf) {
		if rs.offset == 0 {
			// We're at the beginning of the file, so this is a complete line.
			lines = append(lines, buf[start:])
		} else {
			rs.carry = make([]byte, len(buf)-start)
			copy(rs.carry, buf[start:])
		}
	}

	rs.lines = lines
	return nil
}
