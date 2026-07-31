// Package transcode converts uploaded videos into streamable HLS output.
package transcode

type SourceInfo struct {
	Width  int
	Height int
}

type Rendition struct {
	Name         string
	Width        int
	Height       int
	VideoBitrate string
	Bandwidth    int
}

var defaultLadder = []Rendition{
	{Name: "360p", Width: 640, Height: 360, VideoBitrate: "800k", Bandwidth: 928000},
	{Name: "480p", Width: 854, Height: 480, VideoBitrate: "1400k", Bandwidth: 1528000},
	{Name: "720p", Width: 1280, Height: 720, VideoBitrate: "2800k", Bandwidth: 2928000},
	{Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: "5000k", Bandwidth: 5128000},
}

func SelectRenditions(source SourceInfo) []Rendition {
	if source.Width <= 0 || source.Height <= 0 {
		return []Rendition{defaultLadder[0]}
	}

	longest := max(source.Width, source.Height)
	selected := make([]Rendition, 0, len(defaultLadder))
	for _, rendition := range defaultLadder {
		if max(rendition.Width, rendition.Height) <= longest {
			selected = append(selected, rendition)
		}
	}

	if len(selected) == 0 {
		return []Rendition{defaultLadder[0]}
	}

	return selected
}
