package node

import (
	"fmt"
	"testing"
)

func TestSanitizeUploadPath(t *testing.T) {
	cases := []struct {
		input string
		want  string
		err   bool
	}{
		{"file.txt", "file.txt", false},
		{"dir/file.txt", "dir/file.txt", false},
		{"/etc/passwd", "", true},
		{"../etc/passwd", "", true},
		{"dir/../../etc/passwd", "", true},
		{"", "", true},
		{"./file.txt", "file.txt", false},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("%q", tc.input), func(t *testing.T) {
			got, err := sanitizeUploadPath(tc.input)
			if tc.err {
				if err == nil {
					t.Fatalf("expected error for %q, got %q", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("sanitizeUploadPath(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
