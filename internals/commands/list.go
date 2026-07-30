package commands

import (
	"fmt"
	"io"
	"os"

	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"
	"github.com/Omotolani98/framesctl/internals/db"
	"github.com/charmbracelet/x/ansi"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const fallbackTableWidth = 100

var uploadTableColumnTitles = []string{
	"FILENAME",
	"SIZE",
	"TYPE",
	"UPLOADED",
	"URL",
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List uploaded videos",
		Args:    cobra.NoArgs,
		RunE:    runList,
	}
}

func runList(cmd *cobra.Command, args []string) error {
	uploads, err := db.ListUploads(cmd.Context())
	if err != nil {
		debugLogger.Error("list uploads failed", "err", err)

		return err
	}

	debugLogger.Debug("uploads listed", "count", len(uploads))

	if len(uploads) == 0 {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "No uploads yet.")

		return err
	}

	rendered := renderUploadsTable(
		uploads,
		terminalWidth(cmd.OutOrStdout()),
	)
	if !isTerminal(cmd.OutOrStdout()) {
		rendered = ansi.Strip(rendered)
	}

	_, err = fmt.Fprintln(cmd.OutOrStdout(), rendered)

	return err
}

func renderUploadsTable(
	uploads []db.Upload,
	width int,
) string {
	widths := uploadTableColumnWidths(width)
	columns := make([]table.Column, 0, len(widths))

	for index, title := range uploadTableColumnTitles {
		columns = append(columns, table.Column{
			Title: title,
			Width: widths[index],
		})
	}

	rows := make([]table.Row, 0, len(uploads))
	for _, upload := range uploads {
		rows = append(rows, table.Row{
			upload.Filename,
			humanize.IBytes(uint64(upload.ContentLength)),
			upload.ContentType,
			upload.CreatedAt.Local().Format("2006-01-02 15:04"),
			upload.PublicURL,
		})
	}

	styles := table.DefaultStyles()
	styles.Selected = lipgloss.NewStyle()

	uploadsTable := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithHeight(len(rows)+1),
		table.WithWidth(width),
		table.WithStyles(styles),
	)

	return uploadsTable.View()
}

func uploadTableColumnWidths(terminalWidth int) []int {
	widths := []int{24, 10, 13, 19, 36}
	minimums := []int{8, 6, 8, 10, 12}
	available := terminalWidth - 2*len(widths)
	shrinkOrder := []int{0, 2, 1, 3, 4}

	for _, column := range shrinkOrder {
		excess := sum(widths) - available
		if excess <= 0 {
			break
		}

		widths[column] -= min(
			excess,
			widths[column]-minimums[column],
		)
	}

	return widths
}

func sum(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}

	return total
}

func terminalWidth(writer io.Writer) int {
	file, ok := writer.(*os.File)
	if !ok {
		return fallbackTableWidth
	}

	width, _, err := term.GetSize(int(file.Fd()))
	if err != nil || width <= 0 {
		return fallbackTableWidth
	}

	return width
}
