package handler

import (
	"reflect"
	"testing"
)

func TestSanitizeCandidates(t *testing.T) {
	tests := []struct {
		name    string
		in      []string
		want    []string
		wantErr bool
	}{
		{name: "empty", in: nil, want: []string{}},
		{name: "trims and removes blanks", in: []string{"  foo  ", "", "bar"}, want: []string{"foo", "bar"}},
		{name: "dedupes", in: []string{"foo", "foo", "bar"}, want: []string{"foo", "bar"}},
		{name: "cleans path", in: []string{"./foo/", "bar/./baz"}, want: []string{"foo", "bar/baz"}},
		{name: "rejects absolute", in: []string{"/etc/passwd"}, wantErr: true},
		{name: "rejects parent", in: []string{".."}, wantErr: true},
		{name: "rejects ../prefix", in: []string{"../leak"}, wantErr: true},
		{name: "rejects embedded ..", in: []string{"foo/../../bar"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sanitizeCandidates(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
