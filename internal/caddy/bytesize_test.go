package caddy

import "testing"

func TestParseByteSize(t *testing.T) {
	cases := []struct {
		input   string
		want    int64
		wantErr bool
	}{
		{"1B", 1, false},
		{"100B", 100, false},
		{"1KB", 1024, false},
		{"1MB", 1048576, false},
		{"1GB", 1073741824, false},
		{"10MB", 10 * 1024 * 1024, false},
		{"512KB", 512 * 1024, false},
		{"2GB", 2 * 1024 * 1024 * 1024, false},

		// case insensitive
		{"1kb", 1024, false},
		{"1mb", 1048576, false},
		{"1gb", 1073741824, false},
		{"10Mb", 10 * 1024 * 1024, false},

		// whitespace trimming
		{"  1MB  ", 1048576, false},
		{" 512KB", 512 * 1024, false},

		// fractional values
		{"1.5MB", int64(1.5 * 1024 * 1024), false},
		{"0.5GB", int64(0.5 * 1024 * 1024 * 1024), false},
		{"2.5KB", int64(2.5 * 1024), false},

		// bare number (bytes)
		{"1024", 1024, false},
		{"1", 1, false},
		{"999999", 999999, false},

		// errors
		{"", 0, true},
		{"0B", 0, true},
		{"-1MB", 0, true},
		{"0", 0, true},
		{"-100", 0, true},
		{"abc", 0, true},
		{"MB", 0, true},
		{"10TB", 0, true},
	}

	for _, c := range cases {
		got, err := ParseByteSize(c.input)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseByteSize(%q) = %d, want error", c.input, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseByteSize(%q) unexpected error: %v", c.input, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseByteSize(%q) = %d, want %d", c.input, got, c.want)
		}
	}
}

func TestFormatByteSize(t *testing.T) {
	cases := []struct {
		input int64
		want  string
	}{
		{1073741824, "1GB"},
		{2 * 1024 * 1024 * 1024, "2GB"},
		{1048576, "1MB"},
		{10 * 1024 * 1024, "10MB"},
		{1024, "1KB"},
		{512 * 1024, "512KB"},

		// non-aligned values fall through to raw bytes
		{1500, "1500"},
		{1, "1"},
		{1024*1024 + 1, "1048577"},

		// zero
		{0, "0"},
	}

	for _, c := range cases {
		got := formatByteSize(c.input)
		if got != c.want {
			t.Errorf("formatByteSize(%d) = %q, want %q", c.input, got, c.want)
		}
	}
}
