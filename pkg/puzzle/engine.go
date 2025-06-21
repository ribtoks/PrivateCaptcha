package puzzle

import (
	"context"
	"net/http"
	"time"
)

type VerifyResult struct {
	Errors    []VerifyError
	CreatedAt time.Time
	Domain    string
}

func (vr *VerifyResult) Success() bool {
	return (len(vr.Errors) == 0) ||
		((len(vr.Errors) == 1) &&
			(vr.Errors[0] == VerifyNoError) ||
			(vr.Errors[0] == MaintenanceModeError) ||
			(vr.Errors[0] == TestPropertyError))
}

func (vr *VerifyResult) AddError(verr VerifyError) {
	if verr != VerifyNoError {
		vr.Errors = append(vr.Errors, verr)
	}
}

func (vr *VerifyResult) ErrorsToStrings() []string {
	if len(vr.Errors) == 0 {
		return []string{}
	}

	result := make([]string, 0, len(vr.Errors))

	for _, err := range vr.Errors {
		if err != VerifyNoError {
			result = append(result, err.String())
		}
	}

	return result
}

type Engine interface {
	Write(ctx context.Context, p *Puzzle, extraSalt []byte, w http.ResponseWriter) error
	Verify(ctx context.Context, payload []byte, expectedOwner OwnerIDSource, tnow time.Time) (*VerifyResult, error)
}
