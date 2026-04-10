package envx_path

import "os/exec"

// ErrDot is re-exported from os/exec for callers comparing errors returned by
// FindExecutable when a relative PATH entry resolves to a matching executable.
var ErrDot = exec.ErrDot
