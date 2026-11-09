package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/runcoil/runcoil/internal/domain"
	"github.com/runcoil/runcoil/internal/storage/postgres/sqlc"
)

// RunStore keeps the Runs, their leases and their journal.
type RunStore struct {
	db      DB
	queries sqlc.Queries
}

// NewRunStore creates a RunStore on db. Every store shares the one queries value.
func NewRunStore(db DB, queries sqlc.Queries) *RunStore {
	return &RunStore{db: db, queries: queries}
}

// CompleteStep stores the outcome of a Do Step in one transaction and returns the stored
// record. A record of the same kind already stored is returned as it is: a resend
// changes nothing. A failed try that decide sends to a retry ends the attempt instead:
// the Run waits in run_delayed and no record is stored. It returns domain.ErrStaleLease
// when the lease is not this attempt's.
func (s *RunStore) CompleteStep(
	ctx context.Context,
	params domain.CompleteStepParams,
	decide domain.RetryDecider,
) (domain.StepResult, error) {
	lease := params.Lease
	var result domain.StepResult
	err := s.db.WithTx(ctx, func(tx DB) error {
		held, err := s.queries.LeaseLock(ctx, tx, sqlc.LeaseLockParams{
			RunID:      lease.RunID,
			LeaseEpoch: lease.Epoch,
			WorkerID:   params.WorkerID,
		})
		if isNoRowsError(err) {
			return domain.ErrStaleLease
		}

		if err != nil {
			return fmt.Errorf("lock lease: %w", err)
		}

		stored, found, err := s.step(ctx, tx, lease.RunID, params.StepID)
		if err != nil {
			return fmt.Errorf("read step: %w", err)
		}

		if found && stored.Kind == domain.StepKindDo {
			result.Record = stored
			return nil
		}

		tries := 1
		if held.RetryStepID != nil && *held.RetryStepID == params.StepID {
			tries = int(held.RetryCount) + 1
		}

		if params.Outcome.Failure != nil {
			retry, err := s.scheduleRetry(ctx, tx, params, tries, held.Config, decide)
			if err != nil {
				return fmt.Errorf("schedule retry: %w", err)
			}

			if retry != nil {
				result.Retry = retry
				return nil
			}
		}

		record, err := s.createStep(ctx, tx, params, tries)
		if err != nil {
			return fmt.Errorf("create step: %w", err)
		}

		result.Record = record

		return nil
	})
	if err != nil {
		return domain.StepResult{}, fmt.Errorf("complete step in transaction: %w", err)
	}

	return result, nil
}

// scheduleRetry asks decide whether the failed try is tried again and, when it is, moves
// the Run to run_delayed. It returns nil when the failure is final.
func (s *RunStore) scheduleRetry(
	ctx context.Context,
	tx DB,
	params domain.CompleteStepParams,
	tries int,
	config json.RawMessage,
	decide domain.RetryDecider,
) (*domain.ScheduledRetry, error) {
	runPolicy, err := runRetryPolicy(config)
	if err != nil {
		return nil, fmt.Errorf("read run retry policy: %w", err)
	}

	delay, retry := decide(tries, runPolicy)
	if !retry {
		return nil, nil //nolint:nilnil // No retry is not an error.
	}

	row, err := s.queries.LeaseScheduleStepRetry(ctx, tx, sqlc.LeaseScheduleStepRetryParams{
		Delay:      interval(&delay),
		StepID:     params.StepID,
		RunID:      params.Lease.RunID,
		LeaseEpoch: params.Lease.Epoch,
		WorkerID:   params.WorkerID,
	})
	if err != nil {
		return nil, fmt.Errorf("move run to run_delayed: %w", err)
	}

	return &domain.ScheduledRetry{NextAttemptAt: row.ReadyAt, TryCount: int(row.RetryCount)}, nil
}

// createStep stores the record of a Step that succeeded, or failed for good, at its try
// number tries.
func (s *RunStore) createStep(ctx context.Context, tx DB, params domain.CompleteStepParams, tries int) (domain.StepRecord, error) {
	step := sqlc.StepCreateParams{
		RunID:    params.Lease.RunID,
		StepID:   params.StepID,
		Status:   string(params.Outcome.Status()),
		Output:   params.Outcome.Output,
		TryCount: int32(tries), //nolint:gosec // retry_count is a smallint.
	}
	if failure := params.Outcome.Failure; failure != nil {
		code := string(failure.Code)
		step.Output = nil
		step.ErrorCode = &code
		step.ErrorMessage = &failure.Message
	}

	if _, err := s.queries.StepCreate(ctx, tx, step); err != nil {
		return domain.StepRecord{}, fmt.Errorf("insert step: %w", err)
	}

	err := s.queries.LeaseResetStepRetry(ctx, tx, sqlc.LeaseResetStepRetryParams{RunID: params.Lease.RunID, StepID: params.StepID})
	if err != nil {
		return domain.StepRecord{}, fmt.Errorf("reset step retry: %w", err)
	}

	return stepRecord(stepRecordParams{
		StepID:       step.StepID,
		Kind:         string(domain.StepKindDo),
		Status:       step.Status,
		Output:       step.Output,
		ErrorCode:    step.ErrorCode,
		ErrorMessage: step.ErrorMessage,
	}), nil
}

func (s *RunStore) step(ctx context.Context, tx DB, runID uuid.UUID, stepID string) (domain.StepRecord, bool, error) {
	row, err := s.queries.StepGet(ctx, tx, sqlc.StepGetParams{RunID: runID, StepID: stepID})
	if isNoRowsError(err) {
		return domain.StepRecord{}, false, nil
	}

	if err != nil {
		return domain.StepRecord{}, false, fmt.Errorf("get step: %w", err)
	}

	return stepRecord(stepRecordParams(row)), true, nil
}

// FinishRun ends the Run of the attempt that holds lease and reports whether this call
// ended it. A Run that has already ended is not changed, and the call succeeds, so a
// resend is safe. It returns domain.ErrStaleLease when the lease is not this attempt's and
// the Run has not ended.
func (s *RunStore) FinishRun(ctx context.Context, workerID uuid.UUID, lease domain.Lease, end domain.RunEnd) (bool, error) {
	params := sqlc.RunFinishParams{
		RunID:      lease.RunID,
		LeaseEpoch: lease.Epoch,
		WorkerID:   workerID,
		Status:     string(end.Status),
		Output:     end.Output,
	}
	if end.Failure != nil {
		code := string(end.Failure.Code)
		params.Output = nil
		params.ErrorCode = &code
		params.ErrorMessage = &end.Failure.Message
	}

	_, err := s.queries.RunFinish(ctx, s.db, params)
	if err == nil {
		return true, nil
	}

	if !isNoRowsError(err) {
		return false, fmt.Errorf("finish run: %w", err)
	}

	status, err := s.queries.RunGetStatus(ctx, s.db, lease.RunID)
	if isNoRowsError(err) {
		return false, domain.ErrStaleLease
	}

	if err != nil {
		return false, fmt.Errorf("get run status: %w", err)
	}

	if !domain.RunStatus(status).Ended() {
		return false, domain.ErrStaleLease
	}

	return false, nil
}

// PromoteDueDelayed moves at most batchSize Runs whose wait in run_delayed is over to
// run_queue, and returns how many it moved.
func (s *RunStore) PromoteDueDelayed(ctx context.Context, batchSize int) (int, error) {
	ids, err := s.queries.DelayedPromoteDue(ctx, s.db, int32(batchSize)) //nolint:gosec // A small batch size.
	if err != nil {
		return 0, fmt.Errorf("promote due delayed runs: %w", err)
	}

	return len(ids), nil
}

// ExpireStartDeadlines fails at most batchSize queued Runs past their start deadline, and
// returns how many it failed.
func (s *RunStore) ExpireStartDeadlines(ctx context.Context, batchSize int) (int, error) {
	ids, err := s.queries.QueueExpireStartDeadlines(ctx, s.db, int32(batchSize)) //nolint:gosec // A small batch size.
	if err != nil {
		return 0, fmt.Errorf("expire start deadlines: %w", err)
	}

	return len(ids), nil
}
