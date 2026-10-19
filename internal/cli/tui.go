package cli

import (
	"os"
	"os/signal"
	"syscall"

	tea "charm.land/bubbletea/v2"

	"github.com/gputop/gputop-go/internal/config"
	"github.com/gputop/gputop-go/internal/readers"
	"github.com/gputop/gputop-go/internal/sampler"
	"github.com/gputop/gputop-go/internal/sessionlog"
	"github.com/gputop/gputop-go/internal/ui"
)

// TUI carries everything the interface needs to run.
type TUI struct {
	Config  config.Config
	Sampler sampler.SamplerOptions
	Blocks  *readers.RadeontopPool
	Log     *sessionlog.SessionLog
}

// RunTUI starts the sampler and the supervised child, then hands the terminal
// to bubbletea until the user quits.
//
// The event loop's Shutdown is called again after the program returns, so a
// SIGTERM delivered straight to the process still saves the session state:
// bubbletea's quit path is only one of the ways out.
func RunTUI(in TUI) error {
	s := sampler.New(in.Sampler)
	if in.Blocks != nil {
		s = s.WithBlocks(in.Blocks)
	}
	s.Start()
	if in.Blocks != nil {
		in.Blocks.Start()
	}
	// Stop in the reverse order they were started, and only once: the app's
	// Shutdown also stops the sampler on a normal quit.
	defer func() {
		s.Stop()
		if in.Blocks != nil {
			in.Blocks.Stop()
		}
		if in.Log != nil {
			in.Log.Close()
		}
	}()

	app := ui.NewApp(ui.AppOptions{
		Config:  in.Config,
		Sampler: s,
		Log:     in.Log,
	})
	program := tea.NewProgram(app)

	// A signal has no key press to attach to, so the only safe response is to
	// ask the program to quit: Shutdown then runs on the way out.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	go func() {
		if _, ok := <-signals; ok {
			program.Quit()
		}
	}()

	if _, err := program.Run(); err != nil {
		app.Shutdown()
		return err
	}
	app.Shutdown()
	return nil
}
