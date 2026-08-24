package usecase

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"audiax/internal/entity"
	"audiax/internal/model"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormtests "gorm.io/gorm/utils/tests"
)

// fakeMachineRepo keeps machines in a map, so these tests need no database.
// Shared by machine_test.go and, later, the baseline and inspection tests.
type fakeMachineRepo struct {
	byID    map[string]*entity.Machine
	updated map[string]any
	deleted []string
}

func newFakeMachineRepo() *fakeMachineRepo {
	return &fakeMachineRepo{byID: map[string]*entity.Machine{}}
}

// Create mirrors the partial unique index on (user_id, lower(label)): the
// database is what actually enforces this, and the fake has to agree or the
// conflict path would never be exercised.
func (f *fakeMachineRepo) Create(_ *gorm.DB, machine *entity.Machine) error {
	for _, existing := range f.byID {
		if existing.UserID == machine.UserID &&
			strings.EqualFold(existing.Label, machine.Label) {
			return gorm.ErrDuplicatedKey
		}
	}
	copied := *machine
	f.byID[machine.ID] = &copied
	return nil
}

func (f *fakeMachineRepo) FindByIDForUser(_ *gorm.DB, machine *entity.Machine, id, userID string) error {
	found, ok := f.byID[id]
	if !ok || found.UserID != userID {
		return gorm.ErrRecordNotFound
	}
	*machine = *found
	return nil
}

func (f *fakeMachineRepo) ListForUser(_ *gorm.DB, machines *[]entity.Machine, userID string) error {
	for _, m := range f.byID {
		if m.UserID == userID {
			*machines = append(*machines, *m)
		}
	}
	return nil
}

func (f *fakeMachineRepo) Update(_ *gorm.DB, _ *entity.Machine, fields map[string]any) error {
	f.updated = fields
	return nil
}

func (f *fakeMachineRepo) Delete(_ *gorm.DB, machine *entity.Machine) error {
	f.deleted = append(f.deleted, machine.ID)
	delete(f.byID, machine.ID)
	return nil
}

// --- a *gorm.DB that can open a transaction ------------------------------
//
// DummyDialector leaves ConnPool nil, so db.Transaction returns
// ErrInvalidTransaction and never runs its callback -- a use case that opens a
// transaction would look broken for a reason that has nothing to do with it.
// These two types give gorm just enough of a pool to begin and commit. No SQL
// ever reaches them: every repository in these tests is a fake.

type fakeConnPool struct{}

func (fakeConnPool) PrepareContext(context.Context, string) (*sql.Stmt, error) { return nil, nil }
func (fakeConnPool) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, nil
}
func (fakeConnPool) QueryContext(context.Context, string, ...any) (*sql.Rows, error) {
	return nil, nil
}
func (fakeConnPool) QueryRowContext(context.Context, string, ...any) *sql.Row { return nil }

func (fakeConnPool) BeginTx(context.Context, *sql.TxOptions) (gorm.ConnPool, error) {
	return &fakeTx{}, nil
}

type fakeTx struct{ fakeConnPool }

func (*fakeTx) Commit() error   { return nil }
func (*fakeTx) Rollback() error { return nil }

// newTestDB yields a *gorm.DB that needs no server but still supports
// db.Transaction.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(gormtests.DummyDialector{}, &gorm.Config{ConnPool: fakeConnPool{}})
	require.NoError(t, err)
	return db
}

// --- baseline repository -------------------------------------------------

type fakeBaselineRepo struct {
	rows            []*entity.Baseline
	deactivateCalls int
}

func newFakeBaselineRepo() *fakeBaselineRepo { return &fakeBaselineRepo{} }

func (f *fakeBaselineRepo) Create(_ *gorm.DB, baseline *entity.Baseline) error {
	copied := *baseline
	f.rows = append(f.rows, &copied)
	return nil
}

func (f *fakeBaselineRepo) FindActiveForMachine(_ *gorm.DB, baseline *entity.Baseline, machineID string) error {
	for _, row := range f.rows {
		if row.MachineID == machineID && row.IsActive {
			*baseline = *row
			return nil
		}
	}
	return gorm.ErrRecordNotFound
}

func (f *fakeBaselineRepo) DeactivateAllForMachine(_ *gorm.DB, machineID string) error {
	f.deactivateCalls++
	for _, row := range f.rows {
		if row.MachineID == machineID {
			row.IsActive = false
		}
	}
	return nil
}

func (f *fakeBaselineRepo) ListForMachine(_ *gorm.DB, baselines *[]entity.Baseline, machineID string) error {
	for _, row := range f.rows {
		if row.MachineID == machineID {
			*baselines = append(*baselines, *row)
		}
	}
	return nil
}

func (f *fakeBaselineRepo) UpdateAudioPath(_ *gorm.DB, baselineID, path string) error {
	for _, row := range f.rows {
		if row.ID == baselineID {
			stored := path
			row.CalibrationAudioPath = &stored
			return nil
		}
	}
	return gorm.ErrRecordNotFound
}

// --- AI service ----------------------------------------------------------

type fakeAIService struct {
	baseline     *model.AIBaseline
	card         *model.AIHealthCard
	err          error
	gotLabel     string
	gotBaseline  []byte
	inspectCalls int
}

func (f *fakeAIService) Calibrate(_ context.Context, _ []byte, _, machineLabel string) (*model.AIBaseline, error) {
	f.gotLabel = machineLabel
	if f.err != nil {
		return nil, f.err
	}
	return f.baseline, nil
}

func (f *fakeAIService) Inspect(_ context.Context, _ []byte, _ string, baselineJSON []byte) (*model.AIHealthCard, error) {
	f.inspectCalls++
	f.gotBaseline = baselineJSON
	if f.err != nil {
		return nil, f.err
	}
	return f.card, nil
}

// --- object store --------------------------------------------------------

type fakeObjectStore struct {
	puts map[string][]byte
	err  error
}

func newFakeObjectStore() *fakeObjectStore {
	return &fakeObjectStore{puts: map[string][]byte{}}
}

func (f *fakeObjectStore) Put(_ context.Context, path string, content []byte, _ string) error {
	if f.err != nil {
		return f.err
	}
	f.puts[path] = content
	return nil
}
