package sequences

import (
	"context"
	"errors"
	"sync"
	"testing"

	"gorp/internal/base"
	"gorp/internal/record"
	"gorp/internal/sequencecore"
)

func TestNextByCodePrefersCurrentCompanyOverGlobal(t *testing.T) {
	sequencecore.ResetForTesting()
	env := testEnv(t, 2)
	globalID, err := env.Model("ir.sequence").Create(map[string]any{
		"name":             "Global",
		"code":             "sale.order",
		"prefix":           "G/",
		"padding":          int64(3),
		"number_next":      int64(4),
		"number_increment": int64(1),
		"active":           true,
	})
	if err != nil {
		t.Fatal(err)
	}
	companyID, err := env.Model("ir.sequence").Create(map[string]any{
		"name":             "Company",
		"code":             "sale.order",
		"prefix":           "C/",
		"padding":          int64(3),
		"number_next":      int64(8),
		"number_increment": int64(1),
		"company_id":       int64(2),
		"active":           true,
	})
	if err != nil {
		t.Fatal(err)
	}

	value, ok, err := (Service{Env: env}).NextByCode(context.Background(), "sale.order", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || value != "C/008" {
		t.Fatalf("next_by_code = value:%q ok:%v", value, ok)
	}
	globalRows, err := env.Model("ir.sequence").Browse(globalID).Read("number_next")
	if err != nil {
		t.Fatal(err)
	}
	companyRows, err := env.Model("ir.sequence").Browse(companyID).Read("number_next")
	if err != nil {
		t.Fatal(err)
	}
	if globalRows[0]["number_next"] != int64(4) || companyRows[0]["number_next"] != int64(8) {
		t.Fatalf("sequence counters global=%+v company=%+v", globalRows, companyRows)
	}
}

func TestNextByCodeFallsBackToGlobalAndReturnsFalseWhenMissing(t *testing.T) {
	sequencecore.ResetForTesting()
	env := testEnv(t, 5)
	if _, err := env.Model("ir.sequence").Create(map[string]any{
		"name":             "Global",
		"code":             "purchase.order",
		"prefix":           "P/",
		"padding":          int64(2),
		"number_next":      int64(3),
		"number_increment": int64(1),
		"active":           true,
	}); err != nil {
		t.Fatal(err)
	}
	value, ok, err := (Service{Env: env}).NextByCode(context.Background(), "purchase.order", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || value != "P/03" {
		t.Fatalf("fallback value = %q ok=%v", value, ok)
	}
	value, ok, err = (Service{Env: env}).NextByCode(context.Background(), "missing.code", nil)
	if err != nil {
		t.Fatal(err)
	}
	if ok || value != "" {
		t.Fatalf("missing value = %q ok=%v", value, ok)
	}
}

func TestNextByCodeTreatsMissingActiveAsActive(t *testing.T) {
	sequencecore.ResetForTesting()
	env := testEnv(t, 1)
	if _, err := env.Model("ir.sequence").Create(map[string]any{
		"name":             "No Active Field",
		"code":             "no.active",
		"prefix":           "N/",
		"padding":          int64(2),
		"number_next":      int64(5),
		"number_increment": int64(1),
	}); err != nil {
		t.Fatal(err)
	}
	value, ok, err := (Service{Env: env}).NextByCode(context.Background(), "no.active", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || value != "N/05" {
		t.Fatalf("next_by_code = value:%q ok:%v", value, ok)
	}
}

func TestStandardSequenceUsesBackendCounterAndWriteReset(t *testing.T) {
	sequencecore.ResetForTesting()
	env := testEnv(t, 1)
	sequenceID, err := env.Model("ir.sequence").Create(map[string]any{
		"name":             "Standard",
		"code":             "standard.seq",
		"prefix":           "S/",
		"padding":          int64(3),
		"number_next":      int64(4),
		"number_increment": int64(1),
		"active":           true,
		"implementation":   "standard",
	})
	if err != nil {
		t.Fatal(err)
	}
	service := Service{Env: env}
	first, err := service.NextByID(context.Background(), sequenceID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.NextByID(context.Background(), sequenceID)
	if err != nil {
		t.Fatal(err)
	}
	if first != "S/004" || second != "S/005" {
		t.Fatalf("standard values = %q %q", first, second)
	}
	rows, err := env.Model("ir.sequence").Browse(sequenceID).Read("number_next")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["number_next"] != int64(4) {
		t.Fatalf("standard row counter mutated: %+v", rows)
	}
	if err := env.Model("ir.sequence").Browse(sequenceID).Write(map[string]any{"number_next": int64(10)}); err != nil {
		t.Fatal(err)
	}
	reset, err := service.NextByID(context.Background(), sequenceID)
	if err != nil {
		t.Fatal(err)
	}
	if reset != "S/010" {
		t.Fatalf("reset value = %q", reset)
	}
}

func TestStandardSequenceConcurrentDrawsDistinct(t *testing.T) {
	sequencecore.ResetForTesting()
	env := testEnv(t, 1)
	sequenceID, err := env.Model("ir.sequence").Create(map[string]any{
		"name":             "Concurrent",
		"code":             "concurrent.seq",
		"prefix":           "C/",
		"padding":          int64(3),
		"number_next":      int64(1),
		"number_increment": int64(1),
		"active":           true,
	})
	if err != nil {
		t.Fatal(err)
	}
	service := Service{Env: env}
	var wg sync.WaitGroup
	values := make(chan string, 2)
	errs := make(chan error, 2)
	for range []int{0, 1} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, err := service.NextByID(context.Background(), sequenceID)
			if err != nil {
				errs <- err
				return
			}
			values <- value
		}()
	}
	wg.Wait()
	close(values)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	got := map[string]bool{}
	for value := range values {
		got[value] = true
	}
	if !got["C/001"] || !got["C/002"] || len(got) != 2 {
		t.Fatalf("concurrent values = %+v", got)
	}
}

func TestNoGapSequenceMutatesStoredCounterAndReportsLock(t *testing.T) {
	sequencecore.ResetForTesting()
	env := testEnv(t, 1)
	sequenceID, err := env.Model("ir.sequence").Create(map[string]any{
		"name":             "No Gap",
		"code":             "nogap.seq",
		"prefix":           "N/",
		"padding":          int64(2),
		"number_next":      int64(7),
		"number_increment": int64(2),
		"active":           true,
		"implementation":   "no_gap",
	})
	if err != nil {
		t.Fatal(err)
	}
	key := sequencecore.Key{Namespace: env.SequenceNamespace("ir.sequence"), Model: "ir.sequence", ID: sequenceID}
	unlock := sequencecore.LockNoGapForTesting(key)
	if _, err := (Service{Env: env}).NextByID(context.Background(), sequenceID); !errors.Is(err, sequencecore.ErrNoGapLocked) {
		t.Fatalf("locked no_gap error = %v", err)
	}
	unlock()
	value, err := (Service{Env: env}).NextByID(context.Background(), sequenceID)
	if err != nil {
		t.Fatal(err)
	}
	if value != "N/07" {
		t.Fatalf("no_gap value = %q", value)
	}
	rows, err := env.Model("ir.sequence").Browse(sequenceID).Read("number_next")
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["number_next"] != int64(9) {
		t.Fatalf("no_gap row counter = %+v", rows)
	}
}

func testEnv(t *testing.T, companyID int64) *record.Env {
	t.Helper()
	reg := record.NewRegistry()
	for _, model := range base.Models() {
		if err := reg.Register(model); err != nil {
			t.Fatal(err)
		}
	}
	return record.NewEnv(reg, record.Context{UserID: 1, CompanyID: companyID, CompanyIDs: []int64{companyID}})
}
