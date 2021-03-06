package main

import (
	"context"
	"os"
	"os/signal"
	"time"

	"github.com/pkg/errors"

	"github.com/spf13/cobra"

	"github.com/havoc-io/mutagen/cmd"
	"github.com/havoc-io/mutagen/pkg/agent"
	"github.com/havoc-io/mutagen/pkg/local"
	"github.com/havoc-io/mutagen/pkg/remote"
)

const (
	// housekeepingInterval is the interval at which housekeeping will be
	// invoked by the agent.
	housekeepingInterval = 24 * time.Hour
)

// housekeep performs a combined housekeeping operation.
func housekeep() {
	// Perform agent housekeeping.
	agent.Housekeep()

	// Perform cache housekeeping.
	local.HousekeepCaches()

	// Perform staging directory housekeeping.
	local.HousekeepStaging()
}

func housekeepRegularly(context context.Context) {
	// Perform an initial housekeeping operation since the ticker won't fire
	// straight away.
	housekeep()

	// Create a ticker to regulate housekeeping and defer its shutdown.
	ticker := time.NewTicker(housekeepingInterval)
	defer ticker.Stop()

	// Loop and wait for the ticker or cancellation.
	for {
		select {
		case <-context.Done():
			return
		case <-ticker.C:
			housekeep()
		}
	}
}

func endpointMain(command *cobra.Command, arguments []string) error {
	// Set up regular housekeeping and defer its shutdown.
	housekeepingContext, housekeepingCancel := context.WithCancel(context.Background())
	defer housekeepingCancel()
	go housekeepRegularly(housekeepingContext)

	// Create a connection on standard input/output.
	connection := newStdioConnection()

	// Serve an endpoint on standard input/output and monitor for its
	// termination.
	endpointTermination := make(chan error, 1)
	go func() {
		endpointTermination <- remote.ServeEndpoint(connection)
	}()

	// Wait for termination from a signal or the endpoint.
	signalTermination := make(chan os.Signal, 1)
	signal.Notify(signalTermination, cmd.TerminationSignals...)
	select {
	case sig := <-signalTermination:
		return errors.Errorf("terminated by signal: %s", sig)
	case err := <-endpointTermination:
		return errors.Wrap(err, "endpoint terminated")
	}
}

var endpointCommand = &cobra.Command{
	Use:   agent.ModeEndpoint,
	Short: "Run the agent in endpoint mode",
	Run:   cmd.Mainify(endpointMain),
}

var endpointConfiguration struct {
	help bool
}

func init() {
	// Bind flags to configuration. We manually add help to override the default
	// message, but Cobra still implements it automatically.
	flags := endpointCommand.Flags()
	flags.BoolVarP(&endpointConfiguration.help, "help", "h", false, "Show help information")
}
