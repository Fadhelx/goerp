package accounting

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type AccountKind string

const (
	AccountReceivable AccountKind = "asset_receivable"
	AccountPayable    AccountKind = "liability_payable"
	AccountBank       AccountKind = "asset_cash"
	AccountCash       AccountKind = "asset_cash"
	AccountCreditCard AccountKind = "liability_credit_card"
	AccountIncome     AccountKind = "income"
	AccountExpense    AccountKind = "expense"
	AccountLiability  AccountKind = "liability_current"
	AccountEquity     AccountKind = "equity"
	AccountEarnings   AccountKind = "equity_unaffected"
	AccountOffBalance AccountKind = "off_balance"
)

type MoveState string

const (
	MoveDraft  MoveState = "draft"
	MovePosted MoveState = "posted"
	MoveCancel MoveState = "cancel"
)

type PaymentState string

const (
	PaymentNotPaid         PaymentState = "not_paid"
	PaymentInPayment       PaymentState = "in_payment"
	PaymentPaid            PaymentState = "paid"
	PaymentPartial         PaymentState = "partial"
	PaymentReversed        PaymentState = "reversed"
	PaymentBlocked         PaymentState = "blocked"
	PaymentInvoicingLegacy PaymentState = "invoicing_legacy"
)

type JournalType string

const (
	JournalSale     JournalType = "sale"
	JournalPurchase JournalType = "purchase"
	JournalBank     JournalType = "bank"
	JournalCash     JournalType = "cash"
	JournalGeneral  JournalType = "general"
)

var (
	ErrMoveUnbalanced         = errors.New("account move is not balanced")
	ErrPostedMoveImmutable    = errors.New("posted move immutable field changed")
	ErrPostedLineTaxImmutable = errors.New("posted journal item taxes cannot be modified")
	ErrSequenceMissing        = errors.New("posted move requires sequence")
	ErrCompanyMismatch        = errors.New("accounting company mismatch")
	ErrCurrencyMismatch       = errors.New("accounting currency mismatch")
	ErrPartnerRequired        = errors.New("receivable/payable line requires partner")
	ErrFiscalLockDate         = errors.New("move violates fiscal lock date")
	ErrTaxLockDate            = errors.New("move violates tax lock date")
	ErrSaleLockDate           = errors.New("move violates sale lock date")
	ErrPurchaseLockDate       = errors.New("move violates purchase lock date")
	ErrHardLockDate           = errors.New("move violates hard lock date")
	ErrMoveNotDraft           = errors.New("account move must be draft")
	ErrMovePostRequiresAction = errors.New("account move must be posted through action_post")
	ErrMoveHasNoLines         = errors.New("account move requires lines")
	ErrPostedMoveHashLocked   = errors.New("posted hashed move cannot be reset")
	ErrPostedMoveProtected    = errors.New("posted move is protected by restrictive audit trail")
	ErrSequenceGap            = errors.New("secure sequence gap detected")
	ErrCancelRequestRequired  = errors.New("account move requires cancellation request")
	ErrCancelRequestNotFound  = errors.New("account move does not need cancellation request")
	ErrInvalidReconcileLines  = errors.New("reconciliation requires one debit and one credit line")
	ErrReconcileCompany       = errors.New("reconciliation company mismatch")
	ErrReconcileAccount       = errors.New("reconciliation account mismatch")
	ErrReconcileReconcilable  = errors.New("reconciliation requires reconcilable account")
	ErrReconcileAlreadyDone   = errors.New("reconciliation line is already reconciled")
	ErrReconcileAmount        = errors.New("invalid reconciliation amount")
	ErrReconcileLinesMissing  = errors.New("reconciliation requires move lines")
	ErrReversalNoMoves        = errors.New("reversal requires posted moves")
	ErrReversalCompany        = errors.New("reversal moves must belong to one company")
	ErrReversalJournalType    = errors.New("reversal journal must have same type as reversed entry")
	ErrPaymentRegisterNoMoves = errors.New("payment register requires posted invoices or receipts")
	ErrPaymentRegisterCompany = errors.New("payment register moves must belong to one company")
	ErrFiscalPositionMapping  = errors.New("duplicate fiscal position account mapping")
	ErrAccountMapped          = errors.New("account is used by a fiscal position mapping")
	ErrAccountGroupPrefix     = errors.New("invalid account group prefix range")
	ErrAccountGroupOverlap    = errors.New("overlapping account group prefix range")
	ErrAccountRootSearch      = errors.New("unsupported account root search operator")
	ErrAccountCodeDuplicate   = errors.New("account code already exists for company hierarchy")
	ErrLockExceptionFields    = errors.New("lock exception requires exactly one lock date field")
	ErrLockExceptionAccess    = errors.New("account manager rights required to revoke lock exception")
)

type Account struct {
	ID        int64
	Code      string
	Name      string
	Kind      AccountKind
	CompanyID int64
	Currency  string
	Reconcile bool
}

type Journal struct {
	ID                    int64
	Name                  string
	Code                  string
	Type                  JournalType
	CompanyID             int64
	Currency              string
	DefaultAccountID      int64
	RestrictModeHashTable bool
	NextSequence          int64
}

type Move struct {
	ID                   int64
	Name                 string
	Ref                  string
	Date                 time.Time
	InvoiceDate          time.Time
	InvoiceDateDue       time.Time
	State                MoveState
	MoveType             string
	Journal              Journal
	CompanyID            int64
	CompanyCurrencyID    int64
	Currency             string
	CurrencyID           int64
	PartnerID            int64
	CommercialPartnerID  int64
	CountryID            int64
	InvoiceUserID        int64
	FiscalPositionID     int64
	InvoiceCurrencyRate  float64
	Lines                []MoveLine
	PostedBefore         bool
	InalterableHash      string
	SequencePrefix       string
	SequenceNumber       int64
	MadeSequenceGap      bool
	SecureSequenceNumber int64
	AmountTotal          int64
	AmountResidual       int64
	AmountResidualSigned int64
	AutoPost             string
	PaymentState         PaymentState
	StatusInPayment      string
	IsMoveSent           bool
	OriginPaymentID      int64
	StatementLineID      int64
	MatchedPaymentIDs    []int64
	ReconciledPaymentIDs []int64
	PaymentCount         int
	NeedCancelRequest    bool
	ReversedEntryID      int64
}

type MoveLine struct {
	ID                    int64
	Account               Account
	PartnerID             int64
	CompanyID             int64
	Currency              string
	CurrencyID            int64
	Name                  string
	Debit                 int64
	Credit                int64
	Quantity              float64
	PriceUnit             int64
	PriceSubtotal         int64
	PriceTotal            int64
	Discount              float64
	DisplayType           string
	DateMaturity          time.Time
	ProductID             int64
	ProductUOMID          int64
	ProductCategoryID     int64
	ProductUOMFactor      float64
	TemplateUOMFactor     float64
	StandardPrice         int64
	AmountCurrency        int64
	Residual              int64
	ResidualCurrency      int64
	Reconciled            bool
	PaymentID             int64
	FullReconcileID       int64
	MatchedDebitIDs       []int64
	MatchedCreditIDs      []int64
	TaxID                 int64
	TaxRepartitionLineID  int64
	ExcludeFromInvoiceTab bool
}

type Payment struct {
	ID                    int64
	Name                  string
	Amount                int64
	State                 string
	PaymentType           string
	PartnerType           string
	PartnerID             int64
	CompanyID             int64
	Currency              string
	JournalID             int64
	MoveID                int64
	OutstandingAccount    Account
	JournalDefaultAccount Account
	LiquidityLines        []MoveLine
	CounterpartLines      []MoveLine
	WriteoffLines         []MoveLine
	InvoiceIDs            []int64
	ReconciledInvoiceIDs  []int64
	ReconciledBillIDs     []int64
	IsReconciled          bool
	IsMatched             bool
	IsSent                bool
	NeedCancelRequest     bool
}

type PaymentMatch struct {
	SourceLineID        int64
	AccountKind         AccountKind
	CounterpartMoveType string
	HasPayment          bool
	HasStatementLine    bool
	AllPaymentsMatched  bool
}

type PaymentRegister struct {
	ID                            int64
	LineIDs                       []int64
	PaymentDate                   time.Time
	Amount                        int64
	Communication                 string
	GroupPayment                  bool
	Currency                      string
	Journal                       Journal
	AvailableJournalIDs           []int64
	PartnerBankID                 int64
	CompanyID                     int64
	PartnerID                     int64
	PaymentMethodLineID           int64
	AvailablePaymentMethodLineIDs []int64
	PaymentType                   string
	PartnerType                   string
	SourceAmount                  int64
	SourceAmountCurrency          int64
	SourceCurrency                string
	CanEditWizard                 bool
	CanGroupPayments              bool
	PaymentDifference             int64
	PaymentDifferenceHandling     string
	WriteoffAccountID             int64
	WriteoffLabel                 string
	TotalPaymentsAmount           int
}

type MoveReversal struct {
	ID                  int64
	MoveIDs             []int64
	NewMoveIDs          []int64
	Date                time.Time
	Reason              string
	Journal             Journal
	CompanyID           int64
	AvailableJournalIDs []int64
	CountryCode         string
	Residual            int64
	Currency            string
	MoveType            string
}

type ActionResult struct {
	Name     string
	Type     string
	ResModel string
	ViewMode string
	ResID    int64
	Domain   []int64
	Context  map[string]any
}

type LockPolicy struct {
	FiscalLockDate        time.Time
	TaxLockDate           time.Time
	SaleLockDate          time.Time
	PurchaseLockDate      time.Time
	HardLockDate          time.Time
	RestrictiveAuditTrail bool
}

type LockDateKind string

const (
	LockFiscalYear LockDateKind = "fiscalyear_lock_date"
	LockTax        LockDateKind = "tax_lock_date"
	LockSale       LockDateKind = "sale_lock_date"
	LockPurchase   LockDateKind = "purchase_lock_date"
	LockHard       LockDateKind = "hard_lock_date"
)

type LockDateViolation struct {
	Date time.Time
	Kind LockDateKind
}

type SequenceReset string

const (
	SequenceResetMonth          SequenceReset = "month"
	SequenceResetYear           SequenceReset = "year"
	SequenceResetYearRangeMonth SequenceReset = "year_range_month"
)

type AccountingDateOptions struct {
	Today         time.Time
	HighestName   string
	SequenceReset SequenceReset
}

type Sequence struct {
	Prefix string
	Next   int64
}

func (s *Sequence) NextName() (string, error) {
	name, _, _, err := s.NextParts()
	return name, err
}

func (s *Sequence) NextParts() (string, string, int64, error) {
	name, prefix, number, err := s.PeekParts()
	if err != nil {
		return "", "", 0, err
	}
	s.Prefix = prefix
	s.Next = number + 1
	return name, prefix, number, nil
}

func (s *Sequence) PeekParts() (string, string, int64, error) {
	if s == nil {
		return "", "", 0, ErrSequenceMissing
	}
	number := s.Next
	if number <= 0 {
		number = 1
	}
	prefix := s.Prefix
	if prefix == "" {
		prefix = "MISC/"
	}
	name := fmt.Sprintf("%s%04d", prefix, number)
	return name, prefix, number, nil
}

func (m Move) Balance() int64 {
	var balance int64
	for _, line := range m.Lines {
		balance += line.Balance()
	}
	return balance
}

func (l MoveLine) Balance() int64 {
	return l.Debit - l.Credit
}

func (l MoveLine) IsReceivablePayable() bool {
	return l.Account.Kind == AccountReceivable || l.Account.Kind == AccountPayable
}

type AccountRoot struct {
	ID       string
	Name     string
	ParentID string
}

type AccountClassification struct {
	PlaceholderCode string
	RootID          string
	GroupID         int64
}

func ClassifyAccount(code string, groups []AccountGroup, companyID int64) AccountClassification {
	placeholderCode := strings.TrimSpace(code)
	out := AccountClassification{PlaceholderCode: placeholderCode}
	if root, ok := AccountRootFromCode(placeholderCode); ok {
		out.RootID = root.ID
	}
	if group, ok := AccountGroupForCode(groups, placeholderCode, companyID); ok {
		out.GroupID = group.ID
	}
	return out
}

func AccountRootFromCode(code string) (AccountRoot, bool) {
	code = strings.TrimSpace(code)
	if len(code) == 0 {
		return AccountRoot{}, false
	}
	rootID := code
	if len(rootID) > 2 {
		rootID = rootID[:2]
	}
	parentID := ""
	if len(rootID) > 1 {
		parentID = rootID[:len(rootID)-1]
	}
	return AccountRoot{ID: rootID, Name: rootID, ParentID: parentID}, true
}

func AccountRootMatches(rootID string, operator string, values ...string) (bool, error) {
	rootID = strings.TrimSpace(rootID)
	switch operator {
	case "in":
		for _, value := range values {
			if rootID == strings.TrimSpace(value) {
				return true, nil
			}
		}
		return false, nil
	case "child_of":
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value != "" && strings.HasPrefix(rootID, value) {
				return true, nil
			}
		}
		return false, nil
	case "any":
		return rootID != "", nil
	default:
		return false, ErrAccountRootSearch
	}
}

type AccountGroup struct {
	ID              int64
	ParentID        int64
	ParentPath      string
	Name            string
	CodePrefixStart string
	CodePrefixEnd   string
	CompanyID       int64
}

func (g AccountGroup) Validate() error {
	start := strings.TrimSpace(g.CodePrefixStart)
	end := strings.TrimSpace(g.CodePrefixEnd)
	if start == "" {
		return ErrAccountGroupPrefix
	}
	if end == "" {
		return nil
	}
	if len(start) != len(end) || start > end {
		return ErrAccountGroupPrefix
	}
	return nil
}

func (g AccountGroup) ContainsCode(code string) bool {
	start := strings.TrimSpace(g.CodePrefixStart)
	end := strings.TrimSpace(g.CodePrefixEnd)
	code = strings.TrimSpace(code)
	if start == "" || code == "" || len(code) < len(start) {
		return false
	}
	if end == "" {
		end = start
	}
	if len(start) != len(end) {
		return false
	}
	prefix := code[:len(start)]
	return prefix >= start && prefix <= end
}

func ValidateAccountGroups(groups []AccountGroup) error {
	for i, group := range groups {
		if err := group.Validate(); err != nil {
			return err
		}
		for j := 0; j < i; j++ {
			if accountGroupsOverlap(group, groups[j]) {
				return ErrAccountGroupOverlap
			}
		}
	}
	return nil
}

func AccountGroupForCode(groups []AccountGroup, code string, companyID int64) (AccountGroup, bool) {
	var best AccountGroup
	bestScore := -1
	found := false
	for _, group := range groups {
		if group.CompanyID != 0 && companyID != 0 && group.CompanyID != companyID {
			continue
		}
		if !group.ContainsCode(code) {
			continue
		}
		score := len(strings.TrimSpace(group.CodePrefixStart)) * 10
		if group.CompanyID == companyID && companyID != 0 {
			score++
		}
		if !found || score > bestScore || (score == bestScore && group.ID < best.ID) {
			best = group
			bestScore = score
			found = true
		}
	}
	return best, found
}

func SyncAccountGroupParents(groups []AccountGroup) []AccountGroup {
	out := append([]AccountGroup(nil), groups...)
	for i := range out {
		var parent AccountGroup
		found := false
		for _, candidate := range out {
			if candidate.ID == out[i].ID || candidate.CompanyID != out[i].CompanyID {
				continue
			}
			if accountGroupPrefixLen(candidate) >= accountGroupPrefixLen(out[i]) {
				continue
			}
			if !candidate.ContainsCode(out[i].CodePrefixStart) || !candidate.ContainsCode(accountGroupEnd(out[i])) {
				continue
			}
			if !found || accountGroupPrefixLen(candidate) > accountGroupPrefixLen(parent) || (accountGroupPrefixLen(candidate) == accountGroupPrefixLen(parent) && candidate.ID < parent.ID) {
				parent = candidate
				found = true
			}
		}
		if found {
			out[i].ParentID = parent.ID
		} else {
			out[i].ParentID = 0
		}
	}
	return out
}

func accountGroupsOverlap(a AccountGroup, b AccountGroup) bool {
	if a.CompanyID != b.CompanyID {
		return false
	}
	aStart := strings.TrimSpace(a.CodePrefixStart)
	bStart := strings.TrimSpace(b.CodePrefixStart)
	if len(aStart) == 0 || len(aStart) != len(bStart) {
		return false
	}
	aEnd := accountGroupEnd(a)
	bEnd := accountGroupEnd(b)
	return aStart <= bEnd && bStart <= aEnd
}

func accountGroupEnd(group AccountGroup) string {
	end := strings.TrimSpace(group.CodePrefixEnd)
	if end == "" {
		return strings.TrimSpace(group.CodePrefixStart)
	}
	return end
}

func accountGroupPrefixLen(group AccountGroup) int {
	return len(strings.TrimSpace(group.CodePrefixStart))
}

type AccountFormValues struct {
	Code        string
	Name        string
	AccountType AccountKind
	TaxIDs      []int64
}

func AccountTypeOnchange(values AccountFormValues) AccountFormValues {
	out := values
	if out.AccountType == AccountOffBalance {
		out.TaxIDs = nil
	}
	return out
}

func SplitAccountCodeName(value string) (string, string, bool) {
	parts := strings.Fields(strings.TrimSpace(value))
	if len(parts) < 2 || !isAccountCodeToken(parts[0]) {
		return "", strings.TrimSpace(value), false
	}
	return parts[0], strings.Join(parts[1:], " "), true
}

func isAccountCodeToken(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

type AccountCodeRecord struct {
	ID        int64
	Code      string
	CompanyID int64
}

type CompanyRelation struct {
	ID       int64
	ParentID int64
}

func ValidateAccountCodeUnique(existing []AccountCodeRecord, candidate AccountCodeRecord, companies []CompanyRelation) error {
	code := strings.TrimSpace(candidate.Code)
	if code == "" {
		return nil
	}
	for _, account := range existing {
		if account.ID == candidate.ID || strings.TrimSpace(account.Code) != code {
			continue
		}
		if account.CompanyID == 0 || candidate.CompanyID == 0 || companiesRelated(account.CompanyID, candidate.CompanyID, companies) {
			return ErrAccountCodeDuplicate
		}
	}
	return nil
}

func companiesRelated(left int64, right int64, companies []CompanyRelation) bool {
	if left == right {
		return true
	}
	parents := map[int64]int64{}
	for _, company := range companies {
		parents[company.ID] = company.ParentID
	}
	return companyIsAncestor(left, right, parents) || companyIsAncestor(right, left, parents)
}

func companyIsAncestor(ancestor int64, child int64, parents map[int64]int64) bool {
	for current := child; current != 0; current = parents[current] {
		if current == ancestor {
			return true
		}
		next, ok := parents[current]
		if !ok || next == current {
			return false
		}
	}
	return false
}

type LockExceptionState string

const (
	LockExceptionActive  LockExceptionState = "active"
	LockExceptionRevoked LockExceptionState = "revoked"
	LockExceptionExpired LockExceptionState = "expired"
)

type LockException struct {
	ID                 int64
	Active             bool
	State              LockExceptionState
	CompanyID          int64
	UserID             int64
	Reason             string
	EndDatetime        time.Time
	LockDateField      LockDateKind
	LockDate           time.Time
	CompanyLockDate    time.Time
	FiscalYearLockDate time.Time
	TaxLockDate        time.Time
	SaleLockDate       time.Time
	PurchaseLockDate   time.Time
}

type CompanyLockPolicy struct {
	CompanyID int64
	Locks     LockPolicy
}

func NewLockException(input LockException, companyLocks LockPolicy) (LockException, error) {
	out := input
	out.Active = true
	if out.LockDateField != "" {
		if !isSoftLockDateKind(out.LockDateField) {
			return LockException{}, ErrLockExceptionFields
		}
		if out.CompanyLockDate.IsZero() {
			out.CompanyLockDate = companyLockDate(out.LockDateField, companyLocks)
		}
		out.State = out.StateAt(time.Now().UTC())
		return out, nil
	}
	field, lockDate, err := singleLockExceptionDate(input)
	if err != nil {
		return LockException{}, err
	}
	out.LockDateField = field
	out.LockDate = lockDate
	if out.CompanyLockDate.IsZero() {
		out.CompanyLockDate = companyLockDate(field, companyLocks)
	}
	out.State = out.StateAt(time.Now().UTC())
	return out, nil
}

func isSoftLockDateKind(field LockDateKind) bool {
	switch field {
	case LockFiscalYear, LockTax, LockSale, LockPurchase:
		return true
	default:
		return false
	}
}

func EffectiveLockPolicy(companyChain []CompanyLockPolicy, userID int64, exceptions []LockException, now time.Time) LockPolicy {
	var out LockPolicy
	for _, company := range companyChain {
		locks := ApplyLockExceptions(company.Locks, company.CompanyID, userID, exceptions, now)
		out.FiscalLockDate = maxDate(out.FiscalLockDate, locks.FiscalLockDate)
		out.TaxLockDate = maxDate(out.TaxLockDate, locks.TaxLockDate)
		out.SaleLockDate = maxDate(out.SaleLockDate, locks.SaleLockDate)
		out.PurchaseLockDate = maxDate(out.PurchaseLockDate, locks.PurchaseLockDate)
		out.HardLockDate = maxDate(out.HardLockDate, locks.HardLockDate)
		out.RestrictiveAuditTrail = out.RestrictiveAuditTrail || locks.RestrictiveAuditTrail
	}
	return out
}

func ApplyLockExceptions(locks LockPolicy, companyID int64, userID int64, exceptions []LockException, now time.Time) LockPolicy {
	out := locks
	out.FiscalLockDate = effectiveSoftLockDate(companyID, userID, LockFiscalYear, locks.FiscalLockDate, exceptions, now)
	out.TaxLockDate = effectiveSoftLockDate(companyID, userID, LockTax, locks.TaxLockDate, exceptions, now)
	out.SaleLockDate = effectiveSoftLockDate(companyID, userID, LockSale, locks.SaleLockDate, exceptions, now)
	out.PurchaseLockDate = effectiveSoftLockDate(companyID, userID, LockPurchase, locks.PurchaseLockDate, exceptions, now)
	return out
}

func effectiveSoftLockDate(companyID int64, userID int64, field LockDateKind, lockDate time.Time, exceptions []LockException, now time.Time) time.Time {
	if lockDate.IsZero() {
		return time.Time{}
	}
	effective := dateOnly(lockDate)
	for _, exception := range exceptions {
		if exception.CompanyID != companyID || exception.LockDateField != field || exception.StateAt(now) != LockExceptionActive {
			continue
		}
		if exception.UserID != 0 && exception.UserID != userID {
			continue
		}
		if !exception.CompanyLockDate.IsZero() && !dateOnly(exception.CompanyLockDate).Equal(dateOnly(lockDate)) {
			continue
		}
		if exception.LockDate.IsZero() {
			return time.Time{}
		}
		candidate := dateOnly(exception.LockDate)
		if candidate.Before(effective) {
			effective = candidate
		}
	}
	return effective
}

func (e LockException) StateAt(now time.Time) LockExceptionState {
	if !e.Active {
		return LockExceptionRevoked
	}
	if !e.EndDatetime.IsZero() && e.EndDatetime.Before(now) {
		return LockExceptionExpired
	}
	return LockExceptionActive
}

func RevokeLockException(e LockException, now time.Time, canManage bool) (LockException, error) {
	if !canManage {
		return LockException{}, ErrLockExceptionAccess
	}
	e.Active = false
	e.EndDatetime = now
	e.State = LockExceptionRevoked
	return e, nil
}

func ActiveLockExceptions(exceptions []LockException, companyID int64, lockDateField LockDateKind, changedLockDate time.Time, companyLockDate time.Time, now time.Time) []LockException {
	if changedLockDate.IsZero() || companyLockDate.IsZero() || !changedLockDate.Before(companyLockDate) {
		return nil
	}
	out := make([]LockException, 0, len(exceptions))
	for _, exception := range exceptions {
		if exception.CompanyID != companyID || exception.LockDateField != lockDateField || exception.StateAt(now) != LockExceptionActive {
			continue
		}
		if exception.LockDate.IsZero() || !changedLockDate.After(exception.LockDate) {
			out = append(out, exception)
		}
	}
	return out
}

func singleLockExceptionDate(e LockException) (LockDateKind, time.Time, error) {
	candidates := []struct {
		kind LockDateKind
		date time.Time
	}{
		{LockFiscalYear, e.FiscalYearLockDate},
		{LockTax, e.TaxLockDate},
		{LockSale, e.SaleLockDate},
		{LockPurchase, e.PurchaseLockDate},
	}
	var field LockDateKind
	var lockDate time.Time
	for _, candidate := range candidates {
		if candidate.date.IsZero() {
			continue
		}
		if field != "" {
			return "", time.Time{}, ErrLockExceptionFields
		}
		field = candidate.kind
		lockDate = candidate.date
	}
	if field == "" {
		return "", time.Time{}, ErrLockExceptionFields
	}
	return field, lockDate, nil
}

func companyLockDate(field LockDateKind, locks LockPolicy) time.Time {
	switch field {
	case LockFiscalYear:
		return locks.FiscalLockDate
	case LockTax:
		return locks.TaxLockDate
	case LockSale:
		return locks.SaleLockDate
	case LockPurchase:
		return locks.PurchaseLockDate
	default:
		return time.Time{}
	}
}

type PostOptions struct {
	ForceHash          bool
	PreviousHash       string
	LastSequencePrefix string
	LastSequenceNumber int64
	AccountingDate     AccountingDateOptions
}

func PostMove(move *Move, sequence *Sequence, locks LockPolicy) error {
	return PostMoveWithOptions(move, sequence, locks, PostOptions{})
}

func PostMoveWithOptions(move *Move, sequence *Sequence, locks LockPolicy, opts PostOptions) error {
	if move == nil {
		return fmt.Errorf("move is nil")
	}
	if move.State != "" && move.State != MoveDraft {
		return ErrMoveNotDraft
	}
	applyAccountingDate(move, locks, opts.AccountingDate)
	if err := validateMove(*move, locks); err != nil {
		return err
	}
	name, prefix, number, err := sequence.PeekParts()
	if err != nil {
		return err
	}
	hashMove := shouldHashMove(*move, opts)
	if hashMove && opts.LastSequencePrefix == prefix && opts.LastSequenceNumber > 0 && number != opts.LastSequenceNumber+1 {
		return ErrSequenceGap
	}
	sequence.Prefix = prefix
	sequence.Next = number + 1
	move.Name = name
	move.SequencePrefix = prefix
	move.SequenceNumber = number
	move.State = MovePosted
	move.PostedBefore = true
	RefreshMoveAmounts(move)
	move.PaymentState = ComputePaymentState(*move, nil)
	move.StatusInPayment = ComputeStatusInPayment(*move)
	if hashMove {
		move.SecureSequenceNumber = number
		move.InalterableHash = moveHash(*move, opts.PreviousHash)
	}
	return nil
}

func validateMove(move Move, locks LockPolicy) error {
	if err := validateLockDates(move, locks); err != nil {
		return err
	}
	if len(move.Lines) == 0 {
		return ErrMoveHasNoLines
	}
	if move.Balance() != 0 {
		return ErrMoveUnbalanced
	}
	for _, line := range move.Lines {
		if line.CompanyID != 0 && move.CompanyID != 0 && line.CompanyID != move.CompanyID {
			return ErrCompanyMismatch
		}
		if line.Account.CompanyID != 0 && move.CompanyID != 0 && line.Account.CompanyID != move.CompanyID {
			return ErrCompanyMismatch
		}
		if line.Currency != "" && move.Currency != "" && line.Currency != move.Currency {
			return ErrCurrencyMismatch
		}
		if line.Account.Currency != "" && move.Currency != "" && line.Account.Currency != move.Currency {
			return ErrCurrencyMismatch
		}
		if line.IsReceivablePayable() && line.PartnerID == 0 && move.PartnerID == 0 {
			return ErrPartnerRequired
		}
		if line.TaxID != 0 && !locks.TaxLockDate.IsZero() && !move.Date.After(locks.TaxLockDate) {
			return ErrTaxLockDate
		}
	}
	return nil
}

func validateLockDates(move Move, locks LockPolicy) error {
	for _, violation := range ViolatedLockDates(move, locks) {
		switch violation.Kind {
		case LockHard:
			return ErrHardLockDate
		case LockFiscalYear:
			return ErrFiscalLockDate
		case LockSale:
			return ErrSaleLockDate
		case LockPurchase:
			return ErrPurchaseLockDate
		case LockTax:
			return ErrTaxLockDate
		}
	}
	return nil
}

func ValidateLockDates(move Move, locks LockPolicy) error {
	return validateLockDates(move, locks)
}

func ViolatedLockDates(move Move, locks LockPolicy) []LockDateViolation {
	if move.Date.IsZero() {
		return nil
	}
	var violations []LockDateViolation
	addViolation := func(lockDate time.Time, kind LockDateKind) {
		if !lockDate.IsZero() && !move.Date.After(lockDate) {
			violations = append(violations, LockDateViolation{Date: dateOnly(lockDate), Kind: kind})
		}
	}
	addViolation(locks.FiscalLockDate, LockFiscalYear)
	if isSaleMove(move) {
		addViolation(locks.SaleLockDate, LockSale)
	}
	if isPurchaseMove(move) {
		addViolation(locks.PurchaseLockDate, LockPurchase)
	}
	if moveAffectsTaxReport(move) {
		addViolation(locks.TaxLockDate, LockTax)
	}
	addViolation(locks.HardLockDate, LockHard)
	sort.Slice(violations, func(i, j int) bool {
		if violations[i].Date.Equal(violations[j].Date) {
			return violations[i].Kind < violations[j].Kind
		}
		return violations[i].Date.Before(violations[j].Date)
	})
	return violations
}

func AccountingDate(move Move, locks LockPolicy, opts AccountingDateOptions) time.Time {
	return accountingDate(move, ViolatedLockDates(move, locks), opts)
}

func applyAccountingDate(move *Move, locks LockPolicy, opts AccountingDateOptions) {
	violations := ViolatedLockDates(*move, locks)
	if len(violations) == 0 {
		return
	}
	move.Date = accountingDate(*move, violations, opts)
}

func accountingDate(move Move, violations []LockDateViolation, opts AccountingDateOptions) time.Time {
	invoiceDate := dateOnly(moveAccountingDateSource(move))
	if len(violations) == 0 || invoiceDate.IsZero() {
		return invoiceDate
	}
	latestLockDate := violations[len(violations)-1].Date
	openDate := latestLockDate.AddDate(0, 0, 1)
	invoiceDate = openDate
	today := dateOnly(opts.Today)
	if today.IsZero() {
		today = dateOnly(time.Now().UTC())
	}
	reset := opts.SequenceReset
	if reset == "" {
		reset = SequenceResetMonth
	}
	var accountingDate time.Time
	if isSaleMove(move) {
		switch reset {
		case SequenceResetYear:
			accountingDate = minDate(today, endOfYear(invoiceDate))
		default:
			accountingDate = minDate(today, endOfMonth(invoiceDate))
		}
	} else {
		switch reset {
		case SequenceResetYear:
			if today.Year() > invoiceDate.Year() {
				accountingDate = endOfYear(invoiceDate)
			} else {
				accountingDate = maxDate(invoiceDate, today)
			}
		default:
			if today.Year() > invoiceDate.Year() || (today.Year() == invoiceDate.Year() && today.Month() > invoiceDate.Month()) {
				accountingDate = endOfMonth(invoiceDate)
			} else {
				accountingDate = maxDate(invoiceDate, today)
			}
		}
	}
	if accountingDate.Before(openDate) {
		return openDate
	}
	return accountingDate
}

func UpdatePostedFields(move Move, updates map[string]any, allowedFields map[string]bool) error {
	if move.State != MovePosted {
		return nil
	}
	for field := range updates {
		if protectedMoveFields()[field] {
			return fmt.Errorf("%w: %s", ErrPostedMoveImmutable, field)
		}
		if allowedFields[field] {
			continue
		}
		if allowedFields == nil {
			continue
		}
		return fmt.Errorf("%w: %s", ErrPostedMoveImmutable, field)
	}
	return nil
}

func ButtonDraft(move *Move, locks LockPolicy) error {
	if move == nil {
		return fmt.Errorf("move is nil")
	}
	if move.NeedCancelRequest {
		return ErrCancelRequestRequired
	}
	if move.State == MoveCancel {
		if err := validateLockDates(*move, locks); err != nil {
			return err
		}
		move.State = MoveDraft
		return nil
	}
	if move.State != MovePosted {
		return nil
	}
	if move.InalterableHash != "" {
		return ErrPostedMoveHashLocked
	}
	if locks.RestrictiveAuditTrail {
		return ErrPostedMoveProtected
	}
	if err := validateLockDates(*move, locks); err != nil {
		return err
	}
	move.State = MoveDraft
	return nil
}

func ButtonCancel(move *Move, locks LockPolicy) error {
	if move == nil {
		return fmt.Errorf("move is nil")
	}
	if move.State == MoveCancel {
		return nil
	}
	if move.NeedCancelRequest {
		return ErrCancelRequestRequired
	}
	if move.State == MovePosted && move.InalterableHash != "" {
		return ErrPostedMoveHashLocked
	}
	if locks.RestrictiveAuditTrail && (move.PostedBefore || move.State == MovePosted) {
		return ErrPostedMoveProtected
	}
	if err := validateLockDates(*move, locks); err != nil {
		return err
	}
	move.AutoPost = "no"
	move.State = MoveCancel
	return nil
}

func ButtonRequestCancel(move Move) error {
	if !move.NeedCancelRequest {
		return ErrCancelRequestNotFound
	}
	return nil
}

func ShowResetToDraftButton(move Move) bool {
	return !move.Journal.RestrictModeHashTable &&
		move.InalterableHash == "" &&
		(move.State == MoveCancel || (move.State == MovePosted && !move.NeedCancelRequest))
}

func CanUnlinkOrReverse(move Move, locks LockPolicy) error {
	return CanUnlink(move, locks)
}

func CanUnlink(move Move, locks LockPolicy) error {
	if move.State == MovePosted && move.InalterableHash != "" {
		return ErrPostedMoveProtected
	}
	if locks.RestrictiveAuditTrail && (move.PostedBefore || move.State == MovePosted) {
		return ErrPostedMoveProtected
	}
	return validateLockDates(move, locks)
}

func CanReverse(move Move, locks LockPolicy) error {
	return nil
}

func ReverseMove(move Move, reversalID int64, reversalDate time.Time) Move {
	reversal := move
	reversal.ID = reversalID
	reversal.Name = ""
	reversal.Ref = move.Name
	reversal.Date = dateOnly(reversalDate)
	if reversal.Date.IsZero() {
		reversal.Date = dateOnly(time.Now().UTC())
	}
	reversal.InvoiceDate = reversal.Date
	reversal.State = MoveDraft
	reversal.PostedBefore = false
	reversal.InalterableHash = ""
	reversal.SequencePrefix = ""
	reversal.SequenceNumber = 0
	reversal.MadeSequenceGap = false
	reversal.SecureSequenceNumber = 0
	reversal.AutoPost = "no"
	reversal.NeedCancelRequest = false
	reversal.ReversedEntryID = move.ID
	reversal.MoveType = ReversalMoveType(move.MoveType)
	reversal.Lines = make([]MoveLine, len(move.Lines))
	for i, line := range move.Lines {
		line.ID = 0
		line.Debit, line.Credit = line.Credit, line.Debit
		line.AmountCurrency = -line.AmountCurrency
		line.Residual = -line.Residual
		line.Reconciled = false
		reversal.Lines[i] = line
	}
	reversal.AmountTotal = totalDebits(reversal.Lines)
	reversal.AmountResidual = residualTotal(reversal.Lines)
	reversal.AmountResidualSigned = residualSignedTotal(reversal.Lines)
	reversal.PaymentState = ComputePaymentState(reversal, nil)
	reversal.StatusInPayment = ComputeStatusInPayment(reversal)
	return reversal
}

func NewMoveReversal(moves []Move, journal Journal, reversalDate time.Time, reason string) (MoveReversal, error) {
	if len(moves) == 0 {
		return MoveReversal{}, ErrReversalNoMoves
	}
	reversal := MoveReversal{
		Date:   dateOnly(reversalDate),
		Reason: reason,
	}
	if reversal.Date.IsZero() {
		reversal.Date = dateOnly(time.Now().UTC())
	}
	companyID := moves[0].CompanyID
	journalTypes := map[JournalType]bool{}
	currency := moves[0].Currency
	sameCurrency := true
	for _, move := range moves {
		if move.State != MovePosted {
			return MoveReversal{}, ErrReversalNoMoves
		}
		if move.CompanyID != companyID {
			return MoveReversal{}, ErrReversalCompany
		}
		reversal.MoveIDs = append(reversal.MoveIDs, move.ID)
		journalTypes[move.Journal.Type] = true
		reversal.AvailableJournalIDs = appendUniqueID(reversal.AvailableJournalIDs, move.Journal.ID)
		if move.Currency != currency {
			sameCurrency = false
		}
	}
	reversal.CompanyID = companyID
	if journal.ID == 0 {
		journal = moves[0].Journal
	}
	if !journalTypes[journal.Type] {
		return MoveReversal{}, ErrReversalJournalType
	}
	if journal.CompanyID != 0 && journal.CompanyID != companyID {
		return MoveReversal{}, ErrReversalCompany
	}
	reversal.Journal = journal
	if len(moves) == 1 {
		reversal.Residual = moves[0].AmountResidual
		reversal.MoveType = moves[0].MoveType
	}
	if sameCurrency {
		reversal.Currency = currency
	}
	if len(moves) > 1 && anyInvoiceMove(moves) {
		reversal.MoveType = "some_invoice"
	}
	return reversal, nil
}

func ReverseMoves(reversal *MoveReversal, moves []Move, firstReversalID int64, modify bool) ([]Move, error) {
	if reversal == nil {
		return nil, ErrReversalNoMoves
	}
	if len(moves) == 0 {
		return nil, ErrReversalNoMoves
	}
	reversed := make([]Move, 0, len(moves))
	for i, move := range moves {
		if move.State != MovePosted {
			return nil, ErrReversalNoMoves
		}
		newMove := ReverseMove(move, firstReversalID+int64(i), reversal.Date)
		newMove.Ref = reversalRef(move.Name, reversal.Reason)
		newMove.Journal = reversal.Journal
		newMove.CompanyID = reversal.CompanyID
		if isInvoiceMove(move, true) {
			newMove.InvoiceDate = reversal.Date
		} else {
			newMove.InvoiceDate = time.Time{}
		}
		if reversal.Date.After(dateOnly(time.Now().UTC())) {
			newMove.AutoPost = "at_date"
		}
		if modify {
			replacement := ReplacementMoveFromReverse(move, firstReversalID+int64(len(moves)+i), reversal.Date)
			reversed = append(reversed, replacement)
			continue
		}
		reversed = append(reversed, newMove)
	}
	reversal.NewMoveIDs = reversal.NewMoveIDs[:0]
	for _, move := range reversed {
		reversal.NewMoveIDs = append(reversal.NewMoveIDs, move.ID)
	}
	return reversed, nil
}

func ReplacementMoveFromReverse(origin Move, replacementID int64, replacementDate time.Time) Move {
	replacement := origin
	replacement.ID = replacementID
	replacement.Name = ""
	replacement.Ref = origin.Ref
	replacement.Date = dateOnly(replacementDate)
	replacement.InvoiceDate = replacement.Date
	replacement.State = MoveDraft
	replacement.PostedBefore = false
	replacement.InalterableHash = ""
	replacement.SequencePrefix = ""
	replacement.SequenceNumber = 0
	replacement.MadeSequenceGap = false
	replacement.SecureSequenceNumber = 0
	replacement.AutoPost = "no"
	replacement.NeedCancelRequest = false
	replacement.ReversedEntryID = 0
	replacement.Lines = businessLines(origin.Lines)
	RefreshMoveAmounts(&replacement)
	replacement.PaymentState = ComputePaymentState(replacement, nil)
	replacement.StatusInPayment = ComputeStatusInPayment(replacement)
	return replacement
}

func ReversalAction(moves []Move) ActionResult {
	action := ActionResult{
		Name:     "Reverse Moves",
		Type:     "ir.actions.act_window",
		ResModel: "account.move",
		Context:  map[string]any{},
	}
	if len(moves) == 1 {
		action.ViewMode = "form"
		action.ResID = moves[0].ID
		action.Context["default_move_type"] = moves[0].MoveType
		return action
	}
	action.ViewMode = "list,form"
	action.Domain = make([]int64, 0, len(moves))
	moveTypes := map[string]bool{}
	for _, move := range moves {
		action.Domain = append(action.Domain, move.ID)
		moveTypes[move.MoveType] = true
	}
	if len(moveTypes) == 1 && len(moves) > 0 {
		action.Context["default_move_type"] = moves[0].MoveType
	}
	return action
}

func ReversalMoveType(moveType string) string {
	switch moveType {
	case "out_invoice":
		return "out_refund"
	case "out_refund":
		return "out_invoice"
	case "out_receipt":
		return "out_refund"
	case "in_invoice":
		return "in_refund"
	case "in_refund":
		return "in_invoice"
	case "in_receipt":
		return "in_refund"
	default:
		return "entry"
	}
}

func businessLines(lines []MoveLine) []MoveLine {
	out := make([]MoveLine, 0, len(lines))
	for _, line := range lines {
		if line.ExcludeFromInvoiceTab || line.TaxID != 0 || line.TaxRepartitionLineID != 0 {
			continue
		}
		line.ID = 0
		line.Reconciled = false
		line.FullReconcileID = 0
		line.MatchedDebitIDs = nil
		line.MatchedCreditIDs = nil
		out = append(out, line)
	}
	return out
}

func RefreshMoveAmounts(move *Move) {
	if move == nil {
		return
	}
	move.AmountTotal = totalDebits(move.Lines)
	move.AmountResidual = residualTotal(move.Lines)
	move.AmountResidualSigned = residualSignedTotal(move.Lines)
	move.PaymentCount = len(uniqueIDs(move.ReconciledPaymentIDs))
	if move.PaymentCount == 0 {
		move.PaymentCount = countPayments(move.Lines)
	}
}

func ComputePaymentState(move Move, matches []PaymentMatch) PaymentState {
	if move.PaymentState == PaymentInvoicingLegacy || move.PaymentState == PaymentBlocked {
		return move.PaymentState
	}
	if !paymentStateQualifies(move) {
		return PaymentNotPaid
	}
	reconciliation := receivablePayableMatches(matches)
	if move.AmountResidual == 0 {
		if hasPaymentOrStatement(reconciliation) {
			if allPaymentsMatched(reconciliation) {
				return PaymentPaid
			}
			return PaymentInPayment
		}
		if isReversePaidState(move, reconciliation) {
			return PaymentReversed
		}
		return PaymentPaid
	}
	if len(reconciliation) > 0 {
		return PaymentPartial
	}
	return PaymentNotPaid
}

func ComputeStatusInPayment(move Move) string {
	paymentState := move.PaymentState
	if paymentState == "" {
		paymentState = ComputePaymentState(move, nil)
	}
	if move.State == MovePosted {
		if paymentState == PaymentPartial || paymentState == PaymentInPayment || paymentState == PaymentPaid || paymentState == PaymentReversed {
			return string(paymentState)
		}
		if move.IsMoveSent {
			return "sent"
		}
	}
	if move.State == MoveDraft {
		if paymentState == PaymentPartial || paymentState == PaymentInPayment || paymentState == PaymentPaid {
			return string(paymentState)
		}
	}
	if move.State != "" {
		return string(move.State)
	}
	return string(MoveDraft)
}

func ApplyPaymentState(move *Move, matches []PaymentMatch) {
	if move == nil {
		return
	}
	RefreshMoveAmounts(move)
	move.PaymentState = ComputePaymentState(*move, matches)
	move.StatusInPayment = ComputeStatusInPayment(*move)
}

func ApplyMovePaymentLinks(move *Move, matchedPaymentIDs []int64, reconciledPaymentIDs []int64) {
	if move == nil {
		return
	}
	move.MatchedPaymentIDs = uniqueIDs(matchedPaymentIDs)
	move.ReconciledPaymentIDs = uniqueIDs(reconciledPaymentIDs)
	move.PaymentCount = len(move.ReconciledPaymentIDs)
}

func NewPaymentRegister(moves []Move, journal Journal, paymentDate time.Time, amount int64, communication string, groupPayment bool) (PaymentRegister, error) {
	if len(moves) == 0 {
		return PaymentRegister{}, ErrPaymentRegisterNoMoves
	}
	register := PaymentRegister{
		PaymentDate:               dateOnly(paymentDate),
		GroupPayment:              groupPayment,
		Journal:                   journal,
		CanEditWizard:             true,
		CanGroupPayments:          len(moves) > 1,
		PaymentDifferenceHandling: "open",
		WriteoffLabel:             "Write-Off",
		TotalPaymentsAmount:       1,
	}
	if register.PaymentDate.IsZero() {
		register.PaymentDate = dateOnly(time.Now().UTC())
	}
	companyID := moves[0].CompanyID
	currency := moves[0].Currency
	partnerID := paymentRegisterPartnerID(moves[0])
	paymentType, partnerType := paymentRegisterTypes(moves[0].MoveType)
	var sourceAmount int64
	var lineIDs []int64
	moveNames := make([]string, 0, len(moves))
	sameCurrency := true
	samePartner := true
	for _, move := range moves {
		if move.State != MovePosted || !isInvoiceMove(move, true) {
			return PaymentRegister{}, ErrPaymentRegisterNoMoves
		}
		if move.CompanyID != companyID {
			return PaymentRegister{}, ErrPaymentRegisterCompany
		}
		if move.Currency != currency {
			sameCurrency = false
		}
		if paymentRegisterPartnerID(move) != partnerID {
			samePartner = false
		}
		for _, line := range move.Lines {
			if line.IsReceivablePayable() {
				lineIDs = appendUniqueID(lineIDs, line.ID)
			}
		}
		if move.Name != "" {
			moveNames = append(moveNames, move.Name)
		}
		sourceAmount += abs(move.AmountResidual)
		nextPaymentType, nextPartnerType := paymentRegisterTypes(move.MoveType)
		if nextPaymentType != paymentType {
			paymentType = ""
		}
		if nextPartnerType != partnerType {
			partnerType = ""
		}
	}
	if journal.ID == 0 {
		journal = moves[0].Journal
		register.Journal = journal
	}
	if journal.ID != 0 {
		register.AvailableJournalIDs = appendUniqueID(register.AvailableJournalIDs, journal.ID)
	}
	if journal.CompanyID != 0 && journal.CompanyID != companyID {
		return PaymentRegister{}, ErrPaymentRegisterCompany
	}
	register.CompanyID = companyID
	register.LineIDs = lineIDs
	register.PaymentType = paymentType
	register.PartnerType = partnerType
	register.SourceAmount = sourceAmount
	register.SourceAmountCurrency = sourceAmount
	register.Amount = amount
	if register.Amount == 0 {
		register.Amount = sourceAmount
	}
	if sameCurrency {
		register.Currency = currency
		register.SourceCurrency = currency
	}
	if samePartner {
		register.PartnerID = partnerID
	}
	if communication != "" {
		register.Communication = communication
	} else {
		register.Communication = strings.Join(moveNames, ", ")
	}
	if !groupPayment {
		register.TotalPaymentsAmount = len(moves)
	}
	if register.TotalPaymentsAmount == 0 {
		register.TotalPaymentsAmount = 1
	}
	register.PaymentDifference = register.Amount - register.SourceAmount
	return register, nil
}

func CreateRegisteredPayments(register PaymentRegister, moves []Move, firstPaymentID int64) ([]Payment, []Move, error) {
	if len(moves) == 0 {
		return nil, nil, ErrPaymentRegisterNoMoves
	}
	if register.GroupPayment {
		payment := paymentFromRegister(register, moves, firstPaymentID, register.Amount)
		paidMoves := markMovesPaid(moves, payment.ID)
		return []Payment{payment}, paidMoves, nil
	}
	payments := make([]Payment, 0, len(moves))
	paidMoves := make([]Move, 0, len(moves))
	for i, move := range moves {
		paymentRegister := register
		paymentRegister.PaymentType, paymentRegister.PartnerType = paymentRegisterTypes(move.MoveType)
		paymentRegister.PartnerID = move.PartnerID
		paymentRegister.Communication = move.Name
		amount := abs(move.AmountResidual)
		payment := paymentFromRegister(paymentRegister, []Move{move}, firstPaymentID+int64(i), amount)
		payments = append(payments, payment)
		paidMoves = append(paidMoves, markMovesPaid([]Move{move}, payment.ID)...)
	}
	return payments, paidMoves, nil
}

func ComputeLineResidual(line MoveLine, partials []PartialReconcile) MoveLine {
	if !line.Account.Reconcile && line.Account.Kind != AccountCash && line.Account.Kind != AccountCreditCard {
		line.Residual = 0
		line.ResidualCurrency = 0
		line.Reconciled = false
		return line
	}
	residual := line.Balance()
	residualCurrency := line.AmountCurrency
	for _, partial := range partials {
		if partial.DebitLineID == line.ID {
			residual -= partial.Amount
			residualCurrency -= partial.DebitAmountCurrency
			line.MatchedCreditIDs = appendUniqueID(line.MatchedCreditIDs, partial.ID)
		}
		if partial.CreditLineID == line.ID {
			residual += partial.Amount
			residualCurrency += partial.CreditAmountCurrency
			line.MatchedDebitIDs = appendUniqueID(line.MatchedDebitIDs, partial.ID)
		}
		if partial.FullReconcileID != 0 && (partial.DebitLineID == line.ID || partial.CreditLineID == line.ID) {
			line.FullReconcileID = partial.FullReconcileID
		}
	}
	line.Residual = residual
	line.ResidualCurrency = residualCurrency
	line.Reconciled = residual == 0 && residualCurrency == 0
	return line
}

func ActionPostPayment(payment *Payment) {
	if payment == nil {
		return
	}
	if payment.OutstandingAccount.Kind == AccountCash {
		payment.State = "paid"
		return
	}
	if payment.State == "" || payment.State == "draft" || payment.State == "in_process" {
		payment.State = "in_process"
	}
}

func ActionValidatePayment(payment *Payment) {
	if payment != nil {
		payment.State = "paid"
	}
}

func ActionRejectPayment(payment *Payment) {
	if payment != nil {
		payment.State = "rejected"
	}
}

func ActionCancelPayment(payment *Payment) {
	if payment != nil {
		payment.State = "canceled"
	}
}

func ActionDraftPayment(payment *Payment) {
	if payment != nil {
		payment.State = "draft"
	}
}

func MarkPaymentSent(payment *Payment, sent bool) {
	if payment != nil {
		payment.IsSent = sent
	}
}

func RefreshPayment(payment *Payment, reconciledMoves []Move) {
	if payment == nil {
		return
	}
	if payment.State == "" {
		payment.State = "draft"
	}
	if payment.MoveID != 0 && (payment.State == "paid" || payment.State == "in_process") {
		if sumResidual(payment.LiquidityLines) == 0 || !anyReconcilable(payment.LiquidityLines) {
			payment.State = "paid"
		} else {
			payment.State = "in_process"
		}
	}
	if payment.State == "in_process" && len(reconciledMoves) > 0 && allMovesPaid(reconciledMoves) {
		payment.State = "paid"
	}
	payment.IsMatched, payment.IsReconciled = PaymentReconciliationStatus(*payment)
}

func PaymentReconciliationStatus(payment Payment) (matched bool, reconciled bool) {
	if payment.OutstandingAccount.ID == 0 {
		return payment.State == "paid", false
	}
	if payment.MoveID == 0 {
		return false, false
	}
	if payment.Amount == 0 {
		return true, true
	}
	if payment.JournalDefaultAccount.ID != 0 && lineAccountIn(payment.LiquidityLines, payment.JournalDefaultAccount.ID) {
		matched = true
	} else {
		matched = sumResidual(payment.LiquidityLines) == 0
	}
	reconciled = sumResidual(reconcilableLines(append(append([]MoveLine{}, payment.CounterpartLines...), payment.WriteoffLines...))) == 0
	return matched, reconciled
}

func totalDebits(lines []MoveLine) int64 {
	var total int64
	for _, line := range lines {
		total += line.Debit
	}
	return total
}

func residualTotal(lines []MoveLine) int64 {
	var total int64
	for _, line := range lines {
		if line.IsReceivablePayable() {
			total += abs(line.Residual)
		}
	}
	return total
}

func residualSignedTotal(lines []MoveLine) int64 {
	var total int64
	for _, line := range lines {
		if line.IsReceivablePayable() {
			total += line.Residual
		}
	}
	return total
}

func countPayments(lines []MoveLine) int {
	seen := map[int64]bool{}
	for _, line := range lines {
		if line.PaymentID != 0 {
			seen[line.PaymentID] = true
		}
	}
	return len(seen)
}

func uniqueIDs(ids []int64) []int64 {
	out := make([]int64, 0, len(ids))
	seen := map[int64]bool{}
	for _, id := range ids {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func paymentStateQualifies(move Move) bool {
	return isInvoiceMove(move, true) && (move.State == MovePosted || (move.State == MoveDraft && move.AmountTotal != 0))
}

func receivablePayableMatches(matches []PaymentMatch) []PaymentMatch {
	filtered := make([]PaymentMatch, 0, len(matches))
	for _, match := range matches {
		if match.AccountKind == AccountReceivable || match.AccountKind == AccountPayable {
			filtered = append(filtered, match)
		}
	}
	return filtered
}

func hasPaymentOrStatement(matches []PaymentMatch) bool {
	for _, match := range matches {
		if match.HasPayment || match.HasStatementLine {
			return true
		}
	}
	return false
}

func allPaymentsMatched(matches []PaymentMatch) bool {
	for _, match := range matches {
		if !match.AllPaymentsMatched {
			return false
		}
	}
	return true
}

func isReversePaidState(move Move, matches []PaymentMatch) bool {
	if len(matches) == 0 {
		return false
	}
	moveTypes := map[string]bool{}
	for _, match := range matches {
		if match.CounterpartMoveType != "" {
			moveTypes[match.CounterpartMoveType] = true
		}
	}
	return (move.MoveType == "in_invoice" || move.MoveType == "in_receipt") && onlyMoveTypes(moveTypes, "in_refund") ||
		(move.MoveType == "out_invoice" || move.MoveType == "out_receipt") && onlyMoveTypes(moveTypes, "out_refund") ||
		(move.MoveType == "entry" || move.MoveType == "out_refund" || move.MoveType == "in_refund") && onlyMoveTypes(moveTypes, "entry")
}

func onlyMoveTypes(moveTypes map[string]bool, required string) bool {
	if len(moveTypes) == 0 {
		return false
	}
	for moveType := range moveTypes {
		if moveType != required && moveType != "entry" {
			return false
		}
	}
	return moveTypes[required]
}

func sumResidual(lines []MoveLine) int64 {
	var total int64
	for _, line := range lines {
		total += line.Residual
	}
	return total
}

func anyReconcilable(lines []MoveLine) bool {
	for _, line := range lines {
		if line.Account.Reconcile {
			return true
		}
	}
	return false
}

func lineAccountIn(lines []MoveLine, accountID int64) bool {
	for _, line := range lines {
		if line.Account.ID == accountID {
			return true
		}
	}
	return false
}

func reconcilableLines(lines []MoveLine) []MoveLine {
	out := make([]MoveLine, 0, len(lines))
	for _, line := range lines {
		if line.Account.Reconcile {
			out = append(out, line)
		}
	}
	return out
}

func allMovesPaid(moves []Move) bool {
	for _, move := range moves {
		if move.PaymentState != PaymentPaid {
			return false
		}
	}
	return true
}

func paymentFromRegister(register PaymentRegister, moves []Move, paymentID int64, amount int64) Payment {
	invoiceIDs, billIDs := paymentMoveIDsByType(moves)
	payment := Payment{
		ID:                    paymentID,
		Name:                  register.Communication,
		Amount:                amount,
		State:                 "paid",
		PaymentType:           register.PaymentType,
		CompanyID:             register.CompanyID,
		Currency:              register.Currency,
		JournalID:             register.Journal.ID,
		JournalDefaultAccount: Account{ID: register.Journal.DefaultAccountID, Kind: AccountCash, Reconcile: true},
		PartnerType:           register.PartnerType,
		PartnerID:             register.PartnerID,
		InvoiceIDs:            append(append([]int64{}, invoiceIDs...), billIDs...),
		ReconciledInvoiceIDs:  invoiceIDs,
		ReconciledBillIDs:     billIDs,
		IsReconciled:          true,
		IsMatched:             true,
	}
	if payment.Name == "" {
		payment.Name = "Manual Payment"
	}
	return payment
}

func markMovesPaid(moves []Move, paymentID int64) []Move {
	out := make([]Move, 0, len(moves))
	for _, move := range moves {
		move.AmountResidual = 0
		move.AmountResidualSigned = 0
		move.PaymentState = PaymentPaid
		move.StatusInPayment = string(PaymentPaid)
		move.ReconciledPaymentIDs = appendUniqueID(move.ReconciledPaymentIDs, paymentID)
		move.MatchedPaymentIDs = appendUniqueID(move.MatchedPaymentIDs, paymentID)
		move.PaymentCount = len(move.ReconciledPaymentIDs)
		for i := range move.Lines {
			if move.Lines[i].IsReceivablePayable() {
				move.Lines[i].Residual = 0
				move.Lines[i].ResidualCurrency = 0
				move.Lines[i].Reconciled = true
				move.Lines[i].PaymentID = paymentID
			}
		}
		out = append(out, move)
	}
	return out
}

func paymentMoveIDsByType(moves []Move) ([]int64, []int64) {
	var invoices []int64
	var bills []int64
	for _, move := range moves {
		switch move.MoveType {
		case "in_invoice", "in_receipt", "out_refund":
			bills = appendUniqueID(bills, move.ID)
		default:
			invoices = appendUniqueID(invoices, move.ID)
		}
	}
	return invoices, bills
}

func paymentRegisterTypes(moveType string) (string, string) {
	switch moveType {
	case "in_invoice", "in_receipt", "out_refund":
		return "outbound", "supplier"
	default:
		return "inbound", "customer"
	}
}

func paymentRegisterPartnerID(move Move) int64 {
	if move.PartnerID != 0 {
		return move.PartnerID
	}
	for _, line := range move.Lines {
		if line.IsReceivablePayable() && line.PartnerID != 0 {
			return line.PartnerID
		}
	}
	return 0
}

func shouldHashMove(move Move, opts PostOptions) bool {
	return opts.ForceHash || move.Journal.RestrictModeHashTable
}

func moveHash(move Move, previousHash string) string {
	hash := sha256.New()
	fmt.Fprintf(hash, "%s|%d|%s|%s|%d|%s|%d|", previousHash, move.ID, move.Name, move.Date.Format("2006-01-02"), move.CompanyID, move.SequencePrefix, move.SequenceNumber)
	for _, line := range move.Lines {
		fmt.Fprintf(hash, "%d:%d:%d:%d|", line.Account.ID, line.Debit, line.Credit, line.PartnerID)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func protectedMoveFields() map[string]bool {
	return map[string]bool{
		"name":                   true,
		"date":                   true,
		"invoice_date":           true,
		"invoice_date_due":       true,
		"journal_id":             true,
		"company_id":             true,
		"currency_id":            true,
		"partner_id":             true,
		"line_ids":               true,
		"inalterable_hash":       true,
		"sequence_prefix":        true,
		"sequence_number":        true,
		"secure_sequence_number": true,
		"posted_before":          true,
		"made_sequence_gap":      true,
		"need_cancel_request":    true,
		"reversed_entry_id":      true,
	}
}

func isSaleMove(move Move) bool {
	return move.Journal.Type == JournalSale || move.MoveType == "out_invoice" || move.MoveType == "out_refund" || move.MoveType == "out_receipt"
}

func isPurchaseMove(move Move) bool {
	return move.Journal.Type == JournalPurchase || move.MoveType == "in_invoice" || move.MoveType == "in_refund" || move.MoveType == "in_receipt"
}

func isInvoiceMove(move Move, includeReceipts bool) bool {
	switch move.MoveType {
	case "out_invoice", "out_refund", "in_invoice", "in_refund":
		return true
	case "out_receipt", "in_receipt":
		return includeReceipts
	default:
		return false
	}
}

func anyInvoiceMove(moves []Move) bool {
	for _, move := range moves {
		if move.MoveType == "in_invoice" || move.MoveType == "out_invoice" {
			return true
		}
	}
	return false
}

func reversalRef(moveName string, reason string) string {
	if reason == "" {
		return fmt.Sprintf("Reversal of: %s", moveName)
	}
	return fmt.Sprintf("Reversal of: %s, %s", moveName, reason)
}

func moveAccountingDateSource(move Move) time.Time {
	if !move.InvoiceDate.IsZero() {
		return move.InvoiceDate
	}
	return move.Date
}

func moveAffectsTaxReport(move Move) bool {
	for _, line := range move.Lines {
		if line.TaxID != 0 || line.TaxRepartitionLineID != 0 {
			return true
		}
	}
	return false
}

func dateOnly(value time.Time) time.Time {
	if value.IsZero() {
		return time.Time{}
	}
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func endOfMonth(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month()+1, 0, 0, 0, 0, 0, time.UTC)
}

func endOfYear(value time.Time) time.Time {
	return time.Date(value.Year(), 12, 31, 0, 0, 0, 0, time.UTC)
}

func minDate(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func maxDate(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

type TaxAmountType string

const (
	TaxPercent TaxAmountType = "percent"
	TaxFixed   TaxAmountType = "fixed"
)

type Tax struct {
	ID              int64
	Name            string
	AmountType      TaxAmountType
	RateBasisPoints int64
	FixedAmount     int64
	CompanyID       int64
	Account         Account
	Sequence        int
}

type TaxLine struct {
	TaxID     int64
	Name      string
	Base      int64
	Amount    int64
	AccountID int64
	Sequence  int
}

func ComputeTaxLines(base int64, taxes []Tax) []TaxLine {
	ordered := append([]Tax(nil), taxes...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Sequence == ordered[j].Sequence {
			return ordered[i].ID < ordered[j].ID
		}
		return ordered[i].Sequence < ordered[j].Sequence
	})
	lines := make([]TaxLine, 0, len(ordered))
	for _, tax := range ordered {
		amount := taxAmount(base, tax)
		lines = append(lines, TaxLine{
			TaxID:     tax.ID,
			Name:      tax.Name,
			Base:      base,
			Amount:    amount,
			AccountID: tax.Account.ID,
			Sequence:  tax.Sequence,
		})
	}
	return lines
}

func taxAmount(base int64, tax Tax) int64 {
	switch tax.AmountType {
	case TaxFixed:
		return tax.FixedAmount
	default:
		return roundDiv(base*tax.RateBasisPoints, 10000)
	}
}

type FiscalPosition struct {
	ID             int64
	Name           string
	CompanyID      int64
	AccountLines   []FiscalPositionAccountLine
	AccountMapping map[int64]int64
	TaxMappings    map[int64][]int64
	TaxMapping     map[int64]int64
}

type FiscalPositionAccountLine struct {
	ID                   int64
	PositionID           int64
	CompanyID            int64
	SourceAccountID      int64
	DestinationAccountID int64
}

func (f FiscalPosition) MapAccount(accountID int64) int64 {
	return f.MapAccountForCompany(accountID, 0)
}

func (f FiscalPosition) MapAccountForCompany(accountID int64, companyID int64) int64 {
	mapped := int64(0)
	found := false
	for _, line := range fiscalPositionAccountLines(f.AccountLines) {
		if line.SourceAccountID != accountID {
			continue
		}
		if companyID != 0 && line.CompanyID != 0 && line.CompanyID != companyID {
			continue
		}
		mapped = line.DestinationAccountID
		found = true
	}
	if found {
		return mapped
	}
	if mapped, ok := f.AccountMapping[accountID]; ok {
		return mapped
	}
	return accountID
}

func (f FiscalPosition) MapTaxes(taxIDs []int64) []int64 {
	out := make([]int64, 0, len(taxIDs))
	for _, taxID := range taxIDs {
		if mapped, ok := f.TaxMappings[taxID]; ok {
			for _, mappedID := range mapped {
				if mappedID != 0 {
					out = append(out, mappedID)
				}
			}
			continue
		}
		if mapped, ok := f.TaxMapping[taxID]; ok {
			if mapped == 0 {
				continue
			}
			out = append(out, mapped)
			continue
		}
		out = append(out, taxID)
	}
	return out
}

type ProductFiscalAccounts struct {
	IncomeAccountID   int64
	ExpenseAccountID  int64
	CustomerTaxIDs    []int64
	SupplierTaxIDs    []int64
	StandardAccountID int64
}

type InvoiceLinePreparation struct {
	MoveType           string
	CompanyID          int64
	CurrentAccount     Account
	PaymentTermAccount Account
	Product            ProductFiscalAccounts
	AccountTaxIDs      []int64
	FiscalPosition     FiscalPosition
}

type PreparedInvoiceLine struct {
	Account Account
	TaxIDs  []int64
}

func PrepareInvoiceLine(input InvoiceLinePreparation) PreparedInvoiceLine {
	account := input.CurrentAccount
	if input.PaymentTermAccount.ID != 0 {
		account = input.PaymentTermAccount
	} else if productAccountID := productAccountForMoveType(input.MoveType, input.Product); productAccountID != 0 {
		account.ID = productAccountID
	}
	if account.ID != 0 {
		account.ID = input.FiscalPosition.MapAccountForCompany(account.ID, input.CompanyID)
	}
	taxes := productTaxesForMoveType(input.MoveType, input.Product)
	if len(taxes) == 0 {
		taxes = append([]int64(nil), input.AccountTaxIDs...)
	}
	taxes = input.FiscalPosition.MapTaxes(taxes)
	return PreparedInvoiceLine{Account: account, TaxIDs: taxes}
}

func productAccountForMoveType(moveType string, product ProductFiscalAccounts) int64 {
	switch moveType {
	case "in_invoice", "in_refund", "in_receipt":
		if product.ExpenseAccountID != 0 {
			return product.ExpenseAccountID
		}
	default:
		if product.IncomeAccountID != 0 {
			return product.IncomeAccountID
		}
	}
	return product.StandardAccountID
}

func productTaxesForMoveType(moveType string, product ProductFiscalAccounts) []int64 {
	switch moveType {
	case "in_invoice", "in_refund", "in_receipt":
		return append([]int64(nil), product.SupplierTaxIDs...)
	default:
		return append([]int64(nil), product.CustomerTaxIDs...)
	}
}

func ApplyFiscalPositionToLines(lines []MoveLine, fiscalPosition FiscalPosition, companyID int64) []MoveLine {
	out := append([]MoveLine(nil), lines...)
	for i := range out {
		mappedID := fiscalPosition.MapAccountForCompany(out[i].Account.ID, companyID)
		out[i].Account.ID = mappedID
	}
	return out
}

func ValidateFiscalPositionAccountLines(lines []FiscalPositionAccountLine) error {
	seen := map[[3]int64]bool{}
	for _, line := range lines {
		key := [3]int64{line.PositionID, line.SourceAccountID, line.DestinationAccountID}
		if seen[key] {
			return ErrFiscalPositionMapping
		}
		seen[key] = true
	}
	return nil
}

func CanChangeFiscalPositionMappedAccount(accountID int64, lines []FiscalPositionAccountLine) error {
	for _, line := range lines {
		if line.SourceAccountID == accountID || line.DestinationAccountID == accountID {
			return ErrAccountMapped
		}
	}
	return nil
}

func fiscalPositionAccountLines(lines []FiscalPositionAccountLine) []FiscalPositionAccountLine {
	out := append([]FiscalPositionAccountLine(nil), lines...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ID == 0 || out[j].ID == 0 {
			return false
		}
		return out[i].ID < out[j].ID
	})
	return out
}

type InvoiceReportRow struct {
	MoveID                int64
	JournalID             int64
	CompanyID             int64
	CompanyCurrencyID     int64
	PartnerID             int64
	CommercialPartnerID   int64
	CountryID             int64
	InvoiceUserID         int64
	MoveType              string
	State                 MoveState
	PaymentState          PaymentState
	FiscalPositionID      int64
	InvoiceDate           time.Time
	Quantity              float64
	ProductID             int64
	ProductUOMID          int64
	ProductCategoryID     int64
	InvoiceDateDue        time.Time
	AccountID             int64
	PriceSubtotalCurrency int64
	PriceSubtotal         int64
	PriceTotal            int64
	PriceTotalCurrency    int64
	PriceAverage          float64
	PriceMargin           int64
	InventoryValue        int64
	CurrencyID            int64
}

type InvoiceReportTotals struct {
	Quantity       float64
	Subtotal       int64
	PriceAverage   float64
	Margin         int64
	InventoryValue int64
}

func BuildInvoiceReportRows(moves []Move) []InvoiceReportRow {
	rows := []InvoiceReportRow{}
	for _, move := range moves {
		if !isInvoiceMove(move, true) {
			continue
		}
		for _, line := range move.Lines {
			if line.Account.ID == 0 || line.DisplayType != "product" {
				continue
			}
			rows = append(rows, invoiceReportRow(move, line))
		}
	}
	return rows
}

func AggregateInvoiceReportRows(rows []InvoiceReportRow) InvoiceReportTotals {
	var totals InvoiceReportTotals
	for _, row := range rows {
		totals.Quantity += row.Quantity
		totals.Subtotal += row.PriceSubtotal
		totals.Margin += row.PriceMargin
		totals.InventoryValue += row.InventoryValue
	}
	if totals.Quantity != 0 {
		totals.PriceAverage = float64(totals.Subtotal) / totals.Quantity
	}
	return totals
}

func invoiceReportRow(move Move, line MoveLine) InvoiceReportRow {
	sign := invoiceReportSign(move.MoveType)
	quantity := float64(sign) * invoiceReportQuantity(line.Quantity, line.ProductUOMFactor, line.TemplateUOMFactor)
	rate := move.InvoiceCurrencyRate
	if rate == 0 {
		rate = 1
	}
	subtotal := int64(-float64(line.Balance()) * rate)
	total := int64(float64(sign*line.PriceTotal) / rate)
	margin := int64(0)
	inventoryValue := int64(0)
	if invoiceReportHasMargin(move.MoveType) {
		inventoryValue = int64(float64(line.StandardPrice) * quantity)
		margin = sign*line.PriceSubtotal - inventoryValue
	}
	priceAverage := float64(0)
	if quantity != 0 {
		priceAverage = float64(subtotal) / quantity
	}
	partnerID := move.CommercialPartnerID
	if partnerID == 0 {
		partnerID = move.PartnerID
	}
	currencyID := line.CurrencyID
	if currencyID == 0 {
		currencyID = move.CurrencyID
	}
	return InvoiceReportRow{
		MoveID:                move.ID,
		JournalID:             move.Journal.ID,
		CompanyID:             move.CompanyID,
		CompanyCurrencyID:     move.CompanyCurrencyID,
		PartnerID:             move.PartnerID,
		CommercialPartnerID:   partnerID,
		CountryID:             move.CountryID,
		InvoiceUserID:         move.InvoiceUserID,
		MoveType:              move.MoveType,
		State:                 move.State,
		PaymentState:          move.PaymentState,
		FiscalPositionID:      move.FiscalPositionID,
		InvoiceDate:           move.InvoiceDate,
		Quantity:              quantity,
		ProductID:             line.ProductID,
		ProductUOMID:          line.ProductUOMID,
		ProductCategoryID:     line.ProductCategoryID,
		InvoiceDateDue:        move.InvoiceDateDue,
		AccountID:             line.Account.ID,
		PriceSubtotalCurrency: sign * line.PriceSubtotal,
		PriceSubtotal:         subtotal,
		PriceTotal:            total,
		PriceTotalCurrency:    sign * line.PriceTotal,
		PriceAverage:          priceAverage,
		PriceMargin:           margin,
		InventoryValue:        inventoryValue,
		CurrencyID:            currencyID,
	}
}

func invoiceReportQuantity(quantity float64, lineFactor float64, templateFactor float64) float64 {
	if lineFactor == 0 {
		lineFactor = 1
	}
	if templateFactor == 0 {
		templateFactor = 1
	}
	denominator := lineFactor / templateFactor
	if denominator == 0 {
		return 0
	}
	return quantity / denominator
}

func invoiceReportSign(moveType string) int64 {
	switch moveType {
	case "in_invoice", "out_refund", "in_receipt":
		return -1
	default:
		return 1
	}
}

func invoiceReportHasMargin(moveType string) bool {
	return moveType == "out_invoice" || moveType == "out_refund" || moveType == "out_receipt"
}

type PartialReconcile struct {
	ID                   int64
	DebitLineID          int64
	CreditLineID         int64
	Amount               int64
	DebitAmountCurrency  int64
	CreditAmountCurrency int64
	FullReconcileID      int64
	CompanyID            int64
}

type FullReconcile struct {
	ID                  int64
	Name                string
	PartialReconcileIDs []int64
	ReconciledLineIDs   []int64
}

type ReconcileResult struct {
	Debit            MoveLine
	Credit           MoveLine
	Partial          PartialReconcile
	Full             *FullReconcile
	DebitResidual    int64
	CreditResidual   int64
	FullyReconciled  bool
	PartialMatched   bool
	ReconcileBalance int64
}

type ReconcileOptions struct {
	PartialStartID int64
	FullID         int64
	Writeoff       *MoveLine
}

type ReconcilePlanResult struct {
	Lines            []MoveLine
	Writeoff         *MoveLine
	Partials         []PartialReconcile
	Full             *FullReconcile
	FullyReconciled  bool
	DebitResidual    int64
	CreditResidual   int64
	ReconcileBalance int64
}

func ReconcileLines(debit MoveLine, credit MoveLine, amount int64, partialID int64, fullID int64) (ReconcileResult, error) {
	if debit.Balance() <= 0 || credit.Balance() >= 0 {
		return ReconcileResult{}, ErrInvalidReconcileLines
	}
	if debit.CompanyID != 0 && credit.CompanyID != 0 && debit.CompanyID != credit.CompanyID {
		return ReconcileResult{}, ErrReconcileCompany
	}
	if debit.Account.ID != credit.Account.ID {
		return ReconcileResult{}, ErrReconcileAccount
	}
	debitResidual := positiveResidual(debit)
	creditResidual := positiveResidual(credit)
	maxAmount := min(debitResidual, creditResidual)
	if amount <= 0 {
		amount = maxAmount
	}
	if amount <= 0 || amount > maxAmount {
		return ReconcileResult{}, ErrReconcileAmount
	}

	debitResidual -= amount
	creditResidual -= amount
	debitCurrencyResidual := currencyOpenBalance(debit) - amount
	creditCurrencyResidual := currencyOpenBalance(credit) + amount
	debit.Residual = debitResidual
	debit.ResidualCurrency = debitCurrencyResidual
	credit.Residual = -creditResidual
	credit.ResidualCurrency = creditCurrencyResidual
	debit.Reconciled = debitResidual == 0 && debitCurrencyResidual == 0
	credit.Reconciled = creditResidual == 0 && creditCurrencyResidual == 0
	debit.MatchedCreditIDs = appendUniqueID(debit.MatchedCreditIDs, partialID)
	credit.MatchedDebitIDs = appendUniqueID(credit.MatchedDebitIDs, partialID)

	result := ReconcileResult{
		Debit:          debit,
		Credit:         credit,
		DebitResidual:  debitResidual,
		CreditResidual: creditResidual,
		Partial: PartialReconcile{
			ID:                   partialID,
			DebitLineID:          debit.ID,
			CreditLineID:         credit.ID,
			Amount:               amount,
			DebitAmountCurrency:  amount,
			CreditAmountCurrency: amount,
			CompanyID:            debit.CompanyID,
		},
		PartialMatched:   amount < maxAmount,
		ReconcileBalance: debitResidual - creditResidual,
	}
	if debit.Reconciled && credit.Reconciled {
		partialIDs := appendUniqueID(existingPartialIDs([]MoveLine{debit, credit}), partialID)
		result.FullyReconciled = true
		result.Full = &FullReconcile{
			ID:                  fullID,
			Name:                fmt.Sprintf("FULL/%04d", fullID),
			PartialReconcileIDs: partialIDs,
			ReconciledLineIDs:   reconciledLineIDs([]MoveLine{debit, credit}),
		}
		result.Partial.FullReconcileID = fullID
		result.Debit.FullReconcileID = fullID
		result.Credit.FullReconcileID = fullID
	}
	return result, nil
}

func ReconcileMoveLines(lines []MoveLine, options ReconcileOptions) (ReconcilePlanResult, error) {
	allLines := append([]MoveLine(nil), lines...)
	writeoffIndex := -1
	if options.Writeoff != nil {
		allLines = append(allLines, *options.Writeoff)
		writeoffIndex = len(allLines) - 1
	}
	if len(allLines) == 0 {
		return ReconcilePlanResult{}, ErrReconcileLinesMissing
	}
	if err := validateReconcileSet(allLines); err != nil {
		return ReconcilePlanResult{}, err
	}

	debits, credits := splitReconcileLines(allLines)
	if len(debits) == 0 || len(credits) == 0 {
		return ReconcilePlanResult{}, ErrInvalidReconcileLines
	}

	nextPartialID := options.PartialStartID
	if nextPartialID <= 0 {
		nextPartialID = 1
	}
	var partials []PartialReconcile
	for _, debitIdx := range debits {
		for lineOpenAmount(allLines[debitIdx]) > 0 {
			creditIdx := selectCreditForDebit(allLines, credits, debitIdx)
			if creditIdx < 0 {
				break
			}
			result, err := ReconcileLines(allLines[debitIdx], allLines[creditIdx], 0, nextPartialID, 0)
			if err != nil {
				return ReconcilePlanResult{}, err
			}
			allLines[debitIdx] = result.Debit
			allLines[creditIdx] = result.Credit
			partials = append(partials, result.Partial)
			nextPartialID++
		}
	}

	result := ReconcilePlanResult{
		Lines:            append([]MoveLine(nil), allLines[:len(lines)]...),
		Partials:         partials,
		DebitResidual:    totalOpenDebit(allLines),
		CreditResidual:   totalOpenCredit(allLines),
		ReconcileBalance: reconcileOpenBalance(allLines),
	}
	if writeoffIndex >= 0 {
		writeoff := allLines[writeoffIndex]
		result.Writeoff = &writeoff
	}
	if len(partials) > 0 && result.DebitResidual == 0 && result.CreditResidual == 0 && allReconcileLinesCleared(allLines) {
		fullID := options.FullID
		if fullID <= 0 {
			fullID = partials[0].ID
		}
		partialIDs := existingPartialIDs(allLines)
		for idx := range partials {
			partials[idx].FullReconcileID = fullID
			partialIDs = appendUniqueID(partialIDs, partials[idx].ID)
		}
		for idx := range result.Lines {
			result.Lines[idx].FullReconcileID = fullID
		}
		if result.Writeoff != nil {
			result.Writeoff.FullReconcileID = fullID
		}
		result.Partials = partials
		result.Full = &FullReconcile{
			ID:                  fullID,
			Name:                fmt.Sprintf("FULL/%04d", fullID),
			PartialReconcileIDs: partialIDs,
			ReconciledLineIDs:   reconciledLineIDs(allLines),
		}
		result.FullyReconciled = true
	}
	return result, nil
}

func validateReconcileSet(lines []MoveLine) error {
	var accountID int64
	var companyID int64
	var currency string
	for _, line := range lines {
		if line.Reconciled {
			return ErrReconcileAlreadyDone
		}
		if lineOpenBalance(line) == 0 {
			continue
		}
		if accountID == 0 {
			accountID = line.Account.ID
		} else if line.Account.ID != accountID {
			return ErrReconcileAccount
		}
		if !line.Account.Reconcile {
			return ErrReconcileReconcilable
		}
		if line.CompanyID != 0 {
			if companyID == 0 {
				companyID = line.CompanyID
			} else if line.CompanyID != companyID {
				return ErrReconcileCompany
			}
		}
		if line.Currency != "" {
			if currency == "" {
				currency = line.Currency
			} else if line.Currency != currency {
				return ErrCurrencyMismatch
			}
		}
	}
	return nil
}

func splitReconcileLines(lines []MoveLine) ([]int, []int) {
	var debits []int
	var credits []int
	for idx, line := range lines {
		switch balance := lineOpenBalance(line); {
		case balance > 0:
			debits = append(debits, idx)
		case balance < 0:
			credits = append(credits, idx)
		}
	}
	return debits, credits
}

func selectCreditForDebit(lines []MoveLine, credits []int, debitIdx int) int {
	for _, creditIdx := range credits {
		if lineOpenAmount(lines[creditIdx]) == 0 {
			continue
		}
		if lines[debitIdx].PartnerID != 0 && lines[debitIdx].PartnerID == lines[creditIdx].PartnerID {
			return creditIdx
		}
	}
	for _, creditIdx := range credits {
		if lineOpenAmount(lines[creditIdx]) > 0 {
			return creditIdx
		}
	}
	return -1
}

func totalOpenDebit(lines []MoveLine) int64 {
	var total int64
	for _, line := range lines {
		if balance := lineOpenBalance(line); balance > 0 {
			total += balance
		}
	}
	return total
}

func totalOpenCredit(lines []MoveLine) int64 {
	var total int64
	for _, line := range lines {
		if balance := lineOpenBalance(line); balance < 0 {
			total += -balance
		}
	}
	return total
}

func reconcileOpenBalance(lines []MoveLine) int64 {
	var total int64
	for _, line := range lines {
		total += lineOpenBalance(line)
	}
	return total
}

func lineOpenAmount(line MoveLine) int64 {
	return abs(lineOpenBalance(line))
}

func lineOpenBalance(line MoveLine) int64 {
	if line.Reconciled {
		return 0
	}
	if line.Residual != 0 {
		return line.Residual
	}
	return line.Balance()
}

func currencyOpenBalance(line MoveLine) int64 {
	if line.Reconciled {
		return 0
	}
	if line.ResidualCurrency != 0 {
		return line.ResidualCurrency
	}
	if line.AmountCurrency != 0 {
		return line.AmountCurrency
	}
	return lineOpenBalance(line)
}

func allReconcileLinesCleared(lines []MoveLine) bool {
	for _, line := range lines {
		if lineOpenBalance(line) != 0 || currencyOpenBalance(line) != 0 {
			return false
		}
	}
	return true
}

func existingPartialIDs(lines []MoveLine) []int64 {
	var ids []int64
	for _, line := range lines {
		for _, id := range line.MatchedDebitIDs {
			ids = appendUniqueID(ids, id)
		}
		for _, id := range line.MatchedCreditIDs {
			ids = appendUniqueID(ids, id)
		}
	}
	return ids
}

func reconciledLineIDs(lines []MoveLine) []int64 {
	var ids []int64
	for _, line := range lines {
		if line.ID != 0 {
			ids = appendUniqueID(ids, line.ID)
		}
	}
	return ids
}

func appendUniqueID(ids []int64, id int64) []int64 {
	if id == 0 {
		return ids
	}
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	return append(ids, id)
}

func positiveResidual(line MoveLine) int64 {
	if line.Residual != 0 {
		return abs(line.Residual)
	}
	return abs(line.Balance())
}

func roundDiv(value int64, divisor int64) int64 {
	if divisor == 0 {
		return 0
	}
	sign := int64(1)
	if value < 0 {
		sign = -1
		value = -value
	}
	return sign * ((value + divisor/2) / divisor)
}

func abs(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func min(left int64, right int64) int64 {
	if left < right {
		return left
	}
	return right
}
