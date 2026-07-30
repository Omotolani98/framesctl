package commands

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"charm.land/bubbles/v2/progress"
	"github.com/Omotolani98/framesctl/internals/framesrvr"
	"github.com/charmbracelet/x/ansi"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const progressRenderInterval = 100 * time.Millisecond

const (
	defaultProgressWidth = 40
	minimumProgressWidth = 10
	maximumProgressWidth = 80
)

type uploadProgressRenderer struct {
	output     io.Writer
	bar        progress.Model
	filename   string
	lastRender time.Time
	wrote      bool
	disabled   bool
}

func uploadVideo(
	cmd *cobra.Command,
	client *framesrvr.Client,
	path string,
) (*framesrvr.UploadResponse, error) {
	if !isTerminal(cmd.OutOrStdout()) {
		return client.UploadVideo(cmd.Context(), path, nil)
	}

	renderer := newUploadProgressRenderer(cmd.OutOrStdout(), path)
	upload, err := client.UploadVideo(
		cmd.Context(),
		path,
		renderer.render,
	)
	renderer.finish()

	return upload, err
}

func newUploadProgressRenderer(
	output io.Writer,
	path string,
) *uploadProgressRenderer {
	return &uploadProgressRenderer{
		output: output,
		bar: progress.New(
			progress.WithDefaultBlend(),
			progress.WithWidth(defaultProgressWidth),
		),
		filename: filepath.Base(path),
	}
}

func (renderer *uploadProgressRenderer) render(
	read int64,
	total int64,
) {
	if renderer.disabled || total <= 0 {
		return
	}

	now := time.Now()
	if read < total &&
		now.Sub(renderer.lastRender) < progressRenderInterval {
		return
	}

	filename := renderer.filename
	bytesText := fmt.Sprintf(
		"%s / %s",
		humanize.IBytes(uint64(read)),
		humanize.IBytes(uint64(total)),
	)
	suffix := ""
	if read >= total {
		suffix = " · processing…"
	}

	width := terminalWidth(renderer.output)
	barWidth := uploadProgressWidth(
		width,
		filename,
		bytesText,
		suffix,
	)
	if width > 0 && barWidth < minimumProgressWidth {
		filename = truncateProgressFilename(
			width,
			filename,
			bytesText,
			suffix,
		)
		barWidth = uploadProgressWidth(
			width,
			filename,
			bytesText,
			suffix,
		)
	}

	renderer.bar.SetWidth(max(barWidth, minimumProgressWidth))

	_, err := fmt.Fprintf(
		renderer.output,
		"\r\x1b[2KUploading %s %s %s%s",
		filename,
		renderer.bar.ViewAs(float64(read)/float64(total)),
		bytesText,
		suffix,
	)
	if err != nil {
		renderer.disabled = true

		return
	}

	renderer.lastRender = now
	renderer.wrote = true
}

func (renderer *uploadProgressRenderer) finish() {
	if !renderer.wrote {
		return
	}

	_, _ = io.WriteString(renderer.output, "\n")
}

func uploadProgressWidth(
	terminalWidth int,
	filename string,
	bytesText string,
	suffix string,
) int {
	if terminalWidth <= 0 {
		return defaultProgressWidth
	}

	fixedWidth := ansi.StringWidth("Uploading "+filename) +
		ansi.StringWidth(bytesText) +
		ansi.StringWidth(suffix) +
		2 // Spaces around the progress bar.
	width := terminalWidth - fixedWidth

	return min(width, maximumProgressWidth)
}

func truncateProgressFilename(
	terminalWidth int,
	filename string,
	bytesText string,
	suffix string,
) string {
	fixedWidth := ansi.StringWidth("Uploading ") +
		minimumProgressWidth +
		ansi.StringWidth(bytesText) +
		ansi.StringWidth(suffix) +
		2 // Spaces around the progress bar.
	filenameWidth := terminalWidth - fixedWidth
	if filenameWidth <= 1 {
		return "…"
	}

	return ansi.Truncate(filename, filenameWidth, "…")
}

func isTerminal(writer io.Writer) bool {
	file, ok := writer.(*os.File)

	return ok &&
		term.IsTerminal(int(file.Fd())) &&
		os.Getenv("TERM") != "dumb"
}
