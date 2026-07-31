package transcode

import "testing"

func TestSelectRenditionsDoesNotUpscale(t *testing.T) {
	renditions := SelectRenditions(SourceInfo{Width: 1280, Height: 720})

	if len(renditions) != 3 {
		t.Fatalf("rendition count = %d, want 3", len(renditions))
	}

	if got := renditions[len(renditions)-1].Name; got != "720p" {
		t.Errorf("largest rendition = %q, want 720p", got)
	}
}

func TestSelectRenditionsKeepsSmallVideoPlayable(t *testing.T) {
	renditions := SelectRenditions(SourceInfo{Width: 320, Height: 180})

	if len(renditions) != 1 || renditions[0].Name != "360p" {
		t.Fatalf("renditions = %#v, want 360p fallback", renditions)
	}
}

func TestSelectRenditionsHandlesPortraitVideo(t *testing.T) {
	renditions := SelectRenditions(SourceInfo{Width: 1080, Height: 1920})

	if got := renditions[len(renditions)-1].Name; got != "1080p" {
		t.Errorf("largest portrait rendition = %q, want 1080p", got)
	}
}
