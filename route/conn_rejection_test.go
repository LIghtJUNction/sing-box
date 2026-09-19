package route

import (
	"context"
	"fmt"
	"syscall"
	"testing"

	R "github.com/sagernet/sing-box/route/rule"
	"github.com/sagernet/sing/common/logger"
)

type rejectionLogger struct {
	logger.ContextLogger
	debug, errors int
}

func (l *rejectionLogger) DebugContext(context.Context, ...any) { l.debug++ }
func (l *rejectionLogger) ErrorContext(context.Context, ...any) { l.errors++ }

func TestDialErrorsOnlyDowngradeExplicitPolicyRejections(t *testing.T) {
	l := new(rejectionLogger)
	m := NewConnectionManager(l)
	m.logDialError(context.Background(), fmt.Errorf("selector: %w", &R.RejectedError{Cause: syscall.EPERM}))
	m.logDialError(context.Background(), fmt.Errorf("socket: %w", syscall.EPERM))
	m.logDialError(context.Background(), context.DeadlineExceeded)
	if l.debug != 1 || l.errors != 2 {
		t.Fatalf("debug=%d errors=%d", l.debug, l.errors)
	}
}
