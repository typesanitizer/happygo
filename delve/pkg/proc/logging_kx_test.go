package proc_test

import (
	"fmt"
	"os"
	"time"

	"github.com/go-delve/delve/pkg/proc"
	"github.com/go-delve/delve/pkg/proc/gdbserial"
)

func kxLogTestf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[happygo-kx][proc_test][%s] %s\n", time.Now().UTC().Format(time.RFC3339Nano), fmt.Sprintf(format, args...))
}

func describeGroupKX(grp *proc.TargetGroup) string {
	if grp == nil {
		return "grp=<nil>"
	}
	if grp.Selected == nil {
		return fmt.Sprintf("grp=%p selected=<nil>", grp)
	}
	return fmt.Sprintf("grp=%p selectedTarget=%p selectedPID=%d", grp, grp.Selected, grp.Selected.Pid())
}

func describeWaitForKX(waitFor *proc.WaitFor) string {
	if waitFor == nil {
		return "waitFor=<nil>"
	}
	return fmt.Sprintf("waitFor{name=%q interval=%s duration=%s}", waitFor.Name, waitFor.Interval, waitFor.Duration)
}

func logProcessRegistryKX(reason string) {
	gdbserial.LogKXProcessRegistry(reason)
}
