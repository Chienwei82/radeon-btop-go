package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gputop/gputop-go/internal/model"
)

// TestScreenNeverExceedsItsHeight is the height counterpart of
// TestScreenFitsExplicitDimensions: a client list big enough to fill the
// process table must still leave the status line and key hints on screen.
func TestScreenNeverExceedsItsHeight(t *testing.T) {
	in := screenInput()
	procs := make([]model.GpuProcess, 0, 10)
	for i := 0; i < 10; i++ {
		procs = append(procs, testProcessNamed(2000+i, fmt.Sprintf("client-%d", i)))
	}
	in.Snapshot.Processes = procs
	in.Snapshot.VisibleProcessCount = len(procs)
	in.Snapshot.TotalProcessCount = len(procs)

	for _, width := range []int{64, 120} {
		for _, height := range []int{20, 27, 28, 34, 40, 48, 52, 60} {
			in.Width, in.Height = width, height
			rows := RenderScreen(in)
			if len(rows) > height {
				t.Fatalf("%dx%d renders %d rows, want at most %d", width, height, len(rows), height)
			}
			if got := Plain(rows[len(rows)-1]); !strings.Contains(got, "q quit") {
				t.Fatalf("%dx%d lost the key-hint footer; last row = %q", width, height, got)
			}
		}
	}
}
