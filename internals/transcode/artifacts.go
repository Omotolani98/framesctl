package transcode

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type ObjectStore interface {
	Download(ctx context.Context, key string, destination io.Writer) error
	UploadFile(ctx context.Context, key string, contentType string, path string) error
}

type Artifact struct {
	Path        string
	Key         string
	ContentType string
}

func writeMasterPlaylist(outputDir string, renditions []Rendition) error {
	var builder strings.Builder
	builder.WriteString("#EXTM3U\n")
	builder.WriteString("#EXT-X-VERSION:7\n")

	for _, rendition := range renditions {
		builder.WriteString(fmt.Sprintf(
			"#EXT-X-STREAM-INF:BANDWIDTH=%d,RESOLUTION=%dx%d\n",
			rendition.Bandwidth,
			rendition.Width,
			rendition.Height,
		))
		builder.WriteString(rendition.Name + "/index.m3u8\n")
	}

	return os.WriteFile(
		filepath.Join(outputDir, "master.m3u8"),
		[]byte(builder.String()),
		0o600,
	)
}

func DiscoverArtifacts(outputDir string, keyPrefix string) ([]Artifact, error) {
	artifacts := make([]Artifact, 0)
	err := filepath.WalkDir(outputDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			return nil
		}

		relative, err := filepath.Rel(outputDir, path)
		if err != nil {
			return err
		}

		artifacts = append(artifacts, Artifact{
			Path:        path,
			Key:         keyPrefix + "/" + filepath.ToSlash(relative),
			ContentType: artifactContentType(path),
		})

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("discover HLS artifacts: %w", err)
	}

	return artifacts, nil
}

func artifactContentType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".m3u8":
		return "application/vnd.apple.mpegurl"
	case ".m4s":
		return "video/iso.segment"
	case ".mp4":
		return "video/mp4"
	default:
		return "application/octet-stream"
	}
}
