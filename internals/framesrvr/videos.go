package framesrvr

import (
	"sort"
	"strings"
)

type VideoType struct {
	ContentType       string
	DetectedMIMETypes map[string]struct{}
}

var videoTypes = map[string]VideoType{
	".mp4": {
		ContentType: "video/mp4",
		DetectedMIMETypes: mimeSet(
			"video/mp4",
			"application/octet-stream",
		),
	},
	".mov": {
		ContentType: "video/quicktime",
		DetectedMIMETypes: mimeSet(
			"video/quicktime",
			"video/mp4",
			"application/octet-stream",
		),
	},
	".webm": {
		ContentType: "video/webm",
		DetectedMIMETypes: mimeSet(
			"video/webm",
			"application/octet-stream",
		),
	},
	".mkv": {
		ContentType: "video/x-matroska",
		DetectedMIMETypes: mimeSet(
			"video/x-matroska",
			"application/octet-stream",
		),
	},
	".avi": {
		ContentType: "video/x-msvideo",
		DetectedMIMETypes: mimeSet(
			"video/x-msvideo",
			"video/avi",
			"application/octet-stream",
		),
	},
	".m4v": {
		ContentType: "video/x-m4v",
		DetectedMIMETypes: mimeSet(
			"video/x-m4v",
			"video/mp4",
			"application/octet-stream",
		),
	},
}

func LookupVideoType(extension string) (VideoType, bool) {
	videoType, ok := videoTypes[strings.ToLower(extension)]
	return videoType, ok
}

func AllowedExtensions() []string {
	extensions := make([]string, 0, len(videoTypes))

	for extension := range videoTypes {
		extensions = append(extensions, extension)
	}

	sort.Strings(extensions)

	return extensions
}

func AllowedExtensionsText() string {
	return strings.Join(AllowedExtensions(), ", ")
}

func mimeSet(values ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))

	for _, value := range values {
		set[value] = struct{}{}
	}

	return set
}
