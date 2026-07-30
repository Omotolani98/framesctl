package commands

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestUploadProgressRendererFitsFallbackWidth(t *testing.T) {
	output := &strings.Builder{}
	renderer := newUploadProgressRenderer(
		output,
		"very-long-upload-filename-that-must-not-wrap.mp4",
	)

	renderer.render(10<<20, 10<<20)

	visible := ansi.Strip(output.String())
	visible = strings.TrimPrefix(visible, "\r")

	if strings.Contains(visible, "\n") {
		t.Fatalf("progress line wrapped: %q", visible)
	}

	if width := ansi.StringWidth(visible); width > fallbackTableWidth {
		t.Fatalf(
			"progress width = %d, want <= %d: %q",
			width,
			fallbackTableWidth,
			visible,
		)
	}

	if !strings.Contains(visible, "processing") {
		t.Fatalf("progress line missing processing state: %q", visible)
	}
}
