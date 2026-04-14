package logging

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeLogFile(t *testing.T, lines []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.log")
	var content string
	for _, line := range lines {
		content += line + "\n"
	}
	os.WriteFile(path, []byte(content), 0644)
	return path
}

func makeLogLine(level string, status int, host string, ts float64) string {
	entry := map[string]any{"level": level, "ts": ts, "msg": "test", "status": status}
	if host != "" {
		entry["request"] = map[string]any{"host": host}
	}
	b, _ := json.Marshal(entry)
	return string(b)
}

// reverseScanner tests

func TestReverseScannerOrder(t *testing.T) {
	lines := []string{"first", "second", "third"}
	path := writeLogFile(t, lines)

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	rs, err := newReverseScanner(f)
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	for rs.Scan() {
		got = append(got, string(rs.Bytes()))
	}
	if err := rs.Err(); err != nil {
		t.Fatal(err)
	}

	// Expect reverse order: third, second, first
	want := []string{"third", "second", "first"}
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestReverseScannerEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.log")
	os.WriteFile(path, []byte{}, 0644)

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	rs, err := newReverseScanner(f)
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	for rs.Scan() {
		got = append(got, string(rs.Bytes()))
	}
	if err := rs.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no lines from empty file, got %d", len(got))
	}
}

func TestReverseScannerSingleLine(t *testing.T) {
	path := writeLogFile(t, []string{"only line"})

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	rs, err := newReverseScanner(f)
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	for rs.Scan() {
		got = append(got, string(rs.Bytes()))
	}
	if err := rs.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "only line" {
		t.Fatalf("expected [\"only line\"], got %v", got)
	}
}

func TestReverseScannerChunkBoundary(t *testing.T) {
	// Write enough lines so that total content exceeds chunkSize, forcing
	// the scanner to read multiple chunks and handle a line that straddles
	// a chunk boundary.
	lineContent := strings.Repeat("x", 1000)
	var lines []string
	// chunkSize is 32*1024 = 32768 bytes; 40 lines * ~1001 bytes = ~40 KB > 32 KB
	for i := 0; i < 40; i++ {
		lines = append(lines, lineContent)
	}
	path := writeLogFile(t, lines)

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	rs, err := newReverseScanner(f)
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	for rs.Scan() {
		got = append(got, string(rs.Bytes()))
	}
	if err := rs.Err(); err != nil {
		t.Fatal(err)
	}

	if len(got) != len(lines) {
		t.Fatalf("expected %d lines, got %d", len(lines), len(got))
	}
	// All lines have the same content; verify each one is correct.
	for i, line := range got {
		if line != lineContent {
			t.Errorf("line %d has unexpected content (len %d)", i, len(line))
		}
	}
}

func TestReverseScannerNoTrailingNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-trailing.log")
	os.WriteFile(path, []byte("aaa\nbbb\nccc"), 0644)

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	rs, err := newReverseScanner(f)
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	for rs.Scan() {
		got = append(got, string(rs.Bytes()))
	}
	if err := rs.Err(); err != nil {
		t.Fatal(err)
	}

	want := []string{"ccc", "bbb", "aaa"}
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestReverseScannerLongFirstLine(t *testing.T) {
	longLine := strings.Repeat("A", 40000)
	path := filepath.Join(t.TempDir(), "long-first.log")
	os.WriteFile(path, []byte(longLine+"\nshort\n"), 0644)

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	rs, err := newReverseScanner(f)
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	for rs.Scan() {
		got = append(got, string(rs.Bytes()))
	}
	if err := rs.Err(); err != nil {
		t.Fatal(err)
	}

	want := []string{"short", longLine}
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d: got len %d, want len %d", i, len(got[i]), len(want[i]))
		}
	}
}

func TestReverseScannerHugeSingleLine(t *testing.T) {
	hugeLine := strings.Repeat("Z", 100000)
	path := filepath.Join(t.TempDir(), "huge-single.log")
	os.WriteFile(path, []byte(hugeLine), 0644)

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	rs, err := newReverseScanner(f)
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	for rs.Scan() {
		got = append(got, string(rs.Bytes()))
	}
	if err := rs.Err(); err != nil {
		t.Fatal(err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 line, got %d", len(got))
	}
	if len(got[0]) != 100000 {
		t.Errorf("line length: got %d, want 100000", len(got[0]))
	}
}

// LogEntry.UnmarshalJSON tests

func TestUnmarshalJSONKnownFields(t *testing.T) {
	raw := `{"level":"info","ts":1000.5,"logger":"http","msg":"request","status":200}`
	var e LogEntry
	if err := json.Unmarshal([]byte(raw), &e); err != nil {
		t.Fatal(err)
	}
	if e.Level != "info" {
		t.Errorf("Level: got %q, want \"info\"", e.Level)
	}
	if e.Ts != 1000.5 {
		t.Errorf("Ts: got %f, want 1000.5", e.Ts)
	}
	if e.Status != 200 {
		t.Errorf("Status: got %d, want 200", e.Status)
	}
	if e.Extra != nil {
		t.Errorf("Extra should be nil for known-only fields, got %v", e.Extra)
	}
}

func TestUnmarshalJSONExtraFields(t *testing.T) {
	raw := `{"level":"info","ts":1.0,"msg":"hi","custom_key":"custom_value","num":42}`
	var e LogEntry
	if err := json.Unmarshal([]byte(raw), &e); err != nil {
		t.Fatal(err)
	}
	if e.Extra == nil {
		t.Fatal("Extra should not be nil when unknown fields are present")
	}
	if v, ok := e.Extra["custom_key"]; !ok || v != "custom_value" {
		t.Errorf("Extra[custom_key]: got %v, want \"custom_value\"", v)
	}
	if v, ok := e.Extra["num"]; !ok {
		t.Error("Extra[num] missing")
	} else if v.(float64) != 42 {
		t.Errorf("Extra[num]: got %v, want 42", v)
	}
}

func TestUnmarshalJSONRequestHost(t *testing.T) {
	raw := `{"level":"info","ts":1.0,"msg":"req","request":{"host":"example.com","method":"GET","remote_addr":"1.2.3.4","proto":"HTTP/1.1","uri":"/"}}`
	var e LogEntry
	if err := json.Unmarshal([]byte(raw), &e); err != nil {
		t.Fatal(err)
	}
	if e.Request == nil {
		t.Fatal("Request should not be nil")
	}
	if e.Request.Host != "example.com" {
		t.Errorf("Request.Host: got %q, want \"example.com\"", e.Request.Host)
	}
}

// QueryLogs tests

func TestQueryLogsBasic(t *testing.T) {
	lines := []string{
		makeLogLine("info", 200, "a.com", 1000),
		makeLogLine("warn", 404, "b.com", 2000),
		makeLogLine("error", 500, "c.com", 3000),
	}
	path := writeLogFile(t, lines)

	result, err := QueryLogs(path, QueryParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(result.Entries))
	}
	// QueryLogs reads newest-first; last line written is first returned.
	if result.Entries[0].Status != 500 {
		t.Errorf("first entry status: got %d, want 500", result.Entries[0].Status)
	}
	if result.HasMore {
		t.Error("HasMore should be false")
	}
}

func TestQueryLogsFilterByLevel(t *testing.T) {
	lines := []string{
		makeLogLine("info", 200, "", 1000),
		makeLogLine("error", 500, "", 2000),
		makeLogLine("info", 200, "", 3000),
	}
	path := writeLogFile(t, lines)

	result, err := QueryLogs(path, QueryParams{Level: "info"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("expected 2 info entries, got %d", len(result.Entries))
	}
	for _, e := range result.Entries {
		if e.Level != "info" {
			t.Errorf("unexpected level %q in filtered results", e.Level)
		}
	}
}

func TestQueryLogsFilterByHost(t *testing.T) {
	lines := []string{
		makeLogLine("info", 200, "a.com", 1000),
		makeLogLine("info", 200, "b.com", 2000),
		makeLogLine("info", 200, "a.com", 3000),
	}
	path := writeLogFile(t, lines)

	result, err := QueryLogs(path, QueryParams{Host: "a.com"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("expected 2 entries for a.com, got %d", len(result.Entries))
	}
	for _, e := range result.Entries {
		if e.Request == nil || e.Request.Host != "a.com" {
			t.Errorf("unexpected host in filtered results")
		}
	}
}

func TestQueryLogsFilterByStatusRange(t *testing.T) {
	lines := []string{
		makeLogLine("info", 200, "", 1000),
		makeLogLine("warn", 301, "", 2000),
		makeLogLine("warn", 404, "", 3000),
		makeLogLine("error", 500, "", 4000),
	}
	path := writeLogFile(t, lines)

	result, err := QueryLogs(path, QueryParams{StatusMin: 400, StatusMax: 499})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 4xx entry, got %d", len(result.Entries))
	}
	if result.Entries[0].Status != 404 {
		t.Errorf("expected status 404, got %d", result.Entries[0].Status)
	}
}

func TestQueryLogsFilterByTimeRange(t *testing.T) {
	// ts values are Unix seconds
	lines := []string{
		makeLogLine("info", 200, "", 1000),
		makeLogLine("info", 200, "", 2000),
		makeLogLine("info", 200, "", 3000),
		makeLogLine("info", 200, "", 4000),
	}
	path := writeLogFile(t, lines)

	since := time.Unix(1500, 0)
	until := time.Unix(3500, 0)
	result, err := QueryLogs(path, QueryParams{Since: since, Until: until})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("expected 2 entries in time range, got %d", len(result.Entries))
	}
	for _, e := range result.Entries {
		if e.Ts < 1500 || e.Ts > 3500 {
			t.Errorf("entry ts %f outside expected range", e.Ts)
		}
	}
}

func TestQueryLogsPaginationLimitAndOffset(t *testing.T) {
	var lines []string
	for i := 0; i < 10; i++ {
		lines = append(lines, makeLogLine("info", 200, "", float64(1000+i)))
	}
	path := writeLogFile(t, lines)

	// First page: limit=3, offset=0
	r1, err := QueryLogs(path, QueryParams{Limit: 3, Offset: 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(r1.Entries) != 3 {
		t.Fatalf("page 1: expected 3 entries, got %d", len(r1.Entries))
	}
	if !r1.HasMore {
		t.Error("page 1: HasMore should be true")
	}

	// Second page: limit=3, offset=3
	r2, err := QueryLogs(path, QueryParams{Limit: 3, Offset: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(r2.Entries) != 3 {
		t.Fatalf("page 2: expected 3 entries, got %d", len(r2.Entries))
	}
	if !r2.HasMore {
		t.Error("page 2: HasMore should be true")
	}

	// Last page: limit=3, offset=9 - only 1 entry left
	r3, err := QueryLogs(path, QueryParams{Limit: 3, Offset: 9})
	if err != nil {
		t.Fatal(err)
	}
	if len(r3.Entries) != 1 {
		t.Fatalf("last page: expected 1 entry, got %d", len(r3.Entries))
	}
	if r3.HasMore {
		t.Error("last page: HasMore should be false")
	}
}

func TestQueryLogsHasMoreExact(t *testing.T) {
	// Exactly limit entries - HasMore should be false.
	var lines []string
	for i := 0; i < 5; i++ {
		lines = append(lines, makeLogLine("info", 200, "", float64(1000+i)))
	}
	path := writeLogFile(t, lines)

	result, err := QueryLogs(path, QueryParams{Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(result.Entries))
	}
	if result.HasMore {
		t.Error("HasMore should be false when exactly limit entries exist")
	}
}

func TestQueryLogsSkipsEmptyAndInvalidLines(t *testing.T) {
	valid := makeLogLine("info", 200, "", 1000)
	lines := []string{
		valid,
		"",
		"not json at all {{{",
		valid,
	}
	path := writeLogFile(t, lines)

	result, err := QueryLogs(path, QueryParams{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("expected 2 valid entries, got %d", len(result.Entries))
	}
}

func TestQueryLogsDefaultLimit(t *testing.T) {
	// When Limit=0, should default to 100.
	var lines []string
	for i := 0; i < 50; i++ {
		lines = append(lines, makeLogLine("info", 200, "", float64(i)))
	}
	path := writeLogFile(t, lines)

	result, err := QueryLogs(path, QueryParams{Limit: 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 50 {
		t.Fatalf("expected 50 entries with default limit, got %d", len(result.Entries))
	}
	if result.HasMore {
		t.Error("HasMore should be false")
	}
}

func TestQueryLogsFileNotFound(t *testing.T) {
	_, err := QueryLogs("/nonexistent/path/to/log.log", QueryParams{})
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

