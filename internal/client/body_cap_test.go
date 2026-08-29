package client

import (
	"bytes"
	"strings"
	"testing"
)

// An oversized body must error, not truncate silently.
func TestReadCappedBody(t *testing.T) {
	cases := []struct {
		name    string
		size    int
		wantErr bool
	}{
		{"under limit", 1024, false},
		{"exactly at limit", maxResponseBytes, false},
		{"one byte over", maxResponseBytes + 1, true},
		{"far over", maxResponseBytes * 2, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, err := readCappedBody(bytes.NewReader(make([]byte, tc.size)))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("size %d: got nil error, want error (silent truncation!)", tc.size)
				}
				if !strings.Contains(err.Error(), "exceeds") {
					t.Errorf("error should mention the limit, got: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("size %d: unexpected error: %v", tc.size, err)
			}
			if len(body) != tc.size {
				t.Errorf("size %d: got %d bytes back, want %d", tc.size, len(body), tc.size)
			}
		})
	}
}
