package commands

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("home dir: %v", err)
	}

	tests := []struct {
		input string
		want  string
	}{
		{"~/video.mp4", filepath.Join(home, "video.mp4")},
		{"~", home},
		{"/tmp/video.mp4", "/tmp/video.mp4"},
		{"relative/video.mp4", "relative/video.mp4"},
	}

	for _, test := range tests {
		got, err := expandPath(test.input)
		if err != nil {
			t.Fatalf("expandPath(%q): %v", test.input, err)
		}

		if got != test.want {
			t.Errorf(
				"expandPath(%q) = %q, want %q",
				test.input,
				got,
				test.want,
			)
		}
	}
}

func TestValidateVideoFile(t *testing.T) {
	directory := t.TempDir()

	valid := filepath.Join(directory, "clip.mp4")
	if err := os.WriteFile(valid, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	empty := filepath.Join(directory, "empty.mp4")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	badExtension := filepath.Join(directory, "notes.txt")
	if err := os.WriteFile(badExtension, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{"valid", valid, ""},
		{"missing", filepath.Join(directory, "missing.mp4"), "does not exist"},
		{"directory", directory, "directory"},
		{"empty", empty, "empty"},
		{"bad extension", badExtension, "unsupported video extension"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateVideoFile(test.path)

			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				return
			}

			if err == nil ||
				!strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf(
					"error = %v, want substring %q",
					err,
					test.wantErr,
				)
			}
		})
	}
}

func TestNewURLKey(t *testing.T) {
	key, err := newURLKey()
	if err != nil {
		t.Fatalf("newURLKey: %v", err)
	}

	decoded, err := base64.RawURLEncoding.DecodeString(key)
	if err != nil {
		t.Fatalf("key %q is not url-safe base64: %v", key, err)
	}

	if len(decoded) != 16 {
		t.Errorf("decoded length = %d, want 16", len(decoded))
	}

	other, err := newURLKey()
	if err != nil {
		t.Fatalf("newURLKey: %v", err)
	}

	if key == other {
		t.Error("expected unique keys")
	}
}
