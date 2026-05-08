package repo

import (
	"testing"
)

func TestHttpsToSSH(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "HTTPS without .git suffix",
			in:   "https://github.com/owner/repo",
			want: "git@github.com:owner/repo.git",
		},
		{
			name: "HTTPS with .git suffix",
			in:   "https://github.com/owner/repo.git",
			want: "git@github.com:owner/repo.git",
		},
		{
			name: "SSH URL unchanged",
			in:   "git@github.com:owner/repo.git",
			want: "git@github.com:owner/repo.git",
		},
		{
			name: "non-GitHub HTTPS URL unchanged",
			in:   "https://gitlab.com/owner/repo.git",
			want: "https://gitlab.com/owner/repo.git",
		},
		{
			name: "empty string unchanged",
			in:   "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := httpsToSSH(tt.in)
			if got != tt.want {
				t.Errorf("httpsToSSH(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
