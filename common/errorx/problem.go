package errorx

import (
	"fmt"
	"slices"
	"sync"

	"github.com/typesanitizer/happygo/common/assert"
)

// Code classifies a structured non-stack error.
type Code string

const (
	Code_InvalidArgument Code = "invalid argument"
	Code_AccessDenied    Code = "access denied"
)

var allCodes = sync.OnceValue(func() []Code {
	return []Code{Code_InvalidArgument, Code_AccessDenied}
})

func isValidCode(code Code) bool {
	return slices.Contains(allCodes(), code)
}

// Problem is a structured error with a stable code and message.
type Problem struct {
	code Code
	msg  string
}

// NewProblem returns a Problem with the given code and message.
func NewProblem(code Code, msg string) *Problem {
	assert.Preconditionf(isValidCode(code), "invalid errorx.Code %q", code)
	return &Problem{code: code, msg: msg}
}

func (p *Problem) Error() string {
	return fmt.Sprintf("%s (%s)", p.msg, p.code)
}

func (p *Problem) Code() Code {
	return p.code
}

func (p *Problem) Msg() string {
	return p.msg
}
