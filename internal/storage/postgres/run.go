package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/runcoil/runcoil/internal/domain"
	"github.com/runcoil/runcoil/internal/storage/postgres/sqlc"
)

// RunStore keeps the Runs, their leases and their journal.
type RunStore struct {
	db DB
	q  *sqlc.Queries
}

// NewRunStore creates a RunStore on db.
func NewRunStore(db DB) *RunStore {
	return &RunStore{db: db, q: sqlc.New()}
}

// CompleteStep stores the outcome of a Do Step in one transaction and returns the stored
// record. A record of the same kind already stored is returned as it is: a resend
// changes nothing. It returns domain.ErrStale when the lease is not this attempt's.
func (s *RunStore) CompleteStep(ctx context.Context, params domain.CompleteStepParams) (domain.StepRecord, error) {
	lease := params.Lease
	var record domain.StepRecord
	err := s.db.WithTx(ctx, func(tx DB) error {
		held, err := s.q.LeaseLock(ctx, tx, sqlc.LeaseLockParams{
			RunID:      lease.RunID,
			LeaseEpoch: lease.Epoch,
			WorkerID:   params.WorkerID,
		})
		if isNoRowsError(err) {
			return domain.ErrStale
		}

		if err != nil {
			return fmt.Errorf("lock lease: %w", err)
		}

		stored, found, err := s.step(ctx, tx, lease.RunID, params.StepID)
		if err != nil {
			return err
		}

		if found && stored.Kind == domain.StepKindDo {
			record = stored
			return nil
		}

		tries := 1
		if held.RetryStepID != nil && *held.RetryStepID == params.StepID {
			tries = int(held.RetryCount) + 1
		}

		step := sqlc.StepCreateParams{
			RunID:    lease.RunID,
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

		if _, err := s.q.StepCreate(ctx, tx, step); err != nil {
			return fmt.Errorf("create step: %w", err)
		}

		err = s.q.LeaseResetStepRetry(ctx, tx, sqlc.LeaseResetStepRetryParams{RunID: lease.RunID, StepID: params.StepID})
		if err != nil {
			return fmt.Errorf("reset step retry: %w", err)
		}

		record = stepRecord(stepRecordParams{
			StepID:       step.StepID,
			Kind:         string(domain.StepKindDo),
			Status:       step.Status,
			Output:       step.Output,
			ErrorCode:    step.ErrorCode,
			ErrorMessage: step.ErrorMessage,
		})

		return nil
	})

	return record, err
}

func (s *RunStore) step(ctx context.Context, tx DB, runID uuid.UUID, stepID string) (domain.StepRecord, bool, error) {
	row, err := s.q.StepGet(ctx, tx, sqlc.StepGetParams{RunID: runID, StepID: stepID})
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
// resend is safe. It returns domain.ErrStale when the lease is not this attempt's and
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

	_, err := s.q.RunFinish(ctx, s.db, params)
	if err == nil {
		return true, nil
	}

	if !isNoRowsError(err) {
		return false, fmt.Errorf("finish run: %w", err)
	}

	status, err := s.q.RunGetStatus(ctx, s.db, lease.RunID)
	if isNoRowsError(err) {
		return false, domain.ErrStale
	}

	if err != nil {
		return false, fmt.Errorf("get run status: %w", err)
	}

	if !domain.RunStatus(status).Ended() {
		return false, domain.ErrStale
	}

	return false, nil
}
