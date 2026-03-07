package cmdx

import (
	"al.essio.dev/pkg/shellescape"

	. "github.com/typesanitizer/happygo/common/core"
)

// Cmd describes a command invocation.
type Cmd struct {
	// Name is the executable name. Always non-empty.
	Name string
	// Args are positional arguments passed to Name.
	Args []string
	// Dir optionally sets the process working directory.
	Dir Option[string]
}

// String returns a shell-escaped representation of the command.
func (cmd Cmd) String() string {
	return shellescape.QuoteCommand(append([]string{cmd.Name}, cmd.Args...))
}
