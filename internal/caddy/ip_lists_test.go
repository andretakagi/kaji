package caddy

import (
	"sort"
	"strings"
	"testing"
)

func TestResolveIPList(t *testing.T) {
	tests := []struct {
		name    string
		listID  string
		lists   []IPListEntry
		want    []string
		wantErr string
	}{
		{
			name:   "flat_list",
			listID: "a",
			lists: []IPListEntry{
				{ID: "a", IPs: []string{"10.0.0.1", "10.0.0.2"}},
			},
			want: []string{"10.0.0.1", "10.0.0.2"},
		},
		{
			name:   "nested_composite",
			listID: "parent",
			lists: []IPListEntry{
				{ID: "parent", IPs: []string{"10.0.0.1"}, Children: []string{"child"}},
				{ID: "child", IPs: []string{"10.0.0.2", "10.0.0.3"}},
			},
			want: []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"},
		},
		{
			name:   "deduplication",
			listID: "parent",
			lists: []IPListEntry{
				{ID: "parent", IPs: []string{"10.0.0.1", "10.0.0.2"}, Children: []string{"child"}},
				{ID: "child", IPs: []string{"10.0.0.2", "10.0.0.3"}},
			},
			want: []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"},
		},
		{
			name:   "deep_nesting",
			listID: "a",
			lists: []IPListEntry{
				{ID: "a", IPs: []string{"1.0.0.1"}, Children: []string{"b"}},
				{ID: "b", IPs: []string{"2.0.0.1"}, Children: []string{"c"}},
				{ID: "c", IPs: []string{"3.0.0.1"}},
			},
			want: []string{"1.0.0.1", "2.0.0.1", "3.0.0.1"},
		},
		{
			name:   "circular_reference",
			listID: "a",
			lists: []IPListEntry{
				{ID: "a", Children: []string{"b"}},
				{ID: "b", Children: []string{"a"}},
			},
			wantErr: "circular reference",
		},
		{
			name:    "missing_list",
			listID:  "nonexistent",
			lists:   []IPListEntry{},
			wantErr: "not found",
		},
		{
			name:   "child_not_found",
			listID: "a",
			lists: []IPListEntry{
				{ID: "a", IPs: []string{"10.0.0.1"}, Children: []string{"missing"}},
			},
			wantErr: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveIPList(tt.listID, tt.lists)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			sort.Strings(got)
			sort.Strings(tt.want)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("got[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
