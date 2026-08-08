package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/asa/simple-disbursement-service-example/internal/model"
	"github.com/asa/simple-disbursement-service-example/internal/repository"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// fakeDisbursementRepo implements repository.DisbursementRepository in memory.
// UpdateStatus applies the same guarded semantics as the real repository:
// the row transitions only if its current status still equals `from`; the
// affected-row count is 0 when a concurrent transition won first.
type fakeDisbursementRepo struct {
	mu     sync.Mutex
	rows   map[int64]*model.Disbursement
	nextID int64
	// updateStatusOverride, when set, replaces the default guarded transition.
	updateStatusOverride func(d *model.Disbursement, from model.DisbursementStatus) (int64, error)
}

func newFakeDisbursementRepo() *fakeDisbursementRepo {
	return &fakeDisbursementRepo{rows: map[int64]*model.Disbursement{}}
}

func (f *fakeDisbursementRepo) seed(d *model.Disbursement) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	d.ID = f.nextID
	cp := *d
	f.rows[d.ID] = &cp
}

func (f *fakeDisbursementRepo) Create(_ context.Context, d *model.Disbursement) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	d.ID = f.nextID
	cp := *d
	f.rows[d.ID] = &cp
	return nil
}

func (f *fakeDisbursementRepo) FindByID(_ context.Context, id int64) (*model.Disbursement, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.rows[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	cp := *row // copy so callers mutating the result cannot corrupt the store
	return &cp, nil
}

func (f *fakeDisbursementRepo) UpdateStatus(_ context.Context, d *model.Disbursement, from model.DisbursementStatus) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.updateStatusOverride != nil {
		return f.updateStatusOverride(d, from)
	}
	row, ok := f.rows[d.ID]
	if !ok || row.Status != from {
		return 0, nil
	}
	row.Status = d.Status
	row.ApprovedBy = d.ApprovedBy
	return 1, nil
}

func (f *fakeDisbursementRepo) SoftDelete(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.rows[id]
	if !ok || row.Status != model.StatusPending {
		return repository.ErrNotFound
	}
	now := time.Now()
	row.DeletedAt = &now
	return nil
}

func (f *fakeDisbursementRepo) List(context.Context, repository.ListFilter) ([]model.Disbursement, int64, error) {
	return nil, 0, nil
}

func (f *fakeDisbursementRepo) CreateIdempotent(_ context.Context, key string, d *model.Disbursement) (*model.Disbursement, bool, error) {
	return nil, false, errors.New("not implemented in fake")
}

// fakeAuditRepo implements repository.AuditLogRepository, capturing entries.
type fakeAuditRepo struct {
	mu      sync.Mutex
	entries []*model.AuditLog
}

func (f *fakeAuditRepo) Create(_ context.Context, entry *model.AuditLog) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *entry
	f.entries = append(f.entries, &cp)
	return nil
}

func (f *fakeAuditRepo) List(context.Context, repository.AuditFilter) ([]model.AuditLog, int64, error) {
	return nil, 0, nil
}

// newDisbService wires the service under test with fresh fakes.
func newDisbService(t *testing.T) (*DisbursementService, *fakeDisbursementRepo, *fakeAuditRepo) {
	t.Helper()
	repo := newFakeDisbursementRepo()
	audit := &fakeAuditRepo{}
	return NewDisbursementService(repo, NewAuditService(audit)), repo, audit
}

func pendingRow() *model.Disbursement {
	return &model.Disbursement{
		RecipientName: "Budi",
		AccountNumber: "123",
		BankCode:      "BCA",
		Amount:        1_250_000,
		AdminFee:      2500,
		Status:        model.StatusPending,
	}
}

func adminActor(id int64) *model.User {
	return &model.User{ID: id, Username: "admin", Role: model.RoleAdmin}
}

// ---------------------------------------------------------------------------
// AdminFee
// ---------------------------------------------------------------------------

func TestAdminFee(t *testing.T) {
	tests := []struct {
		name   string
		amount int64
		want   int64
	}{
		{"zero", 0, 2500},
		{"negative", -100, 2500},
		{"just above zero", 1, 2500},
		{"minimum valid amount", 10_000, 2500},
		{"typical small payout", 1_250_000, 2500},
		{"one below threshold", 4_999_999, 2500},
		{"exactly at threshold", 5_000_000, 5000},
		{"one above threshold", 5_000_001, 5000},
		{"large payout", 1_000_000_000, 5000},
		{"max int64", int64(^uint64(0) >> 1), 5000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AdminFee(tt.amount); got != tt.want {
				t.Errorf("AdminFee(%d) = %d, want %d", tt.amount, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// UpdateStatus
// ---------------------------------------------------------------------------

func TestUpdateStatus(t *testing.T) {
	adminID := int64(7)
	actor := adminActor(adminID)

	tests := []struct {
		name              string
		setup             func(*fakeDisbursementRepo)
		target            model.DisbursementStatus
		actor             *model.User
		wantErr           error  // sentinel error expected, nil for success
		wantErrMsg        string // substring expected when wantErr is nil
		wantStatus        model.DisbursementStatus
		wantApprovedByID  int64 // approved_by expected value (wantApprovedBySet must be true)
		wantApprovedBySet bool  // approved_by expected non-nil
	}{
		{
			name:   "approve pending as admin sets approved_by",
			setup:  func(r *fakeDisbursementRepo) { r.seed(pendingRow()) },
			target: model.StatusApproved, actor: actor,
			wantStatus: model.StatusApproved, wantApprovedBySet: true, wantApprovedByID: adminID,
		},
		{
			name:   "reject pending as admin clears approved_by",
			setup:  func(r *fakeDisbursementRepo) { r.seed(pendingRow()) },
			target: model.StatusRejected, actor: actor,
			wantStatus: model.StatusRejected, wantApprovedBySet: false,
		},
		{
			name:       "approve pending as superadmin",
			setup:      func(r *fakeDisbursementRepo) { r.seed(pendingRow()) },
			target:     model.StatusApproved,
			actor:      &model.User{ID: 1, Username: "root", Role: model.RoleSuperadmin},
			wantStatus: model.StatusApproved, wantApprovedBySet: true, wantApprovedByID: 1,
		},
		{
			name:   "reject invalid target",
			setup:  func(r *fakeDisbursementRepo) { r.seed(pendingRow()) },
			target: model.DisbursementStatus("CANCELLED"), actor: actor,
			wantErrMsg: "APPROVED or REJECTED",
		},
		{
			name:   "target PENDING itself is invalid",
			setup:  func(r *fakeDisbursementRepo) { r.seed(pendingRow()) },
			target: model.StatusPending, actor: actor,
			wantErrMsg: "APPROVED or REJECTED",
		},
		{
			name:   "empty target is invalid",
			setup:  func(r *fakeDisbursementRepo) { r.seed(pendingRow()) },
			target: "", actor: actor,
			wantErrMsg: "APPROVED or REJECTED",
		},
		{
			name:    "operator is forbidden",
			setup:   func(r *fakeDisbursementRepo) { r.seed(pendingRow()) },
			target:  model.StatusApproved,
			actor:   &model.User{ID: 2, Username: "op", Role: model.RoleOperator},
			wantErr: ErrForbidden,
		},
		{
			name:    "unknown role is forbidden",
			setup:   func(r *fakeDisbursementRepo) { r.seed(pendingRow()) },
			target:  model.StatusApproved,
			actor:   &model.User{ID: 3, Username: "ghost", Role: model.Role("guest")},
			wantErr: ErrForbidden,
		},
		{
			name:    "empty role is forbidden",
			setup:   func(r *fakeDisbursementRepo) { r.seed(pendingRow()) },
			target:  model.StatusApproved,
			actor:   &model.User{ID: 4, Username: "anon"},
			wantErr: ErrForbidden,
		},
		{
			name:   "missing disbursement is not found",
			setup:  func(*fakeDisbursementRepo) {},
			target: model.StatusApproved, actor: actor,
			wantErr: ErrNotFound,
		},
		{
			name: "already approved is terminal",
			setup: func(r *fakeDisbursementRepo) {
				d := pendingRow()
				d.Status = model.StatusApproved
				r.seed(d)
			},
			target: model.StatusRejected, actor: actor,
			wantErr: ErrAlreadyTransitioned,
		},
		{
			name: "already rejected is terminal",
			setup: func(r *fakeDisbursementRepo) {
				d := pendingRow()
				d.Status = model.StatusRejected
				r.seed(d)
			},
			target: model.StatusApproved, actor: actor,
			wantErr: ErrAlreadyTransitioned,
		},
		{
			name: "repo reports zero rows on lost race",
			setup: func(r *fakeDisbursementRepo) {
				r.seed(pendingRow())
				r.updateStatusOverride = func(d *model.Disbursement, from model.DisbursementStatus) (int64, error) {
					return 0, nil // another writer already transitioned
				}
			},
			target: model.StatusApproved, actor: actor,
			wantErr: ErrAlreadyTransitioned,
		},
		{
			name: "repo error propagates",
			setup: func(r *fakeDisbursementRepo) {
				r.seed(pendingRow())
				r.updateStatusOverride = func(d *model.Disbursement, from model.DisbursementStatus) (int64, error) {
					return 0, errors.New("db down")
				}
			},
			target: model.StatusApproved, actor: actor,
			wantErrMsg: "db down",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo, _ := newDisbService(t)
			tt.setup(repo)
			var id int64 = 1
			got, err := svc.UpdateStatus(context.Background(), tt.actor, id, tt.target, "note")

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("UpdateStatus() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if tt.wantErrMsg != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Fatalf("UpdateStatus() error = %v, want message containing %q", err, tt.wantErrMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("UpdateStatus() unexpected error: %v", err)
			}
			if got.Status != tt.wantStatus {
				t.Errorf("status = %s, want %s", got.Status, tt.wantStatus)
			}
			if tt.wantApprovedBySet {
				if got.ApprovedBy == nil || *got.ApprovedBy != tt.wantApprovedByID {
					t.Errorf("approved_by = %v, want %d", got.ApprovedBy, tt.wantApprovedByID)
				}
			} else if got.ApprovedBy != nil {
				t.Errorf("approved_by = %v, want nil", got.ApprovedBy)
			}
		})
	}
}

func TestUpdateStatusRecordsAudit(t *testing.T) {
	svc, repo, audit := newDisbService(t)
	repo.seed(pendingRow())
	actor := adminActor(7)

	if _, err := svc.UpdateStatus(context.Background(), actor, 1, model.StatusApproved, "verified"); err != nil {
		t.Fatalf("UpdateStatus() error: %v", err)
	}

	audit.mu.Lock()
	defer audit.mu.Unlock()
	if len(audit.entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(audit.entries))
	}
	e := audit.entries[0]
	if e.Action != model.ActionStatusChange {
		t.Errorf("audit action = %s, want %s", e.Action, model.ActionStatusChange)
	}
	if e.Actor != actor.Username {
		t.Errorf("audit actor = %s, want %s", e.Actor, actor.Username)
	}
	if e.Before["status"] != model.StatusPending {
		t.Errorf("audit before = %v, want status PENDING", e.Before)
	}
	if e.After["status"] != model.StatusApproved {
		t.Errorf("audit after = %v, want status APPROVED", e.After)
	}
}

func TestUpdateStatusNoAuditOnFailure(t *testing.T) {
	svc, repo, audit := newDisbService(t)
	repo.seed(pendingRow())

	// operator -> forbidden, no audit write expected
	if _, err := svc.UpdateStatus(context.Background(),
		&model.User{ID: 2, Username: "op", Role: model.RoleOperator}, 1, model.StatusApproved, ""); err == nil {
		t.Fatal("expected forbidden error")
	}
	audit.mu.Lock()
	defer audit.mu.Unlock()
	if len(audit.entries) != 0 {
		t.Errorf("audit entries = %d, want 0 on forbidden", len(audit.entries))
	}
}

// TestUpdateStatusConcurrentApprove mirrors the race in the spec: N admins
// approve the same PENDING disbursement at once; exactly one wins.
func TestUpdateStatusConcurrentApprove(t *testing.T) {
	svc, repo, _ := newDisbService(t)
	repo.seed(pendingRow())

	const workers = 8
	var wg sync.WaitGroup
	var mu sync.Mutex
	successes, conflicts := 0, 0

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			actor := &model.User{ID: id, Username: "admin", Role: model.RoleAdmin}
			_, err := svc.UpdateStatus(context.Background(), actor, 1, model.StatusApproved, "")
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				successes++
			case errors.Is(err, ErrAlreadyTransitioned):
				conflicts++
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}(int64(i + 1))
	}
	wg.Wait()

	if successes != 1 {
		t.Errorf("successful transitions = %d, want exactly 1", successes)
	}
	if conflicts != workers-1 {
		t.Errorf("conflicts = %d, want %d", conflicts, workers-1)
	}
	// Final persisted state must be APPROVED.
	got, err := repo.FindByID(context.Background(), 1)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.Status != model.StatusApproved {
		t.Errorf("final status = %s, want APPROVED", got.Status)
	}
}
