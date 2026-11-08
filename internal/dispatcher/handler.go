package dispatcher

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/runcoil/runcoil/internal/apierr"
	"github.com/runcoil/runcoil/internal/domain"
	runcoilv1 "github.com/runcoil/runcoil/proto/gen/go/runcoil/v1"
	"github.com/runcoil/runcoil/proto/gen/go/runcoil/v1/runcoilv1connect"
)

var _ runcoilv1connect.WorkerServiceHandler = workerHandler{}

// workerHandler serves the WorkerService. It maps messages to the Service's calls and
// back, and holds no logic of its own. Register takes the API key as its bearer token; every
// other call takes the session token.
type workerHandler struct {
	runcoilv1connect.UnimplementedWorkerServiceHandler

	s *Service
}

func (h workerHandler) Register(ctx context.Context, req *runcoilv1.RegisterRequest) (*runcoilv1.RegisterResponse, error) {
	apiKey, err := bearerToken(ctx)
	if err != nil {
		return nil, err
	}

	result, err := h.s.Register(ctx, apiKey, registrationFromProto(req))
	if err != nil {
		return nil, err
	}

	return &runcoilv1.RegisterResponse{
		WorkerId:          result.Worker.ID.String(),
		SessionToken:      result.SessionToken,
		HeartbeatInterval: durationpb.New(h.s.HeartbeatInterval()),
	}, nil
}

func (h workerHandler) Listen(
	ctx context.Context,
	req *runcoilv1.ListenRequest,
	out runcoilv1connect.WorkerServiceListenServerStream,
) error {
	worker, err := h.authenticate(ctx)
	if err != nil {
		return err
	}

	running := make([]domain.Lease, 0, len(req.GetRunning()))
	for _, l := range req.GetRunning() {
		lease, err := leaseFromProto(l)
		if err != nil {
			return err
		}

		running = append(running, lease)
	}

	return h.s.Listen(ctx, worker, running, func(msg domain.StreamMessage) error {
		return out.Send(listenResponse(msg))
	})
}

func (h workerHandler) Heartbeat(ctx context.Context, _ *runcoilv1.HeartbeatRequest) (*runcoilv1.HeartbeatResponse, error) {
	worker, err := h.authenticate(ctx)
	if err != nil {
		return nil, err
	}

	h.s.Heartbeat(worker)

	return &runcoilv1.HeartbeatResponse{}, nil
}

func (h workerHandler) CompleteStep(
	ctx context.Context,
	req *runcoilv1.CompleteStepRequest,
) (*runcoilv1.CompleteStepResponse, error) {
	worker, err := h.authenticate(ctx)
	if err != nil {
		return nil, err
	}

	lease, err := leaseFromProto(req.GetLease())
	if err != nil {
		return nil, err
	}

	outcome := domain.StepOutcome{Output: req.GetOutput()}
	if stepErr := req.GetError(); stepErr != nil {
		outcome = domain.StepOutcome{Failure: &domain.Failure{
			Code:    failureCodeOf(stepErr.GetKind()),
			Message: stepErr.GetMessage(),
		}}
	}

	record, err := h.s.CompleteStep(ctx, domain.CompleteStepParams{
		WorkerID: worker.ID,
		Lease:    lease,
		StepID:   req.GetStepId(),
		Outcome:  outcome,
	})
	if err != nil {
		return nil, err
	}

	return &runcoilv1.CompleteStepResponse{
		Result: &runcoilv1.CompleteStepResponse_Record{Record: stepRecordToProto(record)},
	}, nil
}

func (h workerHandler) CompleteRun(
	ctx context.Context,
	req *runcoilv1.CompleteRunRequest,
) (*runcoilv1.CompleteRunResponse, error) {
	worker, err := h.authenticate(ctx)
	if err != nil {
		return nil, err
	}

	lease, err := leaseFromProto(req.GetLease())
	if err != nil {
		return nil, err
	}

	result := domain.RunResult{Output: req.GetOutput()}
	if runErr := req.GetError(); runErr != nil {
		result = domain.RunResult{Failure: &domain.Failure{
			Code:    failureCodeOf(runErr.GetKind()),
			Message: runErr.GetMessage(),
			StepID:  runErr.GetStepId(),
		}}
	}

	if err := h.s.CompleteRun(ctx, worker, lease, result); err != nil {
		return nil, err
	}

	return &runcoilv1.CompleteRunResponse{}, nil
}

func (h workerHandler) authenticate(ctx context.Context) (domain.Worker, error) {
	token, err := bearerToken(ctx)
	if err != nil {
		return domain.Worker{}, err
	}

	return h.s.Authenticate(ctx, token)
}

func registrationFromProto(req *runcoilv1.RegisterRequest) domain.Registration {
	reg := domain.Registration{
		App:   req.GetApp(),
		Slots: int(req.GetSlots()),
		Tasks: make([]domain.TaskDefinition, 0, len(req.GetTasks())),
	}

	for _, t := range req.GetTasks() {
		def := domain.TaskDefinition{Name: t.GetName()}
		if t.GetStartDeadline() != nil {
			d := t.GetStartDeadline().AsDuration()
			def.StartDeadline = &d
		}

		if p := t.GetRetry(); p != nil {
			policy := &domain.RetryPolicy{MaxAttempts: int(p.GetMaxAttempts())}
			for _, d := range p.GetDelays() {
				policy.Delays = append(policy.Delays, d.AsDuration())
			}

			def.Retry = policy
		}

		reg.Tasks = append(reg.Tasks, def)
	}

	return reg
}

// leaseFromProto reads a lease. The request rules have checked that run_id is a UUID.
func leaseFromProto(l *runcoilv1.Lease) (domain.Lease, error) {
	runID, err := uuid.Parse(l.GetRunId())
	if err != nil {
		return domain.Lease{}, apierr.Invalid(apierr.CodeInvalidRequest, "lease run_id must be a UUID")
	}

	return domain.Lease{RunID: runID, Epoch: l.GetEpoch()}, nil
}

func leaseToProto(l domain.Lease) *runcoilv1.Lease {
	return &runcoilv1.Lease{RunId: l.RunID.String(), Epoch: l.Epoch}
}

func listenResponse(msg domain.StreamMessage) *runcoilv1.ListenResponse {
	switch {
	case msg.Assign != nil:
		a := msg.Assign
		steps := make([]*runcoilv1.StepRecord, 0, len(a.Steps))
		for _, st := range a.Steps {
			steps = append(steps, stepRecordToProto(st))
		}

		return &runcoilv1.ListenResponse{Kind: &runcoilv1.ListenResponse_Assign{Assign: &runcoilv1.Assign{
			Lease:     leaseToProto(a.Lease),
			TaskName:  a.TaskName,
			Input:     a.Input,
			Steps:     steps,
			CreatedAt: timestamppb.New(a.CreatedAt),
		}}}
	case msg.LeaseLost != nil:
		return &runcoilv1.ListenResponse{Kind: &runcoilv1.ListenResponse_Cancel{Cancel: &runcoilv1.Cancel{
			Lease:  leaseToProto(*msg.LeaseLost),
			Reason: runcoilv1.Cancel_REASON_LEASE_LOST,
		}}}
	default:
		return &runcoilv1.ListenResponse{Kind: &runcoilv1.ListenResponse_Keepalive{Keepalive: &runcoilv1.Keepalive{}}}
	}
}

var failureCodes = map[domain.FailureCode]runcoilv1.Failure_Code{
	domain.FailureCodeError:              runcoilv1.Failure_CODE_ERROR,
	domain.FailureCodePanic:              runcoilv1.Failure_CODE_PANIC,
	domain.FailureCodeStepFailed:         runcoilv1.Failure_CODE_STEP_FAILED,
	domain.FailureCodeAttemptLost:        runcoilv1.Failure_CODE_ATTEMPT_LOST,
	domain.FailureCodeStartDeadline:      runcoilv1.Failure_CODE_START_DEADLINE,
	domain.FailureCodeOutputNotEncodable: runcoilv1.Failure_CODE_OUTPUT_NOT_ENCODABLE,
}

// failureCodeOf maps what a worker reports to a failure code. An unset kind is an error.
func failureCodeOf(kind runcoilv1.ErrorKind) domain.FailureCode {
	switch kind {
	case runcoilv1.ErrorKind_ERROR_KIND_PANIC:
		return domain.FailureCodePanic
	case runcoilv1.ErrorKind_ERROR_KIND_OUTPUT_NOT_ENCODABLE:
		return domain.FailureCodeOutputNotEncodable
	default:
		return domain.FailureCodeError
	}
}

func stepRecordToProto(r domain.StepRecord) *runcoilv1.StepRecord {
	record := &runcoilv1.StepRecord{StepId: r.StepID, Kind: runcoilv1.StepRecord_KIND_DO}
	if r.Failure != nil {
		record.Result = &runcoilv1.StepRecord_Failure{Failure: &runcoilv1.Failure{
			Code:    failureCodes[r.Failure.Code],
			Message: r.Failure.Message,
			StepId:  r.StepID,
		}}

		return record
	}

	record.Result = &runcoilv1.StepRecord_Output{Output: r.Output}

	return record
}
