package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/runcoil/runcoil/internal/domain"
	"github.com/runcoil/runcoil/internal/storage/postgres/sqlc"
)

// WorkerStore keeps the worker sessions, the Tasks they declare, the dispatchers, and
// the claim of queued Runs.
type WorkerStore struct {
	db      DB
	queries sqlc.Queries
}

// NewWorkerStore creates a WorkerStore on db. Every store shares the one queries value.
func NewWorkerStore(db DB, queries sqlc.Queries) *WorkerStore {
	return &WorkerStore{db: db, queries: queries}
}

// Register stores the Tasks of the registration and the worker session, in one
// transaction. It returns domain.ErrAPIKeyNotFound when no API key has keyHash.
func (s *WorkerStore) Register(ctx context.Context, keyHash []byte, session domain.WorkerSession) (domain.Worker, error) {
	var worker domain.Worker
	err := s.db.WithTx(ctx, func(tx DB) error {
		key, err := s.queries.ApiKeyGetForRegister(ctx, tx, keyHash)
		if isNoRowsError(err) {
			return domain.ErrAPIKeyNotFound
		}

		if err != nil {
			return fmt.Errorf("get api key: %w", err)
		}

		reg := session.Registration
		taskIDs := make([]uuid.UUID, 0, len(reg.Tasks))
		for i, def := range reg.Tasks {
			params, err := taskUpsertParams(session.TaskIDs[i], key.ProjectID, reg.App, def)
			if err != nil {
				return fmt.Errorf("task %q: %w", def.Name, err)
			}

			id, err := s.queries.TaskUpsert(ctx, tx, params)
			if err != nil {
				return fmt.Errorf("upsert task %q: %w", def.Name, err)
			}

			taskIDs = append(taskIDs, id)
		}

		err = s.queries.WorkerCreate(ctx, tx, sqlc.WorkerCreateParams{
			ID:        session.ID,
			ProjectID: key.ProjectID,
			ApiKeyID:  key.ID,
			TokenHash: session.TokenHash,
			App:       reg.App,
			TaskIds:   taskIDs,
			Slots:     int32(reg.Slots), //nolint:gosec // The engine bounds the slots.
		})
		if err != nil {
			return fmt.Errorf("create worker: %w", err)
		}

		worker = domain.Worker{
			ID:        session.ID,
			ProjectID: key.ProjectID,
			App:       reg.App,
			TaskIDs:   taskIDs,
			Slots:     reg.Slots,
		}

		return nil
	})
	if err != nil {
		return domain.Worker{}, fmt.Errorf("register in transaction: %w", err)
	}

	return worker, nil
}

// retryPolicyJSON is how tasks.retry_policy stores a policy.
type retryPolicyJSON struct {
	MaxAttempts int      `json:"max_attempts,omitempty"`
	Delays      []string `json:"delays,omitempty"`
}

func taskUpsertParams(id, projectID uuid.UUID, app string, def domain.TaskDefinition) (sqlc.TaskUpsertParams, error) {
	params := sqlc.TaskUpsertParams{
		ID:            id,
		ProjectID:     projectID,
		App:           app,
		Name:          def.Name,
		StartDeadline: interval(def.StartDeadline),
	}

	if def.Retry != nil {
		policy := retryPolicyJSON{MaxAttempts: def.Retry.MaxAttempts}
		for _, d := range def.Retry.Delays {
			policy.Delays = append(policy.Delays, d.String())
		}

		raw, err := json.Marshal(policy)
		if err != nil {
			return sqlc.TaskUpsertParams{}, fmt.Errorf("encode retry policy: %w", err)
		}

		msg := json.RawMessage(raw)
		params.RetryPolicy = &msg
	}

	return params, nil
}

// WorkerBySessionToken returns the live session whose token has tokenHash, or
// domain.ErrSessionEnded.
func (s *WorkerStore) WorkerBySessionToken(ctx context.Context, tokenHash []byte) (domain.Worker, error) {
	row, err := s.queries.WorkerGetBySessionToken(ctx, s.db, tokenHash)
	if isNoRowsError(err) {
		return domain.Worker{}, domain.ErrSessionEnded
	}

	if err != nil {
		return domain.Worker{}, fmt.Errorf("get worker: %w", err)
	}

	return domain.Worker{
		ID:        row.ID,
		ProjectID: row.ProjectID,
		App:       row.App,
		TaskIDs:   row.TaskIds,
		Slots:     int(row.Slots),
	}, nil
}

// AttachStream moves the worker to a new stream and reconciles its leases, in one
// transaction: the leases it does not report go back to the queue, and the reported
// Runs it no longer holds are returned. It returns domain.ErrSessionEnded when the
// session has ended.
func (s *WorkerStore) AttachStream(ctx context.Context, params domain.AttachStreamParams) ([]domain.Lease, error) {
	runIDs := make([]uuid.UUID, 0, len(params.Running))
	epochs := make([]int64, 0, len(params.Running))
	for _, l := range params.Running {
		runIDs = append(runIDs, l.RunID)
		epochs = append(epochs, l.Epoch)
	}

	var notHeld []domain.Lease
	err := s.db.WithTx(ctx, func(tx DB) error {
		_, err := s.queries.WorkerAttachStream(ctx, tx, sqlc.WorkerAttachStreamParams{
			ID:           params.WorkerID,
			DispatcherID: params.DispatcherID,
			StreamID:     params.StreamID,
		})
		if isNoRowsError(err) {
			return domain.ErrSessionEnded
		}

		if err != nil {
			return fmt.Errorf("attach stream: %w", err)
		}

		_, err = s.queries.LeaseReleaseUnknown(ctx, tx, sqlc.LeaseReleaseUnknownParams{
			WorkerID:   params.WorkerID,
			HeldEpochs: epochs,
		})
		if err != nil {
			return fmt.Errorf("release unknown leases: %w", err)
		}

		rows, err := s.queries.LeaseListNotHeld(ctx, tx, sqlc.LeaseListNotHeldParams{
			WorkerID:    params.WorkerID,
			RunIds:      runIDs,
			LeaseEpochs: epochs,
		})
		if err != nil {
			return fmt.Errorf("list leases not held: %w", err)
		}

		notHeld = make([]domain.Lease, 0, len(rows))
		for _, row := range rows {
			notHeld = append(notHeld, domain.Lease{RunID: row.RunID, Epoch: row.LeaseEpoch})
		}

		params.Attached(notHeld)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("attach stream in transaction: %w", err)
	}

	return notHeld, nil
}

// FlushHeartbeats writes the last heartbeat of each worker. A time older than the stored
// one is ignored.
func (s *WorkerStore) FlushHeartbeats(ctx context.Context, ids []uuid.UUID, beats []time.Time) error {
	if err := s.queries.WorkerFlushHeartbeats(ctx, s.db, sqlc.WorkerFlushHeartbeatsParams{Ids: ids, Beats: beats}); err != nil {
		return fmt.Errorf("flush heartbeats: %w", err)
	}

	return nil
}

// BeatDispatcher records that the dispatcher is alive.
func (s *WorkerStore) BeatDispatcher(ctx context.Context, id uuid.UUID) error {
	if err := s.queries.DispatcherBeat(ctx, s.db, id); err != nil {
		return fmt.Errorf("beat dispatcher: %w", err)
	}

	return nil
}

// ClaimRuns leases queued Runs to the worker in one transaction, then reads what their
// attempts need. It returns domain.ErrStreamFenced when the worker is no longer on the
// stream.
func (s *WorkerStore) ClaimRuns(ctx context.Context, params domain.ClaimRunsParams) ([]domain.Assign, error) {
	var leases []sqlc.LeaseClaimRow
	err := s.db.WithTx(ctx, func(tx DB) error {
		_, err := s.queries.WorkerLockForClaim(ctx, tx, sqlc.WorkerLockForClaimParams{
			ID:           params.WorkerID,
			DispatcherID: params.DispatcherID,
			StreamID:     params.StreamID,
		})
		if isNoRowsError(err) {
			return domain.ErrStreamFenced
		}

		if err != nil {
			return fmt.Errorf("lock worker: %w", err)
		}

		leases, err = s.queries.LeaseClaim(ctx, tx, sqlc.LeaseClaimParams{
			WorkerID: params.WorkerID,
			TaskIds:  params.TaskIDs,
			Slots:    int32(params.Slots), //nolint:gosec // Slots come from the worker row, an integer.
		})
		if err != nil {
			return fmt.Errorf("claim runs: %w", err)
		}

		if len(leases) == 0 {
			return nil
		}

		if err := s.queries.RunMarkStarted(ctx, tx, leaseRunIDs(leases)); err != nil {
			return fmt.Errorf("mark runs started: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("claim in transaction: %w", err)
	}

	if len(leases) == 0 {
		return nil, nil
	}

	assigns, err := s.assigns(ctx, leases)
	if err != nil {
		return nil, fmt.Errorf("read assigns: %w", err)
	}

	return assigns, nil
}

func leaseRunIDs(leases []sqlc.LeaseClaimRow) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(leases))
	for _, l := range leases {
		ids = append(ids, l.RunID)
	}

	return ids
}

// assigns reads the input and the journal of the claimed Runs, outside the claim.
func (s *WorkerStore) assigns(ctx context.Context, leases []sqlc.LeaseClaimRow) ([]domain.Assign, error) {
	ids := leaseRunIDs(leases)

	runs, err := s.queries.RunListForAssign(ctx, s.db, ids)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}

	steps, err := s.queries.StepListForRuns(ctx, s.db, ids)
	if err != nil {
		return nil, fmt.Errorf("list steps: %w", err)
	}

	journal := make(map[uuid.UUID][]domain.StepRecord, len(runs))
	for _, st := range steps {
		journal[st.RunID] = append(journal[st.RunID], stepRecord(stepRecordParams{
			StepID:       st.StepID,
			Kind:         st.Kind,
			Status:       st.Status,
			Output:       st.Output,
			ErrorCode:    st.ErrorCode,
			ErrorMessage: st.ErrorMessage,
		}))
	}

	epochs := make(map[uuid.UUID]int64, len(leases))
	for _, l := range leases {
		epochs[l.RunID] = l.LeaseEpoch
	}

	assigns := make([]domain.Assign, 0, len(runs))
	for _, r := range runs {
		assigns = append(assigns, domain.Assign{
			Lease:     domain.Lease{RunID: r.ID, Epoch: epochs[r.ID]},
			TaskName:  r.TaskName,
			Input:     r.Input,
			Steps:     journal[r.ID],
			CreatedAt: r.CreatedAt,
		})
	}

	return assigns, nil
}

// stepRecordParams is a row of steps, whichever query read it.
type stepRecordParams struct {
	StepID       string
	Kind         string
	Status       string
	Output       []byte
	ErrorCode    *string
	ErrorMessage *string
}

// stepRecord builds a Step record from its row.
func stepRecord(params stepRecordParams) domain.StepRecord {
	record := domain.StepRecord{StepID: params.StepID, Kind: domain.StepKind(params.Kind), Output: params.Output}
	if domain.StepStatus(params.Status) != domain.StepStatusFailed {
		return record
	}

	record.Output = nil
	record.Failure = &domain.Failure{}
	if params.ErrorCode != nil {
		record.Failure.Code = domain.FailureCode(*params.ErrorCode)
	}

	if params.ErrorMessage != nil {
		record.Failure.Message = *params.ErrorMessage
	}

	return record
}
