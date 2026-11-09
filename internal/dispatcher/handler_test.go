package dispatcher

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/runcoil/runcoil/internal/domain"
	runcoilv1 "github.com/runcoil/runcoil/proto/gen/go/runcoil/v1"
)

func TestRegistrationFromProto(t *testing.T) {
	reg := registrationFromProto(&runcoilv1.RegisterRequest{
		App:   "shop",
		Slots: 4,
		Tasks: []*runcoilv1.TaskDefinition{
			{Name: "charge"},
			{
				Name:          "ship",
				Retry:         &runcoilv1.RetryPolicy{MaxAttempts: 3, Delays: []*durationpb.Duration{durationpb.New(5 * time.Second)}},
				StartDeadline: durationpb.New(time.Minute),
			},
		},
	})

	deadline := time.Minute
	assert.Equal(t, domain.Registration{
		App:   "shop",
		Slots: 4,
		Tasks: []domain.TaskDefinition{
			{Name: "charge"},
			{
				Name:          "ship",
				Retry:         &domain.RetryPolicy{MaxAttempts: 3, Delays: []time.Duration{5 * time.Second}},
				StartDeadline: &deadline,
			},
		},
	}, reg)
}

func TestFailureCodeFromProto(t *testing.T) {
	assert.Equal(t, domain.FailureCodeError, failureCodeFromProto(runcoilv1.ErrorKind_ERROR_KIND_UNSPECIFIED))
	assert.Equal(t, domain.FailureCodeError, failureCodeFromProto(runcoilv1.ErrorKind_ERROR_KIND_ERROR))
	assert.Equal(t, domain.FailureCodePanic, failureCodeFromProto(runcoilv1.ErrorKind_ERROR_KIND_PANIC))
	assert.Equal(t, domain.FailureCodeOutputNotEncodable, failureCodeFromProto(runcoilv1.ErrorKind_ERROR_KIND_OUTPUT_NOT_ENCODABLE))
}

func TestStepRecordToProto(t *testing.T) {
	ok := stepRecordToProto(domain.StepRecord{StepID: "charge", Kind: domain.StepKindDo, Output: []byte(`1`)})
	assert.Equal(t, []byte(`1`), ok.GetOutput())

	failed := stepRecordToProto(domain.StepRecord{
		StepID:  "charge",
		Kind:    domain.StepKindDo,
		Failure: &domain.Failure{Code: domain.FailureCodePanic, Message: "boom"},
	})
	assert.Equal(t, runcoilv1.Failure_CODE_PANIC, failed.GetFailure().GetCode())
	assert.Equal(t, "boom", failed.GetFailure().GetMessage())
	assert.Equal(t, "charge", failed.GetFailure().GetStepId())
}

func TestStreamMessageToProto(t *testing.T) {
	lease := domain.Lease{RunID: uuid.New(), Epoch: 7}

	assign := streamMessageToProto(domain.StreamMessage{Assign: &domain.Assign{Lease: lease, TaskName: "charge"}})
	assert.Equal(t, lease.RunID.String(), assign.GetAssign().GetLease().GetRunId())
	assert.Equal(t, int64(7), assign.GetAssign().GetLease().GetEpoch())
	assert.Equal(t, "charge", assign.GetAssign().GetTaskName())

	lost := streamMessageToProto(domain.StreamMessage{LeaseLost: &lease})
	assert.Equal(t, runcoilv1.Cancel_REASON_LEASE_LOST, lost.GetCancel().GetReason())

	assert.NotNil(t, streamMessageToProto(domain.StreamMessage{Keepalive: true}).GetKeepalive())
}
