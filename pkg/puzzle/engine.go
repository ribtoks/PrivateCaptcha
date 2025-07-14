package puzzle

import (
	"context"
	"net/http"
	"time"
)

type VerifyResult struct {
	Error     VerifyError
	CreatedAt time.Time
	Domain    string
}

func (vr *VerifyResult) Success() bool {
	return (vr.Error == VerifyNoError) ||
		(vr.Error == MaintenanceModeError) ||
		(vr.Error == TestPropertyError)
}

func (vr *VerifyResult) SetError(verr VerifyError) {
	vr.Error = verr
}

func (vr *VerifyResult) ErrorString() string {
	if vr.Error == VerifyNoError {
		return ""
	}

	return vr.Error.String()
}

func (vr *VerifyResult) ErrorsToStrings() []string {
	if vr.Error == VerifyNoError {
		return []string{}
	}

	result := make([]string, 0, 1)

	if vr.Error != VerifyNoError {
		result = append(result, vr.Error.String())
	}

	return result
}

type Engine interface {
	Write(ctx context.Context, p *Puzzle, extraSalt []byte, w http.ResponseWriter) error
	Verify(ctx context.Context, payload []byte, expectedOwner OwnerIDSource, tnow time.Time) (*VerifyResult, error)
}
