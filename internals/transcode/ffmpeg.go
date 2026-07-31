package transcode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
)

const maxCommandOutput = 1 << 20

type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	buffer := &limitedBuffer{limit: maxCommandOutput}
	command.Stdout = buffer
	command.Stderr = buffer

	if err := command.Run(); err != nil {
		return buffer.Bytes(), fmt.Errorf("run %s: %w: %s", name, err, buffer.String())
	}

	return buffer.Bytes(), nil
}

func Probe(
	ctx context.Context,
	runner CommandRunner,
	ffprobePath string,
	inputPath string,
) (SourceInfo, error) {
	output, err := runner.Run(
		ctx,
		ffprobePath,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height",
		"-of", "json",
		inputPath,
	)
	if err != nil {
		return SourceInfo{}, err
	}

	var payload struct {
		Streams []SourceInfo `json:"streams"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		return SourceInfo{}, fmt.Errorf("decode ffprobe output: %w", err)
	}

	if len(payload.Streams) == 0 {
		return SourceInfo{}, fmt.Errorf("ffprobe found no video stream")
	}

	return payload.Streams[0], nil
}

func RunFFmpeg(
	ctx context.Context,
	runner CommandRunner,
	ffmpegPath string,
	inputPath string,
	outputDir string,
	renditions []Rendition,
) error {
	for _, rendition := range renditions {
		playlistPath := filepath.Join(outputDir, rendition.Name, "index.m3u8")
		args := BuildFFmpegArgs(inputPath, outputDir, rendition)
		if _, err := runner.Run(ctx, ffmpegPath, args...); err != nil {
			return fmt.Errorf("transcode %s: %w", rendition.Name, err)
		}

		if playlistPath == "" {
			return fmt.Errorf("empty playlist path")
		}
	}

	return writeMasterPlaylist(outputDir, renditions)
}

func BuildFFmpegArgs(inputPath string, outputDir string, rendition Rendition) []string {
	playlistPath := filepath.Join(outputDir, rendition.Name, "index.m3u8")
	segmentPath := filepath.Join(outputDir, rendition.Name, "segment-%05d.m4s")

	return []string{
		"-y",
		"-i", inputPath,
		"-map", "0:v:0",
		"-map", "0:a:0?",
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-profile:v", "main",
		"-pix_fmt", "yuv420p",
		"-vf", fmt.Sprintf(
			"scale=%d:%d:force_original_aspect_ratio=decrease",
			rendition.Width,
			rendition.Height,
		),
		"-b:v", rendition.VideoBitrate,
		"-maxrate", rendition.VideoBitrate,
		"-bufsize", rendition.VideoBitrate,
		"-c:a", "aac",
		"-b:a", "128k",
		"-f", "hls",
		"-hls_time", "6",
		"-hls_playlist_type", "vod",
		"-hls_segment_type", "fmp4",
		"-hls_flags", "independent_segments",
		"-hls_fmp4_init_filename", "init.mp4",
		"-hls_segment_filename", segmentPath,
		playlistPath,
	}
}

type limitedBuffer struct {
	buffer bytes.Buffer
	limit  int
}

func (buffer *limitedBuffer) Write(value []byte) (int, error) {
	if buffer.buffer.Len() < buffer.limit {
		remaining := buffer.limit - buffer.buffer.Len()
		buffer.buffer.Write(value[:min(len(value), remaining)])
	}

	return len(value), nil
}

func (buffer *limitedBuffer) Bytes() []byte {
	return buffer.buffer.Bytes()
}

func (buffer *limitedBuffer) String() string {
	return buffer.buffer.String()
}
