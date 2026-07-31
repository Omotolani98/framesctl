package transcode

import (
	"slices"
	"strings"
	"testing"
)

func TestBuildFFmpegArgs(t *testing.T) {
	args := BuildFFmpegArgs(
		"/tmp/source.mp4",
		"/tmp/hls",
		Rendition{
			Name:         "720p",
			Width:        1280,
			Height:       720,
			VideoBitrate: "2800k",
		},
	)

	joined := strings.Join(args, " ")
	for _, want := range []string{
		"-i /tmp/source.mp4",
		"-map 0:v:0",
		"-map 0:a:0?",
		"scale=1280:720:force_original_aspect_ratio=decrease",
		"-hls_segment_type fmp4",
		"/tmp/hls/720p/index.m3u8",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q in %q", want, joined)
		}
	}

	if !slices.Contains(args, "independent_segments") {
		t.Errorf("args missing independent_segments: %#v", args)
	}
}
