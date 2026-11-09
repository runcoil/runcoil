package runcoil

import (
	"errors"

	"google.golang.org/protobuf/types/known/timestamppb"

	runcoilv1 "github.com/runcoil/runcoil/proto/gen/go/runcoil/v1"
	"github.com/runcoil/runcoil/sdks/go/internal/runstate"
)

// runResultToProto turns a handler's output or error into the result CompleteRun sends.
func runResultToProto(out []byte, err error) *runcoilv1.CompleteRunRequest {
	if err == nil {
		return &runcoilv1.CompleteRunRequest{Result: &runcoilv1.CompleteRunRequest_Output{Output: out}}
	}

	var se *runstate.StepError
	if errors.As(err, &se) {
		// The handler returned a Step's final error.
		return runFailureToProto(runcoilv1.ErrorKind_ERROR_KIND_ERROR, err.Error(), se.StepID)
	}

	var pe *panicError
	if errors.As(err, &pe) {
		return runFailureToProto(runcoilv1.ErrorKind_ERROR_KIND_PANIC, pe.Error(), "")
	}

	var ee *encodeError
	if errors.As(err, &ee) {
		return runFailureToProto(runcoilv1.ErrorKind_ERROR_KIND_OUTPUT_NOT_ENCODABLE, ee.Error(), "")
	}

	return runFailureToProto(runcoilv1.ErrorKind_ERROR_KIND_ERROR, err.Error(), "")
}

// runFailureToProto builds a result that fails the Run. stepID is set when the handler
// returned the final error of that Step.
func runFailureToProto(kind runcoilv1.ErrorKind, message, stepID string) *runcoilv1.CompleteRunRequest {
	return &runcoilv1.CompleteRunRequest{Result: &runcoilv1.CompleteRunRequest_Error{
		Error: &runcoilv1.RunError{Kind: kind, Message: message, StepId: stepID},
	}}
}

func stepReportToProto(lease *runcoilv1.Lease, r runstate.StepReport) *runcoilv1.CompleteStepRequest {
	req := &runcoilv1.CompleteStepRequest{
		Lease:     lease,
		StepId:    r.StepID,
		StartedAt: timestamppb.New(r.StartedAt),
		EndedAt:   timestamppb.New(r.EndedAt),
	}

	if r.Failure != nil {
		req.Outcome = &runcoilv1.CompleteStepRequest_Error{Error: &runcoilv1.StepError{
			Kind:    errorKindToProto(r.Failure.Kind),
			Message: r.Failure.Message,
		}}

		return req
	}

	req.Outcome = &runcoilv1.CompleteStepRequest_Output{Output: r.Output}

	return req
}

// stepRecordsFromProto reads the journal an Assign carries, by Step ID.
func stepRecordsFromProto(steps []*runcoilv1.StepRecord) map[string]runstate.Record {
	recorded := make(map[string]runstate.Record, len(steps))
	for _, rec := range steps {
		recorded[rec.GetStepId()] = stepRecordFromProto(rec)
	}

	return recorded
}

func stepRecordFromProto(rec *runcoilv1.StepRecord) runstate.Record {
	failure := rec.GetFailure()
	if failure == nil {
		return runstate.Record{Succeeded: true, Output: rec.GetOutput()}
	}

	return runstate.Record{Kind: failureKindFromProto(failure.GetCode()), Message: failure.GetMessage()}
}

func errorKindToProto(kind string) runcoilv1.ErrorKind {
	switch kind {
	case runstate.KindPanic:
		return runcoilv1.ErrorKind_ERROR_KIND_PANIC
	case runstate.KindOutputNotEncodable:
		return runcoilv1.ErrorKind_ERROR_KIND_OUTPUT_NOT_ENCODABLE
	default:
		return runcoilv1.ErrorKind_ERROR_KIND_ERROR
	}
}

// failureKindFromProto maps a stored failure code to the kind a Step error carries. A
// code this SDK does not know is an error.
func failureKindFromProto(code runcoilv1.Failure_Code) string {
	switch code {
	case runcoilv1.Failure_CODE_PANIC:
		return runstate.KindPanic
	case runcoilv1.Failure_CODE_OUTPUT_NOT_ENCODABLE:
		return runstate.KindOutputNotEncodable
	default:
		return runstate.KindError
	}
}
