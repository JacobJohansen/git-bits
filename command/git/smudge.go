package gitcommand

import (
	"fmt"
	"os"

	"github.com/mitchellh/cli"
)

type Smudge struct {
	ui cli.Ui
}

func NewSmudge() (cmd cli.Command, err error) {
	return &Smudge{
		ui: &cli.BasicUi{
			Reader:      os.Stdin,
			Writer:      os.Stderr,
			ErrorWriter: os.Stderr,
		},
	}, nil
}

// Help returns long-form help text that includes the command-line
// usage, a brief few sentences explaining the function of the command,
// and the complete list of flags the command accepts.
func (cmd *Smudge) Help() string {
	return fmt.Sprintf(`
  %s
`, cmd.Synopsis())
}

// Synopsis returns a one-line, short synopsis of the command.
// This should be less than 50 characters ideally.
func (cmd *Smudge) Synopsis() string { return "..." }

// Run runs the actual command with the given CLI instance and
// command-line arguments. It returns the exit status when it is
// finished.
func (cmd *Smudge) Run(args []string) int {

	return 0
}
