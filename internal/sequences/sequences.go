package sequences

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorp/internal/domain"
	"gorp/internal/record"
	"gorp/internal/sequencecore"
)

type Service struct {
	Env *record.Env
}

type contextKey string

func (s Service) NextByID(ctx context.Context, sequenceID int64) (string, error) {
	if s.Env == nil {
		return "", fmt.Errorf("sequence environment is nil")
	}
	rows, err := s.Env.Model("ir.sequence").Browse(sequenceID).Read("name", "prefix", "suffix", "padding", "number_next", "number_increment", "implementation", "use_date_range")
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("ir.sequence %d not found", sequenceID)
	}
	row := rows[0]
	effectiveDate := s.effectiveDate(ctx, nil)
	if boolValue(row["use_date_range"]) {
		return s.nextDateRange(sequenceID, row, effectiveDate)
	}
	number, next, mutateRow, err := sequencecore.NextNumber(sequencecore.Key{Namespace: s.Env.SequenceNamespace("ir.sequence"), Model: "ir.sequence", ID: sequenceID}, stringValue(row["implementation"]), int64Value(row["number_next"]), int64Value(row["number_increment"]))
	if err != nil {
		return "", err
	}
	value, err := formatValue(row, number, effectiveDate, time.Now().UTC())
	if err != nil {
		return "", err
	}
	if mutateRow {
		if err := s.Env.Model("ir.sequence").Browse(sequenceID).Write(map[string]any{"number_next": next, "number_next_actual": next}); err != nil {
			return "", err
		}
	}
	return value, nil
}

func (s Service) NextByCode(ctx context.Context, code string, sequenceDate any) (string, bool, error) {
	if s.Env == nil {
		return "", false, fmt.Errorf("sequence environment is nil")
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return "", false, nil
	}
	companyID := s.companyID()
	found, err := s.Env.Model("ir.sequence").Search(domain.Cond("code", domain.Equal, code))
	if err != nil {
		return "", false, err
	}
	rows, err := found.Read("code", "company_id", "active")
	if err != nil {
		return "", false, err
	}
	sort.SliceStable(rows, func(i, j int) bool {
		leftCompany := int64Value(rows[i]["company_id"])
		rightCompany := int64Value(rows[j]["company_id"])
		if leftCompany == companyID && rightCompany != companyID {
			return true
		}
		if rightCompany == companyID && leftCompany != companyID {
			return false
		}
		if leftCompany == 0 && rightCompany != 0 {
			return true
		}
		if rightCompany == 0 && leftCompany != 0 {
			return false
		}
		return int64Value(rows[i]["id"]) < int64Value(rows[j]["id"])
	})
	for _, row := range rows {
		if value, ok := row["active"]; ok && value != nil && !boolValue(value) {
			continue
		}
		rowCompanyID := int64Value(row["company_id"])
		if rowCompanyID != 0 && rowCompanyID != companyID {
			continue
		}
		ctx = ContextWithSequenceDate(ctx, sequenceDate)
		value, err := s.NextByID(ctx, int64Value(row["id"]))
		return value, true, err
	}
	return "", false, nil
}

func (s Service) nextDateRange(sequenceID int64, sequence map[string]any, effectiveDate time.Time) (string, error) {
	rangeID, rangeRow, err := s.currentDateRange(sequenceID, effectiveDate)
	if err != nil {
		return "", err
	}
	rangeDate, ok := dateValue(rangeRow["date_from"])
	if !ok {
		rangeDate = effectiveDate
	}
	number, next, mutateRow, err := sequencecore.NextNumber(sequencecore.Key{Namespace: s.Env.SequenceNamespace("ir.sequence.date_range"), Model: "ir.sequence.date_range", ID: rangeID}, stringValue(sequence["implementation"]), int64Value(rangeRow["number_next"]), int64Value(sequence["number_increment"]))
	if err != nil {
		return "", err
	}
	value, err := formatValue(sequence, number, effectiveDate, rangeDate)
	if err != nil {
		return "", err
	}
	if mutateRow {
		if err := s.Env.Model("ir.sequence.date_range").Browse(rangeID).Write(map[string]any{"number_next": next, "number_next_actual": next}); err != nil {
			return "", err
		}
	}
	return value, nil
}

func (s Service) currentDateRange(sequenceID int64, effectiveDate time.Time) (int64, map[string]any, error) {
	found, err := s.Env.Model("ir.sequence.date_range").Search(domain.Cond("sequence_id", domain.Equal, sequenceID))
	if err != nil {
		return 0, nil, err
	}
	rows, err := found.Read("date_from", "date_to", "sequence_id", "number_next")
	if err != nil {
		return 0, nil, err
	}
	for _, row := range rows {
		dateFrom, fromOK := dateValue(row["date_from"])
		dateTo, toOK := dateValue(row["date_to"])
		if fromOK && toOK && !dateFrom.After(effectiveDate) && !dateTo.Before(effectiveDate) {
			return int64Value(row["id"]), row, nil
		}
	}
	return s.createDateRange(sequenceID, effectiveDate)
}

func (s Service) createDateRange(sequenceID int64, effectiveDate time.Time) (int64, map[string]any, error) {
	yearStart := time.Date(effectiveDate.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	yearEnd := time.Date(effectiveDate.Year(), 12, 31, 0, 0, 0, 0, time.UTC)
	dateFrom := yearStart
	dateTo := yearEnd
	found, err := s.Env.Model("ir.sequence.date_range").Search(domain.Cond("sequence_id", domain.Equal, sequenceID))
	if err != nil {
		return 0, nil, err
	}
	rows, err := found.Read("date_from", "date_to", "sequence_id")
	if err != nil {
		return 0, nil, err
	}
	var latestFrom time.Time
	var latestTo time.Time
	for _, row := range rows {
		if from, ok := dateValue(row["date_from"]); ok && !from.Before(effectiveDate) && !from.After(yearEnd) && from.After(latestFrom) {
			latestFrom = from
		}
		if to, ok := dateValue(row["date_to"]); ok && !to.Before(yearStart) && !to.After(effectiveDate) && to.After(latestTo) {
			latestTo = to
		}
	}
	if !latestFrom.IsZero() {
		dateTo = latestFrom.AddDate(0, 0, -1)
	}
	if !latestTo.IsZero() {
		dateFrom = latestTo.AddDate(0, 0, 1)
	}
	id, err := s.Env.Model("ir.sequence.date_range").Create(map[string]any{
		"date_from":          dateFrom.Format("2006-01-02"),
		"date_to":            dateTo.Format("2006-01-02"),
		"sequence_id":        sequenceID,
		"number_next":        int64(1),
		"number_next_actual": int64(1),
	})
	if err != nil {
		return 0, nil, err
	}
	rows, err = s.Env.Model("ir.sequence.date_range").Browse(id).Read("date_from", "date_to", "sequence_id", "number_next")
	if err != nil {
		return 0, nil, err
	}
	if len(rows) == 0 {
		return 0, nil, fmt.Errorf("ir.sequence.date_range %d not found", id)
	}
	return id, rows[0], nil
}

func (s Service) effectiveDate(ctx context.Context, explicit any) time.Time {
	now := time.Now().UTC()
	for _, value := range []any{
		explicit,
		contextValue(ctx, "ir_sequence_date"),
		s.Env.Context().Values["ir_sequence_date"],
		s.Env.Context().Values["sequence_date"],
	} {
		if parsed, ok := dateValue(value); ok {
			return parsed
		}
	}
	return now
}

func (s Service) companyID() int64 {
	if s.Env == nil {
		return 0
	}
	if companyID := s.Env.Context().CompanyID; companyID != 0 {
		return companyID
	}
	return int64Value(s.Env.Context().Values["company_id"])
}

func ContextWithSequenceDate(ctx context.Context, value any) context.Context {
	if value == nil {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, contextKey("ir_sequence_date"), value)
}

func contextValue(ctx context.Context, key string) any {
	if ctx == nil {
		return nil
	}
	if value := ctx.Value(contextKey(key)); value != nil {
		return value
	}
	return ctx.Value(key)
}

func formatValue(sequence map[string]any, number int64, effectiveDate time.Time, rangeDate time.Time) (string, error) {
	currentDate := time.Now().UTC()
	if rangeDate.IsZero() {
		rangeDate = currentDate
	}
	prefix, err := interpolatePart(stringValue(sequence["prefix"]), effectiveDate, rangeDate, currentDate)
	if err != nil {
		return "", fmt.Errorf("invalid prefix or suffix for sequence %q", firstNonEmpty(stringValue(sequence["name"]), stringValue(sequence["id"])))
	}
	suffix, err := interpolatePart(stringValue(sequence["suffix"]), effectiveDate, rangeDate, currentDate)
	if err != nil {
		return "", fmt.Errorf("invalid prefix or suffix for sequence %q", firstNonEmpty(stringValue(sequence["name"]), stringValue(sequence["id"])))
	}
	return fmt.Sprintf("%s%0*d%s", prefix, intValue(sequence["padding"]), number, suffix), nil
}

func interpolatePart(value string, effectiveDate time.Time, rangeDate time.Time, currentDate time.Time) (string, error) {
	if value == "" {
		return "", nil
	}
	replacements := dateReplacements("", effectiveDate)
	for key, replacement := range dateReplacements("range_", rangeDate) {
		replacements[key] = replacement
	}
	for key, replacement := range dateReplacements("current_", currentDate) {
		replacements[key] = replacement
	}
	out := value
	for key, replacement := range replacements {
		out = strings.ReplaceAll(out, key, replacement)
	}
	if token := unknownToken(out); token != "" {
		return "", fmt.Errorf("unknown sequence interpolation token %s", token)
	}
	return out, nil
}

func dateReplacements(prefix string, value time.Time) map[string]string {
	if value.IsZero() {
		value = time.Now().UTC()
	}
	isoYear, isoWeek := value.ISOWeek()
	return map[string]string{
		"%(" + prefix + "year)s":    value.Format("2006"),
		"%(" + prefix + "month)s":   value.Format("01"),
		"%(" + prefix + "day)s":     value.Format("02"),
		"%(" + prefix + "y)s":       value.Format("06"),
		"%(" + prefix + "doy)s":     fmt.Sprintf("%03d", value.YearDay()),
		"%(" + prefix + "woy)s":     fmt.Sprintf("%02d", weekNumberMonday(value)),
		"%(" + prefix + "weekday)s": fmt.Sprintf("%d", int(value.Weekday())),
		"%(" + prefix + "h24)s":     value.Format("15"),
		"%(" + prefix + "h12)s":     value.Format("03"),
		"%(" + prefix + "min)s":     value.Format("04"),
		"%(" + prefix + "sec)s":     value.Format("05"),
		"%(" + prefix + "isoyear)s": fmt.Sprintf("%04d", isoYear),
		"%(" + prefix + "isoy)s":    fmt.Sprintf("%02d", isoYear%100),
		"%(" + prefix + "isoweek)s": fmt.Sprintf("%02d", isoWeek),
	}
}

func weekNumberMonday(value time.Time) int {
	yearStart := time.Date(value.Year(), 1, 1, 0, 0, 0, 0, value.Location())
	jan1WeekdayMonday := (int(yearStart.Weekday()) + 6) % 7
	firstMondayYearDay := 1 + ((7 - jan1WeekdayMonday) % 7)
	yearDay := value.YearDay()
	if yearDay < firstMondayYearDay {
		return 0
	}
	return ((yearDay - firstMondayYearDay) / 7) + 1
}

func unknownToken(value string) string {
	offset := 0
	for {
		start := strings.Index(value[offset:], "%(")
		if start < 0 {
			return ""
		}
		start += offset
		end := strings.Index(value[start:], ")s")
		if end >= 0 {
			return value[start : start+end+2]
		}
		offset = start + 2
		if offset >= len(value) {
			return ""
		}
	}
}

func dateValue(value any) (time.Time, bool) {
	switch typed := value.(type) {
	case time.Time:
		year, month, day := typed.UTC().Date()
		return time.Date(year, month, day, 0, 0, 0, 0, time.UTC), true
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return time.Time{}, false
		}
		if len(text) >= len("2006-01-02") {
			if parsed, err := time.Parse("2006-01-02", text[:len("2006-01-02")]); err == nil {
				return parsed, true
			}
		}
		for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
			parsed, err := time.Parse(layout, text)
			if err == nil {
				year, month, day := parsed.UTC().Date()
				return time.Date(year, month, day, 0, 0, 0, 0, time.UTC), true
			}
		}
	}
	return time.Time{}, false
}

func int64Value(value any) int64 {
	switch v := value.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case string:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return parsed
	default:
		return 0
	}
}

func intValue(value any) int {
	return int(int64Value(value))
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func boolValue(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "y":
			return true
		case "0", "false", "no", "n":
			return false
		}
	case int:
		return v != 0
	case int64:
		return v != 0
	case float64:
		return v != 0
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
