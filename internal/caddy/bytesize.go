package caddy

import (
	"fmt"
	"strconv"
	"strings"
)

func ParseByteSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty size string")
	}

	upper := strings.ToUpper(s)

	multipliers := []struct {
		suffix string
		factor int64
	}{
		{"GB", 1 << 30},
		{"MB", 1 << 20},
		{"KB", 1 << 10},
		{"B", 1},
	}

	for _, m := range multipliers {
		if strings.HasSuffix(upper, m.suffix) {
			numStr := strings.TrimSpace(upper[:len(upper)-len(m.suffix)])
			n, err := strconv.ParseFloat(numStr, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid number in size %q: %w", s, err)
			}
			if n <= 0 {
				return 0, fmt.Errorf("size must be positive, got %q", s)
			}
			return int64(n * float64(m.factor)), nil
		}
	}

	n, err := strconv.ParseInt(upper, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: must be a number or number with B/KB/MB/GB suffix", s)
	}
	if n <= 0 {
		return 0, fmt.Errorf("size must be positive, got %d", n)
	}
	return n, nil
}
