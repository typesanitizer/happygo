package cmdx

import (
	"al.essio.dev/pkg/shellescape"

	. "github.com/typesanitizer/happygo/common/core"
)

// Cmd describes a command invocation.
type Cmd struct {
	name string
	args []string
	dir  Option[string]
}

// New creates a Cmd from argv (name followed by arguments).
func New(argv ...string) Cmd {
	return Cmd{name: argv[0], args: argv[1:], dir: None[string]()}
}

// In returns a copy of the Cmd with the working directory set.
func (cmd Cmd) In(dir string) Cmd {
	cmd.dir = Some(dir)
	return cmd
}

// Dir returns the optional working directory.
func (cmd Cmd) Dir() Option[string] { return cmd.dir }

// Argv returns the full command as [name, args...].
func (cmd Cmd) Argv() []string {
	return append([]string{cmd.name}, cmd.args...)
}

// String returns a shell-escaped representation of the command.
func (cmd Cmd) String() string {
	return shellescape.QuoteCommand(cmd.Argv())
}
