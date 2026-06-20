package accounting

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"gorp/internal/registry"
)

func TestModels(t *testing.T) {
	reg := registry.New("test")
	if err := RegisterModels(reg); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"account.account",
		"account.group",
		"account.root",
		"account.account.tag",
		"account.account.type",
		"account.bank.statement",
		"account.bank.statement.line",
		"account.cash.rounding",
		"account.chart.template",
		"account.code.mapping",
		"account.journal",
		"account.journal.group",
		"account.incoterms",
		"account.lock_exception",
		"account.move",
		"account.move.line",
		"account.move.reversal",
		"account.payment.register",
		"account.move.send.wizard",
		"account.move.send.batch.wizard",
		"account.payment",
		"account.payment.method",
		"account.payment.method.line",
		"account.payment.term",
		"account.payment.term.line",
		"account.tax",
		"account.tax.group",
		"account.tax.repartition.line",
		"account.fiscal.position",
		"account.fiscal.position.account",
		"account.document.import.mixin",
		"account.partial.reconcile",
		"account.full.reconcile",
		"account.reconcile.model",
		"account.reconcile.model.line",
		"account.report",
		"account.report.line",
		"account.report.expression",
		"account.report.column",
		"account.report.external.value",
		"account.invoice.report",
		"account.automatic.entry.wizard",
		"account.autopost.bills.wizard",
		"account.resequence.wizard",
		"account.secure.entries.wizard",
		"account.merge.wizard",
		"account.merge.wizard.line",
		"account.accrued.orders.wizard",
	} {
		if _, ok := reg.Models[name]; !ok {
			t.Fatalf("missing model %s", name)
		}
	}
	if !reg.Models["account.move.reversal"].Transient {
		t.Fatal("account.move.reversal must be transient")
	}
	if !reg.Models["account.payment.register"].Transient {
		t.Fatal("account.payment.register must be transient")
	}
	if !reg.Models["account.move.send.wizard"].Transient || !reg.Models["account.move.send.batch.wizard"].Transient {
		t.Fatal("account move send wizards must be transient")
	}
	for _, name := range []string{"account.automatic.entry.wizard", "account.autopost.bills.wizard", "account.resequence.wizard", "account.secure.entries.wizard", "account.merge.wizard", "account.merge.wizard.line", "account.accrued.orders.wizard"} {
		if !reg.Models[name].Transient {
			t.Fatalf("%s must be transient", name)
		}
	}
	if !reg.Models["account.document.import.mixin"].Abstract {
		t.Fatal("account.document.import.mixin must be abstract")
	}
}

func TestStructuralModelsExposeOdooFields(t *testing.T) {
	fieldsByModel := map[string]map[string]bool{}
	for _, m := range Models() {
		fields := map[string]bool{}
		for name := range m.Fields {
			fields[name] = true
		}
		fieldsByModel[m.Name] = fields
	}
	assertAccountingFields(t, fieldsByModel["account.account"], "placeholder_code", "root_id", "group_id", "tax_ids", "non_trade")
	assertAccountingFields(t, fieldsByModel["account.group"], "parent_id", "parent_path", "name", "code_prefix_start", "code_prefix_end", "company_id")
	assertAccountingFields(t, fieldsByModel["account.root"], "name", "parent_id")
	assertAccountingFields(t, fieldsByModel["account.journal.group"], "name", "company_id", "excluded_journal_ids", "sequence")
	assertAccountingFields(t, fieldsByModel["account.incoterms"], "name", "code", "active")
	assertAccountingFields(t, fieldsByModel["account.lock_exception"], "active", "state", "company_id", "user_id", "reason", "end_datetime", "lock_date_field", "lock_date", "company_lock_date", "fiscalyear_lock_date", "tax_lock_date", "sale_lock_date", "purchase_lock_date")
	assertAccountingFields(t, fieldsByModel["account.tax"], "description", "invoice_label", "fiscal_position_ids", "original_tax_ids", "children_tax_ids", "is_domestic")
	assertAccountingFields(t, fieldsByModel["account.tax.group"], "country_id", "tax_payable_account_id", "tax_receivable_account_id")
	assertAccountingFields(t, fieldsByModel["account.tax.repartition.line"], "tag_ids")
	assertAccountingFields(t, fieldsByModel["account.fiscal.position"], "account_ids", "tax_ids", "sequence")
	assertAccountingFields(t, fieldsByModel["account.fiscal.position.account"], "position_id", "company_id", "account_src_id", "account_dest_id")
	assertAccountingFields(t, fieldsByModel["account.move"], "fiscal_position_id", "ubl_cii_xml_file")
	assertAccountingFields(t, fieldsByModel["account.move.line"], "tax_ids", "tax_tag_ids")
	assertAccountingFields(t, fieldsByModel["account.invoice.report"], "move_id", "journal_id", "company_id", "company_currency_id", "partner_id", "commercial_partner_id", "country_id", "invoice_user_id", "move_type", "state", "payment_state", "fiscal_position_id", "invoice_date", "quantity", "product_id", "product_uom_id", "product_categ_id", "invoice_date_due", "account_id", "price_subtotal_currency", "price_subtotal", "price_total", "price_total_currency", "price_average", "price_margin", "inventory_value", "currency_id")
	assertAccountingFields(t, fieldsByModel["account.automatic.entry.wizard"], "action", "move_data", "preview_move_data", "move_line_ids", "date", "company_id", "company_currency_id", "percentage", "total_amount", "journal_id", "account_type", "expense_accrual_account", "revenue_accrual_account", "lock_date_message", "destination_account_id", "display_currency_helper")
	assertAccountingFields(t, fieldsByModel["account.autopost.bills.wizard"], "partner_id", "partner_name", "nb_unmodified_bills")
	assertAccountingFields(t, fieldsByModel["account.resequence.wizard"], "sequence_number_reset", "first_date", "end_date", "first_name", "ordering", "move_ids", "new_values", "preview_moves")
	assertAccountingFields(t, fieldsByModel["account.secure.entries.wizard"], "company_id", "country_code", "hash_date", "chains_to_hash_with_gaps", "max_hash_date", "unreconciled_bank_statement_line_ids", "not_hashable_unlocked_move_ids", "move_to_hash_ids", "warnings")
	assertAccountingFields(t, fieldsByModel["account.merge.wizard"], "account_ids", "is_group_by_name", "wizard_line_ids", "disable_merge_button")
	assertAccountingFields(t, fieldsByModel["account.merge.wizard.line"], "wizard_id", "grouping_key", "sequence", "display_type", "is_selected", "account_id", "company_ids", "info", "account_has_hashed_entries")
	assertAccountingFields(t, fieldsByModel["account.accrued.orders.wizard"], "company_id", "journal_id", "date", "reversal_date", "amount", "currency_id", "account_id", "preview_data", "display_amount")
}

func assertAccountingFields(t *testing.T, fields map[string]bool, names ...string) {
	t.Helper()
	for _, name := range names {
		if !fields[name] {
			t.Fatalf("missing field %s in %+v", name, fields)
		}
	}
}

func TestPostingBalancedMoveAssignsSequenceWithoutHashByDefault(t *testing.T) {
	move := balancedMove()
	seq := &Sequence{Prefix: "SAJ/", Next: 12}
	if err := PostMove(&move, seq, LockPolicy{}); err != nil {
		t.Fatal(err)
	}
	if move.State != MovePosted || !move.PostedBefore {
		t.Fatalf("state = %s posted_before = %t", move.State, move.PostedBefore)
	}
	if move.Name != "SAJ/0012" || seq.Next != 13 {
		t.Fatalf("sequence name = %s next = %d", move.Name, seq.Next)
	}
	if move.SequencePrefix != "SAJ/" || move.SequenceNumber != 12 || move.SecureSequenceNumber != 0 {
		t.Fatalf("sequence fields = %+v", move)
	}
	if move.AmountTotal != 10000 || move.AmountResidual != 10000 {
		t.Fatalf("totals = %d residual = %d", move.AmountTotal, move.AmountResidual)
	}
	if move.InalterableHash != "" {
		t.Fatalf("unexpected hash: %s", move.InalterableHash)
	}
}

func TestPostingRestrictedJournalAssignsHashAndSecureSequence(t *testing.T) {
	move := balancedMove()
	move.Journal.RestrictModeHashTable = true
	seq := &Sequence{Prefix: "SAJ/", Next: 12}
	if err := PostMoveWithOptions(&move, seq, LockPolicy{}, PostOptions{PreviousHash: "previous", LastSequencePrefix: "SAJ/", LastSequenceNumber: 11}); err != nil {
		t.Fatal(err)
	}
	if move.InalterableHash == "" || move.SecureSequenceNumber != 12 {
		t.Fatalf("hash fields = %+v", move)
	}
}

func TestPostingRestrictedJournalRejectsSequenceGap(t *testing.T) {
	move := balancedMove()
	move.Journal.RestrictModeHashTable = true
	seq := &Sequence{Prefix: "SAJ/", Next: 14}
	err := PostMoveWithOptions(&move, seq, LockPolicy{}, PostOptions{LastSequencePrefix: "SAJ/", LastSequenceNumber: 11})
	if !errors.Is(err, ErrSequenceGap) {
		t.Fatalf("error = %v", err)
	}
	if move.State != MoveDraft || move.Name != "" || move.SequenceNumber != 0 || move.MadeSequenceGap || seq.Next != 14 {
		t.Fatalf("gap mutated move=%+v seq=%+v", move, seq)
	}
}

func TestPostingRejectsInvalidMoves(t *testing.T) {
	tests := []struct {
		name string
		move Move
		lock LockPolicy
		err  error
	}{
		{
			name: "unbalanced",
			move: withLines(balancedMove(), []MoveLine{
				line(1, receivableAccount(), 10000, 0, 7),
				line(2, incomeAccount(), 0, 9000, 0),
			}),
			err: ErrMoveUnbalanced,
		},
		{
			name: "company mismatch",
			move: withLines(balancedMove(), []MoveLine{
				line(1, receivableAccount(), 10000, 0, 7),
				func() MoveLine {
					l := line(2, incomeAccount(), 0, 10000, 0)
					l.CompanyID = 2
					return l
				}(),
			}),
			err: ErrCompanyMismatch,
		},
		{
			name: "currency mismatch",
			move: withLines(balancedMove(), []MoveLine{
				line(1, receivableAccount(), 10000, 0, 7),
				func() MoveLine {
					l := line(2, incomeAccount(), 0, 10000, 0)
					l.Currency = "EUR"
					return l
				}(),
			}),
			err: ErrCurrencyMismatch,
		},
		{
			name: "partner required",
			move: withLines(balancedMove(), []MoveLine{
				line(1, receivableAccount(), 10000, 0, 0),
				line(2, incomeAccount(), 0, 10000, 0),
			}),
			err: ErrPartnerRequired,
		},
		{
			name: "empty lines",
			move: func() Move {
				m := balancedMove()
				m.Lines = nil
				return m
			}(),
			err: ErrMoveHasNoLines,
		},
		{
			name: "posted state",
			move: func() Move {
				m := balancedMove()
				m.State = MovePosted
				return m
			}(),
			err: ErrMoveNotDraft,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seq := &Sequence{Prefix: "MISC/", Next: 1}
			err := PostMove(&tt.move, seq, tt.lock)
			if !errors.Is(err, tt.err) {
				t.Fatalf("error = %v, want %v", err, tt.err)
			}
		})
	}
}

func TestPostingShiftsLockedMoveToOpenAccountingDate(t *testing.T) {
	tests := []struct {
		name string
		move Move
		lock LockPolicy
		opts PostOptions
		want time.Time
	}{
		{
			name: "customer invoice fiscal lock uses current open-period date",
			move: func() Move {
				m := balancedMove()
				m.Date = date(2016, 12, 31)
				m.InvoiceDate = date(2016, 1, 1)
				m.MoveType = "out_invoice"
				m.Journal.Type = JournalSale
				return m
			}(),
			lock: LockPolicy{FiscalLockDate: date(2016, 12, 31)},
			opts: PostOptions{AccountingDate: AccountingDateOptions{Today: date(2017, 1, 12)}},
			want: date(2017, 1, 12),
		},
		{
			name: "vendor bill fiscal lock uses current open-period date",
			move: func() Move {
				m := balancedMove()
				m.Date = date(2016, 1, 1)
				m.MoveType = "in_invoice"
				m.Journal.Type = JournalPurchase
				return m
			}(),
			lock: LockPolicy{FiscalLockDate: date(2016, 12, 31)},
			opts: PostOptions{AccountingDate: AccountingDateOptions{Today: date(2017, 1, 12)}},
			want: date(2017, 1, 12),
		},
		{
			name: "sale lock after open month closes at month end",
			move: func() Move {
				m := balancedMove()
				m.Date = date(2023, 1, 2)
				m.MoveType = "out_invoice"
				m.Journal.Type = JournalSale
				return m
			}(),
			lock: LockPolicy{SaleLockDate: date(2023, 2, 1)},
			opts: PostOptions{AccountingDate: AccountingDateOptions{Today: date(2023, 5, 1)}},
			want: date(2023, 2, 28),
		},
		{
			name: "purchase lock after open month closes at month end",
			move: func() Move {
				m := balancedMove()
				m.Date = date(2023, 1, 2)
				m.MoveType = "in_invoice"
				m.Journal.Type = JournalPurchase
				return m
			}(),
			lock: LockPolicy{PurchaseLockDate: date(2023, 2, 1)},
			opts: PostOptions{AccountingDate: AccountingDateOptions{Today: date(2023, 5, 1)}},
			want: date(2023, 2, 28),
		},
		{
			name: "tax lock shifts only taxed moves",
			move: func() Move {
				m := withLines(balancedMove(), []MoveLine{
					line(1, receivableAccount(), 10000, 0, 7),
					func() MoveLine {
						l := line(2, incomeAccount(), 0, 10000, 0)
						l.TaxID = 1
						return l
					}(),
				})
				m.Date = date(2017, 1, 1)
				return m
			}(),
			lock: LockPolicy{TaxLockDate: date(2017, 1, 31)},
			opts: PostOptions{AccountingDate: AccountingDateOptions{Today: date(2017, 2, 12)}},
			want: date(2017, 2, 12),
		},
		{
			name: "hard lock shifts to next open day when today is inside locked period",
			move: func() Move {
				m := balancedMove()
				m.Date = date(2026, 6, 1)
				return m
			}(),
			lock: LockPolicy{HardLockDate: date(2026, 6, 30)},
			opts: PostOptions{AccountingDate: AccountingDateOptions{Today: date(2026, 6, 17)}},
			want: date(2026, 7, 1),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seq := &Sequence{Prefix: "MISC/", Next: 1}
			if err := PostMoveWithOptions(&tt.move, seq, tt.lock, tt.opts); err != nil {
				t.Fatal(err)
			}
			if !tt.move.Date.Equal(tt.want) {
				t.Fatalf("date = %s, want %s", tt.move.Date.Format("2006-01-02"), tt.want.Format("2006-01-02"))
			}
			if tt.move.InvoiceDate.Equal(tt.want) {
				t.Fatalf("invoice date was shifted with accounting date: %s", tt.move.InvoiceDate.Format("2006-01-02"))
			}
			if tt.move.State != MovePosted {
				t.Fatalf("state = %s", tt.move.State)
			}
		})
	}
}

func TestAccountingDateKeepsUntaxedMoveOutsideTaxLock(t *testing.T) {
	move := balancedMove()
	move.Date = date(2017, 1, 1)
	got := AccountingDate(move, LockPolicy{TaxLockDate: date(2017, 1, 31)}, AccountingDateOptions{Today: date(2017, 2, 12)})
	if !got.Equal(move.Date) {
		t.Fatalf("date = %s, want %s", got.Format("2006-01-02"), move.Date.Format("2006-01-02"))
	}
}

func TestPostedMoveImmutableExceptAllowedFields(t *testing.T) {
	move := balancedMove()
	move.State = MovePosted
	if err := UpdatePostedFields(move, map[string]any{"ref": "customer ref"}, map[string]bool{"ref": true}); err != nil {
		t.Fatal(err)
	}
	if err := UpdatePostedFields(move, map[string]any{"invoice_payment_state": "paid"}, nil); err != nil {
		t.Fatal(err)
	}
	err := UpdatePostedFields(move, map[string]any{"date": date(2026, 7, 1)}, map[string]bool{"date": true})
	if !errors.Is(err, ErrPostedMoveImmutable) {
		t.Fatalf("error = %v", err)
	}
	err = UpdatePostedFields(move, map[string]any{"invoice_date": date(2026, 7, 1)}, nil)
	if !errors.Is(err, ErrPostedMoveImmutable) {
		t.Fatalf("invoice date error = %v", err)
	}
}

func TestButtonDraftCancelAndUnlinkProtection(t *testing.T) {
	move := balancedMove()
	move.State = MovePosted
	move.InalterableHash = "hash"
	if err := ButtonDraft(&move, LockPolicy{}); !errors.Is(err, ErrPostedMoveHashLocked) {
		t.Fatalf("button draft error = %v", err)
	}
	if err := ButtonCancel(&move, LockPolicy{}); !errors.Is(err, ErrPostedMoveHashLocked) {
		t.Fatalf("button cancel hash error = %v", err)
	}
	if err := CanUnlinkOrReverse(move, LockPolicy{}); !errors.Is(err, ErrPostedMoveProtected) {
		t.Fatalf("unlink error = %v", err)
	}

	move.InalterableHash = ""
	if err := ButtonDraft(&move, LockPolicy{RestrictiveAuditTrail: true}); !errors.Is(err, ErrPostedMoveProtected) {
		t.Fatalf("restrictive draft error = %v", err)
	}
	if err := ButtonCancel(&move, LockPolicy{RestrictiveAuditTrail: true}); !errors.Is(err, ErrPostedMoveProtected) {
		t.Fatalf("restrictive cancel error = %v", err)
	}

	if err := ButtonDraft(&move, LockPolicy{}); err != nil {
		t.Fatal(err)
	}
	if move.State != MoveDraft {
		t.Fatalf("state = %s", move.State)
	}
	if err := ButtonCancel(&move, LockPolicy{}); err != nil {
		t.Fatal(err)
	}
	if move.State != MoveCancel {
		t.Fatalf("state = %s", move.State)
	}
	if move.AutoPost != "no" {
		t.Fatalf("auto_post = %s", move.AutoPost)
	}
}

func TestCancellationRequestAndResetVisibility(t *testing.T) {
	move := lockedPostedMove(func(m *Move) {
		m.NeedCancelRequest = true
	})
	if err := ButtonDraft(&move, LockPolicy{}); !errors.Is(err, ErrCancelRequestRequired) {
		t.Fatalf("button draft error = %v", err)
	}
	if err := ButtonCancel(&move, LockPolicy{}); !errors.Is(err, ErrCancelRequestRequired) {
		t.Fatalf("button cancel error = %v", err)
	}
	if err := ButtonRequestCancel(move); err != nil {
		t.Fatalf("request cancel error = %v", err)
	}
	if ShowResetToDraftButton(move) {
		t.Fatal("reset button visible for cancellation-request move")
	}

	move.NeedCancelRequest = false
	if err := ButtonRequestCancel(move); !errors.Is(err, ErrCancelRequestNotFound) {
		t.Fatalf("request cancel missing error = %v", err)
	}
	if !ShowResetToDraftButton(move) {
		t.Fatal("reset button hidden for unrestricted posted move")
	}
	move.Journal.RestrictModeHashTable = true
	if ShowResetToDraftButton(move) {
		t.Fatal("reset button visible for restricted journal move")
	}
	move.Journal.RestrictModeHashTable = false
	move.InalterableHash = "hash"
	if ShowResetToDraftButton(move) {
		t.Fatal("reset button visible for hashed move")
	}

	cancelled := lockedPostedMove(nil)
	cancelled.State = MoveCancel
	if !ShowResetToDraftButton(cancelled) {
		t.Fatal("reset button hidden for cancellable move")
	}
	if err := ButtonDraft(&cancelled, LockPolicy{}); err != nil {
		t.Fatal(err)
	}
	if cancelled.State != MoveDraft {
		t.Fatalf("state = %s", cancelled.State)
	}
}

func TestPostedMoveTransitionsRespectLockDates(t *testing.T) {
	tests := []struct {
		name string
		move Move
		lock LockPolicy
		err  error
	}{
		{
			name: "fiscal",
			move: lockedPostedMove(func(m *Move) {
				m.Date = date(2026, 6, 1)
			}),
			lock: LockPolicy{FiscalLockDate: date(2026, 6, 30)},
			err:  ErrFiscalLockDate,
		},
		{
			name: "sale",
			move: lockedPostedMove(func(m *Move) {
				m.Date = date(2026, 6, 1)
				m.Journal.Type = JournalSale
			}),
			lock: LockPolicy{SaleLockDate: date(2026, 6, 30)},
			err:  ErrSaleLockDate,
		},
		{
			name: "purchase",
			move: lockedPostedMove(func(m *Move) {
				m.Date = date(2026, 6, 1)
				m.Journal.Type = JournalPurchase
			}),
			lock: LockPolicy{PurchaseLockDate: date(2026, 6, 30)},
			err:  ErrPurchaseLockDate,
		},
		{
			name: "tax",
			move: lockedPostedMove(func(m *Move) {
				m.Date = date(2026, 6, 1)
				m.Lines[1].TaxID = 1
			}),
			lock: LockPolicy{TaxLockDate: date(2026, 6, 30)},
			err:  ErrTaxLockDate,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			draftMove := tt.move
			if err := ButtonDraft(&draftMove, tt.lock); !errors.Is(err, tt.err) {
				t.Fatalf("button draft error = %v, want %v", err, tt.err)
			}
			cancelMove := tt.move
			if err := ButtonCancel(&cancelMove, tt.lock); !errors.Is(err, tt.err) {
				t.Fatalf("button cancel error = %v, want %v", err, tt.err)
			}
			if err := CanUnlink(tt.move, tt.lock); !errors.Is(err, tt.err) {
				t.Fatalf("unlink error = %v, want %v", err, tt.err)
			}
		})
	}
}

func TestCanReverseAllowsProtectedPostedMove(t *testing.T) {
	move := lockedPostedMove(func(m *Move) {
		m.InalterableHash = "hash"
		m.Date = date(2026, 6, 1)
	})
	locks := LockPolicy{RestrictiveAuditTrail: true, FiscalLockDate: date(2026, 6, 30)}
	if err := CanUnlink(move, locks); !errors.Is(err, ErrPostedMoveProtected) {
		t.Fatalf("unlink error = %v", err)
	}
	if err := CanReverse(move, locks); err != nil {
		t.Fatalf("reverse error = %v", err)
	}
}

func TestCanUnlinkBlocksPreviouslyPostedRestrictiveMove(t *testing.T) {
	for _, state := range []MoveState{MoveDraft, MoveCancel} {
		move := lockedPostedMove(func(m *Move) {
			m.State = state
			m.PostedBefore = true
			m.InalterableHash = ""
		})
		if err := CanUnlink(move, LockPolicy{RestrictiveAuditTrail: true}); !errors.Is(err, ErrPostedMoveProtected) {
			t.Fatalf("state %s unlink error = %v", state, err)
		}
	}
}

func TestReverseMoveBuildsDraftReversal(t *testing.T) {
	move := lockedPostedMove(func(m *Move) {
		m.Name = "SAJ/0012"
		m.InvoiceDate = date(2026, 7, 15)
		m.InalterableHash = "hash"
		m.SequencePrefix = "SAJ/"
		m.SequenceNumber = 12
		m.SecureSequenceNumber = 12
	})
	reversal := ReverseMove(move, 99, date(2026, 8, 1))
	if reversal.ID != 99 || reversal.ReversedEntryID != move.ID || reversal.State != MoveDraft {
		t.Fatalf("reversal identity = %+v", reversal)
	}
	if reversal.Name != "" || reversal.InalterableHash != "" || reversal.SequenceNumber != 0 || reversal.SecureSequenceNumber != 0 {
		t.Fatalf("reversal sequence fields = %+v", reversal)
	}
	if !reversal.Date.Equal(date(2026, 8, 1)) || !reversal.InvoiceDate.Equal(date(2026, 8, 1)) {
		t.Fatalf("reversal dates = %s %s", reversal.Date.Format("2006-01-02"), reversal.InvoiceDate.Format("2006-01-02"))
	}
	if reversal.Lines[0].Debit != 0 || reversal.Lines[0].Credit != 10000 || reversal.Lines[1].Debit != 10000 || reversal.Lines[1].Credit != 0 {
		t.Fatalf("reversal lines = %+v", reversal.Lines)
	}
	if reversal.AmountTotal != 10000 || reversal.AmountResidual != 10000 {
		t.Fatalf("reversal totals = %d %d", reversal.AmountTotal, reversal.AmountResidual)
	}
}

func TestMoveReversalWizardDefaultsAndBatch(t *testing.T) {
	move := lockedPostedMove(func(m *Move) {
		m.Name = "SAJ/0012"
		m.MoveType = "out_invoice"
		m.Journal = Journal{ID: 8, Type: JournalSale, CompanyID: 1}
		m.Currency = "BHD"
		m.AmountResidual = 6000
	})
	wizard, err := NewMoveReversal([]Move{move}, Journal{}, date(2026, 8, 1), "refund")
	if err != nil {
		t.Fatal(err)
	}
	if wizard.CompanyID != 1 || wizard.Journal.ID != 8 || wizard.Residual != 6000 || wizard.Currency != "BHD" || wizard.MoveType != "out_invoice" {
		t.Fatalf("wizard = %+v", wizard)
	}
	reversed, err := ReverseMoves(&wizard, []Move{move}, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(wizard.NewMoveIDs, []int64{100}) {
		t.Fatalf("new move ids = %+v", wizard.NewMoveIDs)
	}
	if len(reversed) != 1 || reversed[0].ID != 100 || reversed[0].MoveType != "out_refund" || reversed[0].Ref != "Reversal of: SAJ/0012, refund" || reversed[0].Journal.ID != 8 {
		t.Fatalf("reversed = %+v", reversed)
	}
	if !reversed[0].InvoiceDate.Equal(date(2026, 8, 1)) {
		t.Fatalf("invoice date = %s", reversed[0].InvoiceDate.Format("2006-01-02"))
	}
	action := ReversalAction(reversed)
	if action.ViewMode != "form" || action.ResID != 100 || action.Context["default_move_type"] != "out_refund" {
		t.Fatalf("single action = %+v", action)
	}
}

func TestReversalMoveTypeMap(t *testing.T) {
	tests := map[string]string{
		"entry":       "entry",
		"out_invoice": "out_refund",
		"out_refund":  "out_invoice",
		"in_invoice":  "in_refund",
		"in_refund":   "in_invoice",
		"out_receipt": "out_refund",
		"in_receipt":  "in_refund",
	}
	for moveType, want := range tests {
		if got := ReversalMoveType(moveType); got != want {
			t.Fatalf("%s -> %s, want %s", moveType, got, want)
		}
	}
}

func TestMoveReversalModifyReturnsReplacementDrafts(t *testing.T) {
	move := lockedPostedMove(func(m *Move) {
		m.Name = "BILL/0001"
		m.MoveType = "in_invoice"
		m.Journal = Journal{ID: 9, Type: JournalPurchase, CompanyID: 1}
		m.Currency = "BHD"
		m.Lines[0].ExcludeFromInvoiceTab = true
		m.Lines[1].TaxID = 1
	})
	wizard, err := NewMoveReversal([]Move{move}, Journal{}, date(2026, 8, 1), "modify")
	if err != nil {
		t.Fatal(err)
	}
	replacements, err := ReverseMoves(&wizard, []Move{move}, 200, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(replacements) != 1 || replacements[0].ID != 201 || replacements[0].State != MoveDraft || replacements[0].ReversedEntryID != 0 {
		t.Fatalf("replacement = %+v", replacements)
	}
	if len(replacements[0].Lines) != 0 {
		t.Fatalf("expected tax/payment lines filtered, got %+v", replacements[0].Lines)
	}
	if !reflect.DeepEqual(wizard.NewMoveIDs, []int64{201}) {
		t.Fatalf("new move ids = %+v", wizard.NewMoveIDs)
	}
	action := ReversalAction([]Move{
		{ID: 201, MoveType: "in_invoice"},
		{ID: 202, MoveType: "in_invoice"},
	})
	if action.ViewMode != "list,form" || !reflect.DeepEqual(action.Domain, []int64{201, 202}) || action.Context["default_move_type"] != "in_invoice" {
		t.Fatalf("list action = %+v", action)
	}
}

func TestMoveReversalWizardRejectsInvalidInputs(t *testing.T) {
	posted := lockedPostedMove(func(m *Move) {
		m.Journal = Journal{ID: 1, Type: JournalSale}
	})
	draft := posted
	draft.State = MoveDraft
	if _, err := NewMoveReversal(nil, Journal{}, date(2026, 8, 1), ""); !errors.Is(err, ErrReversalNoMoves) {
		t.Fatalf("empty error = %v", err)
	}
	if _, err := NewMoveReversal([]Move{draft}, Journal{}, date(2026, 8, 1), ""); !errors.Is(err, ErrReversalNoMoves) {
		t.Fatalf("draft error = %v", err)
	}
	otherCompany := posted
	otherCompany.CompanyID = 2
	if _, err := NewMoveReversal([]Move{posted, otherCompany}, Journal{}, date(2026, 8, 1), ""); !errors.Is(err, ErrReversalCompany) {
		t.Fatalf("company error = %v", err)
	}
	if _, err := NewMoveReversal([]Move{posted}, Journal{ID: 2, Type: JournalPurchase}, date(2026, 8, 1), ""); !errors.Is(err, ErrReversalJournalType) {
		t.Fatalf("journal error = %v", err)
	}
	if _, err := NewMoveReversal([]Move{posted}, Journal{ID: 3, Type: JournalSale, CompanyID: 2}, date(2026, 8, 1), ""); !errors.Is(err, ErrReversalCompany) {
		t.Fatalf("journal company error = %v", err)
	}
}

func TestPaymentRegisterDefaultsAndCreatePayments(t *testing.T) {
	move := lockedPostedMove(func(m *Move) {
		m.Name = "INV/001"
		m.MoveType = "out_invoice"
		m.Journal = Journal{ID: 8, Type: JournalSale, CompanyID: 1, DefaultAccountID: 101}
		m.AmountResidual = 6000
		m.AmountResidualSigned = 6000
	})
	register, err := NewPaymentRegister([]Move{move}, Journal{}, date(2026, 8, 1), 0, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if register.Amount != 6000 || register.SourceAmount != 6000 || register.PaymentType != "inbound" || register.PartnerType != "customer" || register.PartnerID != 7 {
		t.Fatalf("register = %+v", register)
	}
	if register.Journal.ID != 8 || register.Communication != "INV/001" || !reflect.DeepEqual(register.LineIDs, []int64{1}) {
		t.Fatalf("register defaults = %+v", register)
	}
	payments, paidMoves, err := CreateRegisteredPayments(register, []Move{move}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(payments) != 1 || payments[0].ID != 100 || payments[0].State != "paid" || payments[0].Amount != 6000 || !payments[0].IsReconciled {
		t.Fatalf("payments = %+v", payments)
	}
	if len(paidMoves) != 1 || paidMoves[0].PaymentState != PaymentPaid || paidMoves[0].AmountResidual != 0 || paidMoves[0].PaymentCount != 1 {
		t.Fatalf("paid moves = %+v", paidMoves)
	}
	if !paidMoves[0].Lines[0].Reconciled || paidMoves[0].Lines[0].PaymentID != 100 {
		t.Fatalf("paid lines = %+v", paidMoves[0].Lines)
	}
}

func TestPaymentRegisterRejectsInvalidInputs(t *testing.T) {
	posted := lockedPostedMove(func(m *Move) {
		m.MoveType = "entry"
	})
	if _, err := NewPaymentRegister(nil, Journal{}, date(2026, 8, 1), 0, "", false); !errors.Is(err, ErrPaymentRegisterNoMoves) {
		t.Fatalf("empty error = %v", err)
	}
	if _, err := NewPaymentRegister([]Move{posted}, Journal{}, date(2026, 8, 1), 0, "", false); !errors.Is(err, ErrPaymentRegisterNoMoves) {
		t.Fatalf("entry error = %v", err)
	}
	invoice := lockedPostedMove(func(m *Move) {
		m.MoveType = "out_invoice"
	})
	otherCompany := invoice
	otherCompany.CompanyID = 2
	if _, err := NewPaymentRegister([]Move{invoice, otherCompany}, Journal{}, date(2026, 8, 1), 0, "", false); !errors.Is(err, ErrPaymentRegisterCompany) {
		t.Fatalf("company error = %v", err)
	}
	if _, err := NewPaymentRegister([]Move{invoice}, Journal{ID: 9, Type: JournalBank, CompanyID: 2}, date(2026, 8, 1), 0, "", false); !errors.Is(err, ErrPaymentRegisterCompany) {
		t.Fatalf("journal company error = %v", err)
	}
}

func TestTaxComputationAndFiscalPosition(t *testing.T) {
	taxes := []Tax{
		{ID: 2, Name: "VAT 5", AmountType: TaxPercent, RateBasisPoints: 500, Account: Account{ID: 2200}, Sequence: 20},
		{ID: 1, Name: "Fee", AmountType: TaxFixed, FixedAmount: 125, Account: Account{ID: 2300}, Sequence: 10},
	}
	lines := ComputeTaxLines(10000, taxes)
	if got := []int64{lines[0].TaxID, lines[1].TaxID}; !reflect.DeepEqual(got, []int64{1, 2}) {
		t.Fatalf("tax order = %+v", got)
	}
	if lines[0].Amount != 125 || lines[1].Amount != 500 {
		t.Fatalf("tax lines = %+v", lines)
	}

	fp := FiscalPosition{
		AccountMapping: map[int64]int64{4000: 4010},
		TaxMappings:    map[int64][]int64{3: []int64{30, 31}},
		TaxMapping:     map[int64]int64{2: 20},
	}
	if fp.MapAccount(4000) != 4010 || fp.MapAccount(9999) != 9999 {
		t.Fatalf("account mapping failed")
	}
	if got := fp.MapTaxes([]int64{1, 2}); !reflect.DeepEqual(got, []int64{1, 20}) {
		t.Fatalf("tax mapping = %+v", got)
	}
	if got := fp.MapTaxes([]int64{3}); !reflect.DeepEqual(got, []int64{30, 31}) {
		t.Fatalf("multi tax mapping = %+v", got)
	}
	removeTax := FiscalPosition{TaxMapping: map[int64]int64{2: 0}}
	if got := removeTax.MapTaxes([]int64{1, 2}); !reflect.DeepEqual(got, []int64{1}) {
		t.Fatalf("tax removal mapping = %+v", got)
	}
}

func TestFiscalPositionAccountLines(t *testing.T) {
	lines := []FiscalPositionAccountLine{
		{ID: 1, PositionID: 9, CompanyID: 1, SourceAccountID: 4000, DestinationAccountID: 4010},
		{ID: 2, PositionID: 9, CompanyID: 2, SourceAccountID: 4000, DestinationAccountID: 4020},
		{ID: 3, PositionID: 9, SourceAccountID: 5000, DestinationAccountID: 5010},
		{ID: 4, PositionID: 9, CompanyID: 1, SourceAccountID: 6000, DestinationAccountID: 6010},
		{ID: 5, PositionID: 9, CompanyID: 1, SourceAccountID: 6000, DestinationAccountID: 6020},
	}
	fp := FiscalPosition{
		AccountLines:   lines,
		AccountMapping: map[int64]int64{7000: 7010},
	}
	if got := fp.MapAccountForCompany(4000, 1); got != 4010 {
		t.Fatalf("company 1 mapping = %d", got)
	}
	if got := fp.MapAccountForCompany(4000, 2); got != 4020 {
		t.Fatalf("company 2 mapping = %d", got)
	}
	if got := fp.MapAccountForCompany(5000, 1); got != 5010 {
		t.Fatalf("global mapping = %d", got)
	}
	if got := fp.MapAccountForCompany(6000, 1); got != 6020 {
		t.Fatalf("deterministic duplicate-source mapping = %d", got)
	}
	if got := fp.MapAccount(7000); got != 7010 {
		t.Fatalf("fallback map mapping = %d", got)
	}
	if err := ValidateFiscalPositionAccountLines(lines); err != nil {
		t.Fatalf("valid mappings rejected: %v", err)
	}
	duplicate := append(append([]FiscalPositionAccountLine{}, lines...), FiscalPositionAccountLine{ID: 6, PositionID: 9, SourceAccountID: 5000, DestinationAccountID: 5010})
	if err := ValidateFiscalPositionAccountLines(duplicate); !errors.Is(err, ErrFiscalPositionMapping) {
		t.Fatalf("duplicate mapping error = %v", err)
	}
	if err := CanChangeFiscalPositionMappedAccount(4010, lines); !errors.Is(err, ErrAccountMapped) {
		t.Fatalf("mapped account change error = %v", err)
	}
	if err := CanChangeFiscalPositionMappedAccount(9999, lines); err != nil {
		t.Fatalf("unmapped account rejected: %v", err)
	}

	moveLines := []MoveLine{{Account: Account{ID: 4000, Name: "Revenue"}}, {Account: Account{ID: 9000}}}
	mapped := ApplyFiscalPositionToLines(moveLines, fp, 1)
	if mapped[0].Account.ID != 4010 || mapped[0].Account.Name != "Revenue" || mapped[1].Account.ID != 9000 {
		t.Fatalf("applied lines = %+v", mapped)
	}
	if moveLines[0].Account.ID != 4000 {
		t.Fatalf("source lines mutated: %+v", moveLines)
	}
}

func TestFiscalPositionInvoiceLinePreparation(t *testing.T) {
	fp := FiscalPosition{
		AccountLines: []FiscalPositionAccountLine{
			{ID: 1, CompanyID: 1, SourceAccountID: 4000, DestinationAccountID: 4010},
			{ID: 2, CompanyID: 1, SourceAccountID: 5000, DestinationAccountID: 5010},
			{ID: 3, CompanyID: 1, SourceAccountID: 1100, DestinationAccountID: 1110},
		},
		TaxMapping: map[int64]int64{1: 10, 2: 20, 3: 0},
	}
	sale := PrepareInvoiceLine(InvoiceLinePreparation{
		MoveType:       "out_invoice",
		CompanyID:      1,
		CurrentAccount: Account{ID: 4999, Name: "Fallback"},
		Product:        ProductFiscalAccounts{IncomeAccountID: 4000, ExpenseAccountID: 5000, CustomerTaxIDs: []int64{1, 3}, SupplierTaxIDs: []int64{2}},
		AccountTaxIDs:  []int64{9},
		FiscalPosition: fp,
	})
	if sale.Account.ID != 4010 || sale.Account.Name != "Fallback" || !reflect.DeepEqual(sale.TaxIDs, []int64{10}) {
		t.Fatalf("sale prepared line = %+v", sale)
	}
	purchase := PrepareInvoiceLine(InvoiceLinePreparation{
		MoveType:       "in_invoice",
		CompanyID:      1,
		CurrentAccount: Account{ID: 5999},
		Product:        ProductFiscalAccounts{IncomeAccountID: 4000, ExpenseAccountID: 5000, CustomerTaxIDs: []int64{1}, SupplierTaxIDs: []int64{2}},
		FiscalPosition: fp,
	})
	if purchase.Account.ID != 5010 || !reflect.DeepEqual(purchase.TaxIDs, []int64{20}) {
		t.Fatalf("purchase prepared line = %+v", purchase)
	}
	paymentTerm := PrepareInvoiceLine(InvoiceLinePreparation{
		MoveType:           "out_invoice",
		CompanyID:          1,
		CurrentAccount:     Account{ID: 4000},
		PaymentTermAccount: Account{ID: 1100, Name: "Receivable"},
		Product:            ProductFiscalAccounts{IncomeAccountID: 4000, CustomerTaxIDs: []int64{1}},
		FiscalPosition:     fp,
	})
	if paymentTerm.Account.ID != 1110 || paymentTerm.Account.Name != "Receivable" || !reflect.DeepEqual(paymentTerm.TaxIDs, []int64{10}) {
		t.Fatalf("payment term prepared line = %+v", paymentTerm)
	}
	accountTax := PrepareInvoiceLine(InvoiceLinePreparation{
		MoveType:       "out_invoice",
		CompanyID:      1,
		CurrentAccount: Account{ID: 7000},
		AccountTaxIDs:  []int64{1, 2},
		FiscalPosition: fp,
	})
	if accountTax.Account.ID != 7000 || !reflect.DeepEqual(accountTax.TaxIDs, []int64{10, 20}) {
		t.Fatalf("account tax prepared line = %+v", accountTax)
	}
}

func TestAccountRootAndGroupBehavior(t *testing.T) {
	root, ok := AccountRootFromCode("101000")
	if !ok || root.ID != "10" || root.ParentID != "1" {
		t.Fatalf("root = %+v ok=%v", root, ok)
	}
	root, ok = AccountRootFromCode("AB123")
	if !ok || root.ID != "AB" || root.ParentID != "A" {
		t.Fatalf("alphanumeric root = %+v ok=%v", root, ok)
	}
	if _, ok := AccountRootFromCode(""); ok {
		t.Fatal("empty code produced root")
	}
	if matches, err := AccountRootMatches("10", "child_of", "1"); err != nil || !matches {
		t.Fatalf("child_of match = %v %v", matches, err)
	}
	if matches, err := AccountRootMatches("10", "child_of", "2"); err != nil || matches {
		t.Fatalf("child_of non-match = %v %v", matches, err)
	}
	if matches, err := AccountRootMatches("10", "in", "10", "11"); err != nil || !matches {
		t.Fatalf("in match = %v %v", matches, err)
	}
	if _, err := AccountRootMatches("10", "="); !errors.Is(err, ErrAccountRootSearch) {
		t.Fatalf("unsupported root search error = %v", err)
	}

	groups := []AccountGroup{
		{ID: 1, CodePrefixStart: "1", CodePrefixEnd: "1", CompanyID: 1},
		{ID: 2, CodePrefixStart: "10", CodePrefixEnd: "19", CompanyID: 1},
		{ID: 3, CodePrefixStart: "101", CodePrefixEnd: "101", CompanyID: 1},
		{ID: 4, CodePrefixStart: "10", CodePrefixEnd: "10", CompanyID: 2},
	}
	if err := ValidateAccountGroups(groups); err != nil {
		t.Fatalf("valid groups rejected: %v", err)
	}
	if err := ValidateAccountGroups([]AccountGroup{{CodePrefixStart: "10", CodePrefixEnd: "100", CompanyID: 1}}); !errors.Is(err, ErrAccountGroupPrefix) {
		t.Fatalf("prefix length error = %v", err)
	}
	if err := ValidateAccountGroups([]AccountGroup{{CodePrefixStart: "10", CodePrefixEnd: "20", CompanyID: 1}, {CodePrefixStart: "15", CodePrefixEnd: "16", CompanyID: 1}}); !errors.Is(err, ErrAccountGroupOverlap) {
		t.Fatalf("overlap error = %v", err)
	}
	group, ok := AccountGroupForCode(groups, "101200", 1)
	if !ok || group.ID != 3 {
		t.Fatalf("group for code = %+v ok=%v", group, ok)
	}
	group, ok = AccountGroupForCode(groups, "109999", 2)
	if !ok || group.ID != 4 {
		t.Fatalf("company-specific group for code = %+v ok=%v", group, ok)
	}
	classification := ClassifyAccount(" 101200 ", groups, 1)
	if classification.PlaceholderCode != "101200" || classification.RootID != "10" || classification.GroupID != 3 {
		t.Fatalf("classification = %+v", classification)
	}
	alpha := ClassifyAccount("AB123", groups, 1)
	if alpha.PlaceholderCode != "AB123" || alpha.RootID != "AB" || alpha.GroupID != 0 {
		t.Fatalf("alphanumeric classification = %+v", alpha)
	}
	synced := SyncAccountGroupParents(groups)
	if synced[1].ParentID != 1 || synced[2].ParentID != 2 || synced[3].ParentID != 0 {
		t.Fatalf("synced parents = %+v", synced)
	}
}

func TestAccountHelperParity(t *testing.T) {
	values := AccountTypeOnchange(AccountFormValues{AccountType: AccountOffBalance, TaxIDs: []int64{1, 2}})
	if len(values.TaxIDs) != 0 {
		t.Fatalf("off-balance taxes = %+v", values.TaxIDs)
	}
	values = AccountTypeOnchange(AccountFormValues{AccountType: AccountIncome, TaxIDs: []int64{1, 2}})
	if !reflect.DeepEqual(values.TaxIDs, []int64{1, 2}) {
		t.Fatalf("income taxes = %+v", values.TaxIDs)
	}
	code, name, ok := SplitAccountCodeName("550003 Existing Account")
	if !ok || code != "550003" || name != "Existing Account" {
		t.Fatalf("split = %q %q %v", code, name, ok)
	}
	if code, name, ok := SplitAccountCodeName("Existing Account"); ok || code != "" || name != "Existing Account" {
		t.Fatalf("non-split = %q %q %v", code, name, ok)
	}
	companies := []CompanyRelation{{ID: 1}, {ID: 2, ParentID: 1}, {ID: 3}}
	accounts := []AccountCodeRecord{{ID: 1, Code: "101200", CompanyID: 1}, {ID: 2, Code: "101200", CompanyID: 3}}
	if err := ValidateAccountCodeUnique(accounts, AccountCodeRecord{ID: 3, Code: "101200", CompanyID: 2}, companies); !errors.Is(err, ErrAccountCodeDuplicate) {
		t.Fatalf("child duplicate error = %v", err)
	}
	if err := ValidateAccountCodeUnique(accounts, AccountCodeRecord{ID: 3, Code: "101200", CompanyID: 4}, companies); err != nil {
		t.Fatalf("unrelated company duplicate error = %v", err)
	}
	if err := ValidateAccountCodeUnique(accounts, AccountCodeRecord{ID: 1, Code: "101200", CompanyID: 1}, companies); err != nil {
		t.Fatalf("same account duplicate error = %v", err)
	}
}

func TestLockExceptionLifecycle(t *testing.T) {
	locks := LockPolicy{TaxLockDate: date(2026, 3, 31)}
	if _, err := NewLockException(LockException{}, locks); !errors.Is(err, ErrLockExceptionFields) {
		t.Fatalf("zero fields error = %v", err)
	}
	if _, err := NewLockException(LockException{TaxLockDate: date(2026, 1, 31), SaleLockDate: date(2026, 2, 28)}, locks); !errors.Is(err, ErrLockExceptionFields) {
		t.Fatalf("multiple fields error = %v", err)
	}
	exception, err := NewLockException(LockException{CompanyID: 1, TaxLockDate: date(2026, 1, 31)}, locks)
	if err != nil {
		t.Fatal(err)
	}
	if !exception.Active || exception.LockDateField != LockTax || !exception.LockDate.Equal(date(2026, 1, 31)) || !exception.CompanyLockDate.Equal(date(2026, 3, 31)) {
		t.Fatalf("normalized exception = %+v", exception)
	}
	now := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	if got := (LockException{Active: true, EndDatetime: now.Add(-time.Hour)}).StateAt(now); got != LockExceptionExpired {
		t.Fatalf("expired state = %s", got)
	}
	revoked, err := RevokeLockException(exception, now, true)
	if err != nil {
		t.Fatal(err)
	}
	if revoked.StateAt(now) != LockExceptionRevoked || revoked.Active {
		t.Fatalf("revoked exception = %+v", revoked)
	}
	if _, err := RevokeLockException(exception, now, false); !errors.Is(err, ErrLockExceptionAccess) {
		t.Fatalf("revoke access error = %v", err)
	}

	active := []LockException{
		{ID: 1, Active: true, CompanyID: 1, LockDateField: LockTax},
		{ID: 2, Active: true, CompanyID: 1, LockDateField: LockTax, LockDate: date(2026, 1, 31)},
		{ID: 3, Active: true, CompanyID: 2, LockDateField: LockTax},
		{ID: 4, Active: false, CompanyID: 1, LockDateField: LockTax},
	}
	matches := ActiveLockExceptions(active, 1, LockTax, date(2026, 1, 15), date(2026, 3, 31), now)
	if got := []int64{matches[0].ID, matches[1].ID}; !reflect.DeepEqual(got, []int64{1, 2}) {
		t.Fatalf("active exceptions = %+v", matches)
	}
	if matches := ActiveLockExceptions(active, 1, LockTax, date(2026, 4, 1), date(2026, 3, 31), now); len(matches) != 0 {
		t.Fatalf("unexpected active exceptions = %+v", matches)
	}
}

func TestEffectiveLockPolicyAppliesLockExceptions(t *testing.T) {
	now := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	locks := LockPolicy{
		FiscalLockDate:        date(2026, 3, 31),
		TaxLockDate:           date(2026, 2, 28),
		SaleLockDate:          date(2026, 1, 31),
		PurchaseLockDate:      date(2026, 1, 31),
		HardLockDate:          date(2026, 4, 30),
		RestrictiveAuditTrail: true,
	}
	exceptions := []LockException{
		{ID: 1, Active: true, CompanyID: 1, UserID: 7, LockDateField: LockFiscalYear, LockDate: date(2026, 1, 31), CompanyLockDate: date(2026, 3, 31)},
		{ID: 2, Active: true, CompanyID: 1, LockDateField: LockTax, CompanyLockDate: date(2026, 2, 28)},
		{ID: 3, Active: true, CompanyID: 1, UserID: 8, LockDateField: LockSale, LockDate: date(2025, 12, 31), CompanyLockDate: date(2026, 1, 31)},
		{ID: 4, Active: false, CompanyID: 1, UserID: 7, LockDateField: LockPurchase, LockDate: date(2025, 12, 31), CompanyLockDate: date(2026, 1, 31)},
		{ID: 5, Active: true, CompanyID: 2, UserID: 7, LockDateField: LockPurchase, LockDate: date(2025, 12, 31), CompanyLockDate: date(2026, 1, 31)},
		{ID: 6, Active: true, CompanyID: 1, UserID: 7, LockDateField: LockPurchase, LockDate: date(2026, 2, 28), CompanyLockDate: date(2026, 1, 31)},
		{ID: 7, Active: true, CompanyID: 1, UserID: 7, LockDateField: LockFiscalYear, LockDate: date(2025, 12, 31), CompanyLockDate: date(2026, 2, 28)},
		{ID: 8, Active: true, CompanyID: 1, UserID: 7, LockDateField: LockFiscalYear, LockDate: date(2025, 12, 31), CompanyLockDate: date(2026, 3, 31), EndDatetime: now.Add(-time.Hour)},
	}
	effective := ApplyLockExceptions(locks, 1, 7, exceptions, now)
	if !effective.FiscalLockDate.Equal(date(2026, 1, 31)) || !effective.TaxLockDate.IsZero() || !effective.SaleLockDate.Equal(date(2026, 1, 31)) || !effective.PurchaseLockDate.Equal(date(2026, 1, 31)) || !effective.HardLockDate.Equal(date(2026, 4, 30)) || !effective.RestrictiveAuditTrail {
		t.Fatalf("effective locks = %+v", effective)
	}
	move := withLines(balancedMove(), []MoveLine{
		line(1, receivableAccount(), 10000, 0, 7),
		line(2, incomeAccount(), 0, 10000, 0),
	})
	move.Date = date(2026, 2, 15)
	move.InvoiceDate = move.Date
	softLocks := locks
	softLocks.HardLockDate = time.Time{}
	softEffective := ApplyLockExceptions(softLocks, 1, 7, exceptions, now)
	if err := validateMove(move, softLocks); !errors.Is(err, ErrFiscalLockDate) {
		t.Fatalf("base lock error = %v", err)
	}
	if err := validateMove(move, softEffective); err != nil {
		t.Fatalf("effective lock should pass: %v", err)
	}
	wrongUser := ApplyLockExceptions(locks, 1, 9, exceptions, now)
	if !wrongUser.FiscalLockDate.Equal(date(2026, 3, 31)) || !wrongUser.SaleLockDate.Equal(date(2026, 1, 31)) {
		t.Fatalf("wrong user locks = %+v", wrongUser)
	}

	parentChain := []CompanyLockPolicy{
		{CompanyID: 9, Locks: LockPolicy{FiscalLockDate: date(2026, 5, 31)}},
		{CompanyID: 1, Locks: locks},
	}
	effective = EffectiveLockPolicy(parentChain, 7, exceptions, now)
	if !effective.FiscalLockDate.Equal(date(2026, 5, 31)) {
		t.Fatalf("parent lock without parent exception = %+v", effective)
	}
	parentException := append(append([]LockException{}, exceptions...), LockException{ID: 9, Active: true, CompanyID: 9, UserID: 7, LockDateField: LockFiscalYear, LockDate: date(2025, 12, 31), CompanyLockDate: date(2026, 5, 31)})
	effective = EffectiveLockPolicy(parentChain, 7, parentException, now)
	if !effective.FiscalLockDate.Equal(date(2026, 1, 31)) || !effective.HardLockDate.Equal(date(2026, 4, 30)) {
		t.Fatalf("parent lock with parent exception = %+v", effective)
	}
	removed, err := NewLockException(LockException{CompanyID: 1, UserID: 7, LockDateField: LockFiscalYear}, locks)
	if err != nil {
		t.Fatalf("lock removal exception error = %v", err)
	}
	removedEffective := ApplyLockExceptions(locks, 1, 7, []LockException{removed}, now)
	if !removedEffective.FiscalLockDate.IsZero() || !removedEffective.HardLockDate.Equal(locks.HardLockDate) {
		t.Fatalf("lock removal effective policy = %+v", removedEffective)
	}
}

func TestInvoiceReportProjection(t *testing.T) {
	move := Move{
		ID:                  1,
		MoveType:            "out_invoice",
		State:               MoveDraft,
		PaymentState:        PaymentNotPaid,
		Journal:             Journal{ID: 10},
		CompanyID:           1,
		CompanyCurrencyID:   100,
		CurrencyID:          200,
		PartnerID:           7,
		CommercialPartnerID: 8,
		CountryID:           9,
		InvoiceUserID:       11,
		FiscalPositionID:    12,
		InvoiceDate:         date(2026, 1, 10),
		InvoiceDateDue:      date(2026, 2, 10),
		InvoiceCurrencyRate: 2,
		Lines: []MoveLine{
			{Account: Account{ID: 4000}, DisplayType: "product", Quantity: 4, PriceSubtotal: 1000, PriceTotal: 1100, Credit: 500, ProductID: 30, ProductUOMID: 40, ProductCategoryID: 50, ProductUOMFactor: 1, TemplateUOMFactor: 1, StandardPrice: 100},
			{Account: Account{ID: 4001}, DisplayType: "line_section", Quantity: 9, PriceSubtotal: 900},
			{DisplayType: "product", Quantity: 9, PriceSubtotal: 900},
		},
	}
	rows := BuildInvoiceReportRows([]Move{move, {ID: 2, MoveType: "entry", Lines: move.Lines}})
	if len(rows) != 1 {
		t.Fatalf("rows = %+v", rows)
	}
	row := rows[0]
	if row.MoveID != 1 || row.AccountID != 4000 || row.Quantity != 4 || row.PriceSubtotalCurrency != 1000 || row.PriceSubtotal != 1000 || row.PriceTotal != 550 || row.PriceTotalCurrency != 1100 {
		t.Fatalf("row amounts = %+v", row)
	}
	if row.PriceMargin != 600 || row.InventoryValue != 400 || row.CommercialPartnerID != 8 || row.CurrencyID != 200 {
		t.Fatalf("row projection = %+v", row)
	}
	totals := AggregateInvoiceReportRows(rows)
	if totals.Quantity != 4 || totals.Subtotal != 1000 || totals.PriceAverage != 250 || totals.Margin != 600 || totals.InventoryValue != 400 {
		t.Fatalf("totals = %+v", totals)
	}

	signs := map[string]int64{
		"out_invoice": 1,
		"in_invoice":  -1,
		"out_refund":  -1,
		"in_refund":   1,
		"out_receipt": 1,
		"in_receipt":  -1,
	}
	for moveType, sign := range signs {
		rows := BuildInvoiceReportRows([]Move{{
			ID:       3,
			MoveType: moveType,
			Lines:    []MoveLine{{Account: Account{ID: 4000}, DisplayType: "product", Quantity: 2, PriceSubtotal: 300, PriceTotal: 330}},
		}})
		if len(rows) != 1 || rows[0].Quantity != float64(sign)*2 || rows[0].PriceSubtotalCurrency != sign*300 {
			t.Fatalf("%s sign row = %+v", moveType, rows)
		}
	}

	uomRows := BuildInvoiceReportRows([]Move{{ID: 4, MoveType: "out_invoice", Lines: []MoveLine{{Account: Account{ID: 4000}, DisplayType: "product", Quantity: 6, PriceSubtotal: 600, ProductUOMFactor: 3, TemplateUOMFactor: 1}}}})
	if len(uomRows) != 1 || uomRows[0].Quantity != 2 {
		t.Fatalf("uom converted row = %+v", uomRows)
	}
	zeroFactorRows := BuildInvoiceReportRows([]Move{{ID: 5, MoveType: "out_invoice", Lines: []MoveLine{{Account: Account{ID: 4000}, DisplayType: "product", Quantity: 6, PriceSubtotal: 600}}}})
	if len(zeroFactorRows) != 1 || zeroFactorRows[0].Quantity != 6 {
		t.Fatalf("zero factor row = %+v", zeroFactorRows)
	}
}

func TestReconcile(t *testing.T) {
	account := receivableAccount()
	debit := line(1, account, 10000, 0, 7)
	credit := line(2, account, 0, 4000, 7)
	result, err := ReconcileLines(debit, credit, 0, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.Partial.Amount != 4000 || result.DebitResidual != 6000 || result.CreditResidual != 0 {
		t.Fatalf("partial result = %+v", result)
	}
	if result.FullyReconciled || result.Full != nil {
		t.Fatalf("unexpected full reconcile = %+v", result)
	}

	credit = line(3, account, 0, 6000, 7)
	result, err = ReconcileLines(result.Debit, credit, 0, 11, 99)
	if err != nil {
		t.Fatal(err)
	}
	if !result.FullyReconciled || result.Full == nil || result.Full.Name != "FULL/0099" {
		t.Fatalf("full result = %+v", result)
	}
	if result.Partial.FullReconcileID != 99 {
		t.Fatalf("full reconcile id = %+v", result.Partial)
	}
}

func TestReconcileMoveLinesFullMultiLine(t *testing.T) {
	account := receivableAccount()
	lines := []MoveLine{
		line(1, account, 6000, 0, 7),
		line(2, account, 4000, 0, 8),
		line(3, account, 0, 4000, 8),
		line(4, account, 0, 6000, 7),
	}

	result, err := ReconcileMoveLines(lines, ReconcileOptions{PartialStartID: 100, FullID: 200})
	if err != nil {
		t.Fatal(err)
	}
	if !result.FullyReconciled || result.Full == nil || result.Full.Name != "FULL/0200" {
		t.Fatalf("full result = %+v", result)
	}
	if len(result.Partials) != 2 || !reflect.DeepEqual(result.Full.PartialReconcileIDs, []int64{100, 101}) {
		t.Fatalf("partials = %+v full=%+v", result.Partials, result.Full)
	}
	if !reflect.DeepEqual(result.Full.ReconciledLineIDs, []int64{1, 2, 3, 4}) {
		t.Fatalf("full line ids = %+v", result.Full.ReconciledLineIDs)
	}
	if result.Partials[0].DebitLineID != 1 || result.Partials[0].CreditLineID != 4 || result.Partials[0].Amount != 6000 {
		t.Fatalf("partner-priority first partial = %+v", result.Partials[0])
	}
	if result.Partials[1].DebitLineID != 2 || result.Partials[1].CreditLineID != 3 || result.Partials[1].Amount != 4000 {
		t.Fatalf("partner-priority second partial = %+v", result.Partials[1])
	}
	for _, line := range result.Lines {
		if !line.Reconciled || line.Residual != 0 || line.ResidualCurrency != 0 || line.FullReconcileID != 200 {
			t.Fatalf("line not fully reconciled = %+v", line)
		}
	}
}

func TestReconcileMoveLinesPartial(t *testing.T) {
	account := receivableAccount()
	result, err := ReconcileMoveLines([]MoveLine{
		line(1, account, 10000, 0, 7),
		line(2, account, 0, 4000, 7),
	}, ReconcileOptions{PartialStartID: 10, FullID: 99})
	if err != nil {
		t.Fatal(err)
	}
	if result.FullyReconciled || result.Full != nil || len(result.Partials) != 1 {
		t.Fatalf("partial result = %+v", result)
	}
	if result.DebitResidual != 6000 || result.CreditResidual != 0 || result.ReconcileBalance != 6000 {
		t.Fatalf("residuals = %+v", result)
	}
	if result.Lines[0].Residual != 6000 || result.Lines[0].Reconciled || !result.Lines[1].Reconciled {
		t.Fatalf("lines = %+v", result.Lines)
	}
}

func TestReconcileMoveLinesWithWriteoff(t *testing.T) {
	account := receivableAccount()
	writeoff := line(3, account, 0, 500, 7)
	result, err := ReconcileMoveLines([]MoveLine{
		line(1, account, 10000, 0, 7),
		line(2, account, 0, 9500, 7),
	}, ReconcileOptions{PartialStartID: 20, FullID: 30, Writeoff: &writeoff})
	if err != nil {
		t.Fatal(err)
	}
	if !result.FullyReconciled || result.Full == nil || len(result.Partials) != 2 {
		t.Fatalf("writeoff result = %+v", result)
	}
	if result.Writeoff == nil || !result.Writeoff.Reconciled || result.Writeoff.FullReconcileID != 30 {
		t.Fatalf("writeoff line = %+v", result.Writeoff)
	}
	if result.Partials[0].Amount != 9500 || result.Partials[1].Amount != 500 {
		t.Fatalf("writeoff partials = %+v", result.Partials)
	}
}

func TestReconcileMoveLinesExtendsExistingPartialGroupToFull(t *testing.T) {
	account := receivableAccount()
	debit := line(1, account, 10000, 0, 7)
	debit.Residual = 6000
	debit.ResidualCurrency = 6000
	debit.MatchedCreditIDs = []int64{10}

	result, err := ReconcileMoveLines([]MoveLine{
		debit,
		line(2, account, 0, 6000, 7),
	}, ReconcileOptions{PartialStartID: 11, FullID: 99})
	if err != nil {
		t.Fatal(err)
	}
	if !result.FullyReconciled || result.Full == nil {
		t.Fatalf("result = %+v", result)
	}
	if !reflect.DeepEqual(result.Full.PartialReconcileIDs, []int64{10, 11}) {
		t.Fatalf("partial ids = %+v", result.Full.PartialReconcileIDs)
	}
	if result.Lines[0].FullReconcileID != 99 || !reflect.DeepEqual(result.Lines[0].MatchedCreditIDs, []int64{10, 11}) {
		t.Fatalf("extended line = %+v", result.Lines[0])
	}
}

func TestReconcileMoveLinesRequiresResidualCurrencyZeroForFull(t *testing.T) {
	account := receivableAccount()
	debit := line(1, account, 10000, 0, 7)
	debit.ResidualCurrency = 10001

	result, err := ReconcileMoveLines([]MoveLine{
		debit,
		line(2, account, 0, 10000, 7),
	}, ReconcileOptions{PartialStartID: 10, FullID: 99})
	if err != nil {
		t.Fatal(err)
	}
	if result.FullyReconciled || result.Full != nil {
		t.Fatalf("unexpected full reconcile = %+v", result)
	}
	if result.Lines[0].Residual != 0 || result.Lines[0].ResidualCurrency != 1 || result.Lines[0].Reconciled {
		t.Fatalf("residual currency line = %+v", result.Lines[0])
	}
}

func TestReconcileMoveLinesRejectsCompanyAndAccountMismatch(t *testing.T) {
	account := receivableAccount()
	otherCompany := line(2, account, 0, 1000, 7)
	otherCompany.CompanyID = 2
	_, err := ReconcileMoveLines([]MoveLine{line(1, account, 1000, 0, 7), otherCompany}, ReconcileOptions{})
	if !errors.Is(err, ErrReconcileCompany) {
		t.Fatalf("company error = %v", err)
	}

	otherAccount := line(3, incomeAccount(), 0, 1000, 7)
	_, err = ReconcileMoveLines([]MoveLine{line(1, account, 1000, 0, 7), otherAccount}, ReconcileOptions{})
	if !errors.Is(err, ErrReconcileAccount) {
		t.Fatalf("account error = %v", err)
	}

	otherCurrency := line(4, account, 0, 1000, 7)
	otherCurrency.Currency = "USD"
	_, err = ReconcileMoveLines([]MoveLine{line(1, account, 1000, 0, 7), otherCurrency}, ReconcileOptions{})
	if !errors.Is(err, ErrCurrencyMismatch) {
		t.Fatalf("currency error = %v", err)
	}

	_, err = ReconcileMoveLines([]MoveLine{line(1, account, 1000, 0, 7)}, ReconcileOptions{})
	if !errors.Is(err, ErrInvalidReconcileLines) {
		t.Fatalf("invalid lines error = %v", err)
	}
}

func TestReconcileMoveLinesRejectsAlreadyReconciledAndNonReconcilable(t *testing.T) {
	account := receivableAccount()
	reconciled := line(2, account, 0, 1000, 7)
	reconciled.Reconciled = true
	_, err := ReconcileMoveLines([]MoveLine{line(1, account, 1000, 0, 7), reconciled}, ReconcileOptions{})
	if !errors.Is(err, ErrReconcileAlreadyDone) {
		t.Fatalf("already reconciled error = %v", err)
	}

	nonReconcilable := Account{ID: 999, Code: "999", Name: "Expense", Kind: AccountExpense, CompanyID: 1, Currency: "BHD"}
	_, err = ReconcileMoveLines([]MoveLine{
		line(3, nonReconcilable, 1000, 0, 7),
		line(4, nonReconcilable, 0, 1000, 7),
	}, ReconcileOptions{})
	if !errors.Is(err, ErrReconcileReconcilable) {
		t.Fatalf("non-reconcilable error = %v", err)
	}
}

func TestLineResidualAndPaymentState(t *testing.T) {
	account := receivableAccount()
	debit := line(1, account, 10000, 0, 7)
	debit.Residual = 0
	debit.ResidualCurrency = 0
	partials := []PartialReconcile{
		{ID: 10, DebitLineID: 1, CreditLineID: 2, Amount: 4000, DebitAmountCurrency: 4000, CreditAmountCurrency: 4000},
	}
	debit = ComputeLineResidual(debit, partials)
	if debit.Residual != 6000 || debit.ResidualCurrency != 6000 || debit.Reconciled || !reflect.DeepEqual(debit.MatchedCreditIDs, []int64{10}) {
		t.Fatalf("line residual = %+v", debit)
	}

	move := balancedMove()
	move.State = MovePosted
	move.MoveType = "out_invoice"
	move.Lines[0] = debit
	ApplyPaymentState(&move, []PaymentMatch{{SourceLineID: debit.ID, AccountKind: AccountReceivable, HasPayment: true, AllPaymentsMatched: true}})
	if move.AmountResidual != 6000 || move.AmountResidualSigned != 6000 || move.PaymentState != PaymentPartial || move.StatusInPayment != "partial" {
		t.Fatalf("partial move = %+v", move)
	}

	move.Lines[0].Residual = 0
	move.Lines[0].ResidualCurrency = 0
	ApplyPaymentState(&move, []PaymentMatch{{SourceLineID: debit.ID, AccountKind: AccountReceivable, HasPayment: true, AllPaymentsMatched: false}})
	if move.PaymentState != PaymentInPayment || move.StatusInPayment != "in_payment" {
		t.Fatalf("in payment state = %+v", move)
	}
	ApplyPaymentState(&move, []PaymentMatch{{SourceLineID: debit.ID, AccountKind: AccountReceivable, HasPayment: true, AllPaymentsMatched: true}})
	if move.PaymentState != PaymentPaid || move.StatusInPayment != "paid" {
		t.Fatalf("paid state = %+v", move)
	}

	move.PaymentState = ""
	ApplyPaymentState(&move, []PaymentMatch{{SourceLineID: debit.ID, AccountKind: AccountReceivable, CounterpartMoveType: "out_refund", AllPaymentsMatched: true}})
	if move.PaymentState != PaymentReversed || move.StatusInPayment != "reversed" {
		t.Fatalf("reversed state = %+v", move)
	}
}

func TestStatusInPaymentDraftSentAndBlocked(t *testing.T) {
	move := balancedMove()
	move.MoveType = "out_invoice"
	move.State = MoveDraft
	move.PaymentState = PaymentPartial
	if got := ComputeStatusInPayment(move); got != "partial" {
		t.Fatalf("draft partial status = %s", got)
	}
	move.State = MovePosted
	move.PaymentState = PaymentNotPaid
	move.IsMoveSent = true
	if got := ComputeStatusInPayment(move); got != "sent" {
		t.Fatalf("sent status = %s", got)
	}
	move.PaymentState = PaymentBlocked
	if got := ComputePaymentState(move, nil); got != PaymentBlocked {
		t.Fatalf("blocked payment state = %s", got)
	}
}

func TestPaymentCountUsesDistinctPaymentLines(t *testing.T) {
	move := balancedMove()
	move.Lines[0].PaymentID = 1
	move.Lines[1].PaymentID = 1
	RefreshMoveAmounts(&move)
	if move.PaymentCount != 1 {
		t.Fatalf("payment count = %d", move.PaymentCount)
	}
	ApplyMovePaymentLinks(&move, []int64{2, 2, 3}, []int64{4, 4, 5})
	if !reflect.DeepEqual(move.MatchedPaymentIDs, []int64{2, 3}) || !reflect.DeepEqual(move.ReconciledPaymentIDs, []int64{4, 5}) || move.PaymentCount != 2 {
		t.Fatalf("payment links = %+v", move)
	}
	RefreshMoveAmounts(&move)
	if move.PaymentCount != 2 {
		t.Fatalf("payment count from reconciled ids = %d", move.PaymentCount)
	}
}

func TestPaymentActionsAndReconciliationStatus(t *testing.T) {
	payment := Payment{Amount: 10000, OutstandingAccount: Account{ID: 2000, Kind: AccountCash}}
	ActionPostPayment(&payment)
	if payment.State != "paid" {
		t.Fatalf("cash payment state = %s", payment.State)
	}
	ActionDraftPayment(&payment)
	if payment.State != "draft" {
		t.Fatalf("draft state = %s", payment.State)
	}
	ActionRejectPayment(&payment)
	if payment.State != "rejected" {
		t.Fatalf("reject state = %s", payment.State)
	}
	ActionCancelPayment(&payment)
	if payment.State != "canceled" {
		t.Fatalf("cancel state = %s", payment.State)
	}
	MarkPaymentSent(&payment, true)
	if !payment.IsSent {
		t.Fatal("payment not marked sent")
	}

	payment = Payment{
		Amount:             10000,
		State:              "draft",
		MoveID:             42,
		OutstandingAccount: Account{ID: 2100, Kind: AccountReceivable, Reconcile: true},
		LiquidityLines: []MoveLine{
			{Account: Account{ID: 2100, Reconcile: true}, Residual: 10000},
		},
		CounterpartLines: []MoveLine{
			{Account: receivableAccount(), Residual: 0},
		},
	}
	ActionPostPayment(&payment)
	if payment.State != "in_process" {
		t.Fatalf("posted payment state = %s", payment.State)
	}
	RefreshPayment(&payment, nil)
	if payment.State != "in_process" || payment.IsMatched || !payment.IsReconciled {
		t.Fatalf("payment reconcile status = %+v", payment)
	}

	payment.LiquidityLines[0].Residual = 0
	RefreshPayment(&payment, nil)
	if payment.State != "paid" || !payment.IsMatched || !payment.IsReconciled {
		t.Fatalf("paid payment status = %+v", payment)
	}
}

func TestPaymentRefreshUsesDefaultAccountAndPaidInvoices(t *testing.T) {
	payment := Payment{
		Amount:                10000,
		State:                 "in_process",
		MoveID:                42,
		OutstandingAccount:    Account{ID: 2100, Kind: AccountReceivable, Reconcile: true},
		JournalDefaultAccount: Account{ID: 999},
		LiquidityLines: []MoveLine{
			{Account: Account{ID: 999, Reconcile: true}, Residual: 10000},
		},
		CounterpartLines: []MoveLine{{Account: receivableAccount(), Residual: 10000}},
	}
	RefreshPayment(&payment, []Move{{PaymentState: PaymentPaid}})
	if payment.State != "paid" || !payment.IsMatched || payment.IsReconciled {
		t.Fatalf("default account/payment status = %+v", payment)
	}

	matched, reconciled := PaymentReconciliationStatus(Payment{State: "paid"})
	if !matched || reconciled {
		t.Fatalf("no outstanding account matched=%t reconciled=%t", matched, reconciled)
	}
	matched, reconciled = PaymentReconciliationStatus(Payment{Amount: 0, MoveID: 1, OutstandingAccount: receivableAccount()})
	if !matched || !reconciled {
		t.Fatalf("zero payment matched=%t reconciled=%t", matched, reconciled)
	}
}

func balancedMove() Move {
	return Move{
		ID:        1,
		Date:      date(2026, 7, 15),
		State:     MoveDraft,
		CompanyID: 1,
		Currency:  "BHD",
		Journal:   Journal{ID: 1, Code: "SAJ", CompanyID: 1, Currency: "BHD"},
		Lines: []MoveLine{
			line(1, receivableAccount(), 10000, 0, 7),
			line(2, incomeAccount(), 0, 10000, 0),
		},
	}
}

func lockedPostedMove(mutator func(*Move)) Move {
	move := balancedMove()
	move.State = MovePosted
	move.PostedBefore = true
	if mutator != nil {
		mutator(&move)
	}
	return move
}

func withLines(move Move, lines []MoveLine) Move {
	move.Lines = lines
	return move
}

func line(id int64, account Account, debit int64, credit int64, partnerID int64) MoveLine {
	balance := debit - credit
	return MoveLine{
		ID:             id,
		Account:        account,
		PartnerID:      partnerID,
		CompanyID:      1,
		Currency:       "BHD",
		Debit:          debit,
		Credit:         credit,
		AmountCurrency: balance,
		Residual:       balance,
	}
}

func receivableAccount() Account {
	return Account{ID: 1100, Code: "1100", Name: "Receivable", Kind: AccountReceivable, CompanyID: 1, Currency: "BHD", Reconcile: true}
}

func incomeAccount() Account {
	return Account{ID: 4000, Code: "4000", Name: "Income", Kind: AccountIncome, CompanyID: 1, Currency: "BHD"}
}

func date(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}
