package data

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorp/internal/domain"
	"gorp/internal/record"
)

type evalTuple []any

type evalParser struct {
	input string
	pos   int
	ctx   evalContext
}

type evalKeywordArg struct {
	Name  string
	Value any
}

type evalKeywordExpansion struct {
	Value any
}

type evalLambda struct {
	Name string
	Expr string
}

type evalSlice struct {
	Start *int
	Stop  *int
}

type evalComprehensionClause struct {
	Name      string
	Iterable  string
	Condition string
}

type evalContext struct {
	env           *record.Env
	currentModel  string
	locals        map[string]any
	resolveID     func(string) (int64, error)
	resolveRecord func(string) (evalRef, error)
	preserveRS    bool
}

type evalRef struct {
	Model string
	ID    int64
}

type SafeEvalOptions struct {
	Env       *record.Env
	Model     string
	RecordID  int64
	RecordIDs []int64
	Locals    map[string]any
}

type evalEnvProxy struct{}

type evalUserProxy struct{}

type evalModelProxy struct {
	model string
}

type evalRecordSet struct {
	model string
	ids   []int64
}

func parseEval(input string, resolveRef func(string) (int64, error)) (any, error) {
	return parseEvalWithContext(input, evalContext{resolveID: resolveRef})
}

func parseEvalWithContext(input string, ctx evalContext) (any, error) {
	input = strings.TrimSpace(input)
	if value, ok, err := parseSpecialEval(input); ok || err != nil {
		return value, err
	}
	if value, ok, err := parseContextSpecialEval(input, ctx); ok || err != nil {
		return value, err
	}
	parser := evalParser{input: input, ctx: ctx}
	value, err := parser.parseValue()
	if err != nil {
		return nil, err
	}
	parser.skipSpace()
	if !parser.eof() {
		return nil, parser.errorf("unexpected trailing input")
	}
	return value, nil
}

func SafeEvalExpression(input string, opts SafeEvalOptions) (any, error) {
	locals := map[string]any{}
	for key, value := range opts.Locals {
		locals[key] = value
	}
	recordIDs := append([]int64(nil), opts.RecordIDs...)
	if len(recordIDs) == 0 && opts.RecordID != 0 {
		recordIDs = []int64{opts.RecordID}
	}
	if opts.Model != "" {
		locals["model"] = evalModelProxy{model: opts.Model}
		locals["records"] = evalRecordSet{model: opts.Model, ids: recordIDs}
		if len(recordIDs) > 0 {
			locals["record"] = evalRecordSet{model: opts.Model, ids: []int64{recordIDs[0]}}
		} else {
			locals["record"] = evalRecordSet{model: opts.Model}
		}
	}
	return parseEvalWithContext(input, evalContext{
		env:           opts.Env,
		currentModel:  opts.Model,
		locals:        locals,
		resolveRecord: safeEvalResolveRecord(opts.Env),
	})
}

func SafeEvalIDs(input string, opts SafeEvalOptions) ([]int64, error) {
	value, err := SafeEvalExpression(input, opts)
	if err != nil {
		return nil, err
	}
	return SafeEvalValueIDs(value), nil
}

func SafeEvalValueIDs(value any) []int64 {
	return evalIDsFromValues([]any{value})
}

func safeEvalResolveRecord(env *record.Env) func(string) (evalRef, error) {
	if env == nil {
		return nil
	}
	return func(raw string) (evalRef, error) {
		text := strings.TrimSpace(raw)
		if text == "" {
			return evalRef{}, fmt.Errorf("empty ref")
		}
		found, err := env.Model("ir.model.data").SearchWithOptions(domain.Cond("complete_name", "=", text), record.SearchOptions{Limit: 1})
		if err != nil {
			return evalRef{}, err
		}
		rows, err := found.Read("model", "res_id")
		if err != nil {
			return evalRef{}, err
		}
		if len(rows) == 0 {
			return evalRef{}, fmt.Errorf("unknown ref %s", text)
		}
		id, _ := int64Value(rows[0]["res_id"])
		return evalRef{Model: fmt.Sprint(rows[0]["model"]), ID: id}, nil
	}
}

func parseContextSpecialEval(input string, ctx evalContext) (any, bool, error) {
	if value, ok, err := evalTopLevelTernary(input, ctx); ok || err != nil {
		return value, ok, err
	}
	if value, ok, err := evalUnaryNot(input, ctx); ok || err != nil {
		return value, ok, err
	}
	if value, ok, err := evalTopLevelBoolean(input, ctx); ok || err != nil {
		return value, ok, err
	}
	if value, ok, err := evalTopLevelComparison(input, ctx); ok || err != nil {
		return value, ok, err
	}
	if value, ok, err := evalTopLevelStringFormat(input, ctx); ok || err != nil {
		return value, ok, err
	}
	if value, ok, err := evalTopLevelPlus(input, ctx); ok || err != nil {
		return value, ok, err
	}
	if value, ok, err := evalTopLevelFloorDivision(input, ctx); ok || err != nil {
		return value, ok, err
	}
	if value, ok, err := evalTopLevelSubtraction(input, ctx); ok || err != nil {
		return value, ok, err
	}
	return nil, false, nil
}

func evalUnaryNot(input string, ctx evalContext) (any, bool, error) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(input), "not ")
	if !ok || strings.TrimSpace(rest) == "" {
		return nil, false, nil
	}
	value, err := parseEvalWithContext(rest, ctx)
	if err != nil {
		return nil, true, err
	}
	return !evalTruthy(value), true, nil
}

func evalTopLevelTernary(input string, ctx evalContext) (any, bool, error) {
	ifIndex := findTopLevelKeyword(input, " if ")
	if ifIndex < 0 {
		return nil, false, nil
	}
	rest := input[ifIndex+len(" if "):]
	elseIndex := findTopLevelKeyword(rest, " else ")
	if elseIndex < 0 {
		return nil, false, nil
	}
	trueExpr := strings.TrimSpace(input[:ifIndex])
	conditionExpr := strings.TrimSpace(rest[:elseIndex])
	falseExpr := strings.TrimSpace(rest[elseIndex+len(" else "):])
	if trueExpr == "" || conditionExpr == "" || falseExpr == "" {
		return nil, false, nil
	}
	condition, err := parseEvalWithContext(conditionExpr, ctx)
	if err != nil {
		return nil, true, err
	}
	if evalTruthy(condition) {
		value, err := parseEvalWithContext(trueExpr, ctx)
		return value, true, err
	}
	value, err := parseEvalWithContext(falseExpr, ctx)
	return value, true, err
}

func evalTopLevelBoolean(input string, ctx evalContext) (any, bool, error) {
	if parts := splitTopLevelKeywordParts(input, " or "); len(parts) > 1 {
		var last any
		for _, part := range parts {
			value, err := parseEvalWithContext(part, ctx)
			if err != nil {
				return nil, true, err
			}
			last = value
			if evalTruthy(value) {
				return value, true, nil
			}
		}
		return last, true, nil
	}
	if parts := splitTopLevelKeywordParts(input, " and "); len(parts) > 1 {
		var last any
		for _, part := range parts {
			value, err := parseEvalWithContext(part, ctx)
			if err != nil {
				return nil, true, err
			}
			last = value
			if !evalTruthy(value) {
				return value, true, nil
			}
		}
		return last, true, nil
	}
	return nil, false, nil
}

func evalTopLevelComparison(input string, ctx evalContext) (any, bool, error) {
	op, index := findTopLevelComparison(input)
	if index < 0 {
		return nil, false, nil
	}
	leftExpr := strings.TrimSpace(input[:index])
	rightExpr := strings.TrimSpace(input[index+len(op):])
	if leftExpr == "" || rightExpr == "" {
		return nil, false, nil
	}
	left, err := parseEvalWithContext(leftExpr, ctx)
	if err != nil {
		return nil, true, err
	}
	right, err := parseEvalWithContext(rightExpr, ctx)
	if err != nil {
		return nil, true, err
	}
	return compareEvalValues(left, right, op), true, nil
}

func evalTopLevelStringFormat(input string, ctx evalContext) (any, bool, error) {
	index := findTopLevelOperator(input, []string{"%"})
	if index < 0 {
		return nil, false, nil
	}
	leftExpr := strings.TrimSpace(input[:index])
	rightExpr := strings.TrimSpace(input[index+1:])
	if leftExpr == "" || rightExpr == "" {
		return nil, false, nil
	}
	left, err := parseEvalWithContext(leftExpr, ctx)
	if err != nil {
		return nil, true, err
	}
	format, ok := left.(string)
	if !ok {
		return nil, false, nil
	}
	right, err := parseEvalWithContext(rightExpr, ctx)
	if err != nil {
		return nil, true, err
	}
	out, err := formatPythonPercent(format, right)
	return out, true, err
}

func evalTopLevelFloorDivision(input string, ctx evalContext) (any, bool, error) {
	index := findTopLevelOperator(input, []string{"//"})
	if index < 0 {
		return nil, false, nil
	}
	leftExpr := strings.TrimSpace(input[:index])
	rightExpr := strings.TrimSpace(input[index+2:])
	if leftExpr == "" || rightExpr == "" {
		return nil, false, nil
	}
	left, err := parseEvalWithContext(leftExpr, ctx)
	if err != nil {
		return nil, true, err
	}
	right, err := parseEvalWithContext(rightExpr, ctx)
	if err != nil {
		return nil, true, err
	}
	leftNumber, _, leftOK := numericEvalValue(left)
	rightNumber, _, rightOK := numericEvalValue(right)
	if !leftOK || !rightOK || rightNumber == 0 {
		return nil, true, fmt.Errorf("floor division requires non-zero numeric operands")
	}
	return int64(leftNumber / rightNumber), true, nil
}

func evalTopLevelSubtraction(input string, ctx evalContext) (any, bool, error) {
	if findTopLevelOperator(input, []string{"-"}) < 0 {
		return nil, false, nil
	}
	parts, signs := splitTopLevelAddSub(input)
	if len(parts) < 2 {
		return nil, false, nil
	}
	total := float64(0)
	floatResult := false
	for i, part := range parts {
		value, err := parseEvalWithContext(part, ctx)
		if err != nil {
			return nil, true, err
		}
		number, isFloat, ok := numericEvalValue(value)
		if !ok {
			return nil, false, nil
		}
		total += float64(signs[i]) * number
		floatResult = floatResult || isFloat
	}
	if floatResult {
		return total, true, nil
	}
	return int64(total), true, nil
}

func evalTopLevelPlus(input string, ctx evalContext) (any, bool, error) {
	parts := splitTopLevelPlus(input)
	if len(parts) < 2 {
		return nil, false, nil
	}
	values := make([]any, 0, len(parts))
	hasString := false
	allLists := true
	allNumbers := true
	for _, part := range parts {
		value, err := parseEvalWithContext(part, ctx)
		if err != nil {
			return nil, true, err
		}
		values = append(values, value)
		if _, ok := value.(string); ok {
			hasString = true
		}
		if _, ok := evalListValue(value); !ok {
			allLists = false
		}
		if _, _, ok := numericEvalValue(value); !ok {
			allNumbers = false
		}
	}
	switch {
	case hasString:
		var out strings.Builder
		for _, value := range values {
			out.WriteString(fmt.Sprint(value))
		}
		return out.String(), true, nil
	case allLists:
		var out []any
		for _, value := range values {
			items, _ := evalListValue(value)
			out = append(out, items...)
		}
		return out, true, nil
	case allNumbers:
		var sum float64
		floatResult := false
		for _, value := range values {
			number, isFloat, _ := numericEvalValue(value)
			sum += number
			floatResult = floatResult || isFloat
		}
		if floatResult {
			return sum, true, nil
		}
		return int64(sum), true, nil
	default:
		return nil, false, nil
	}
}

func (c evalContext) withLocal(name string, value any) evalContext {
	locals := make(map[string]any, len(c.locals)+1)
	for key, item := range c.locals {
		locals[key] = item
	}
	locals[name] = value
	c.locals = locals
	return c
}

func parseSpecialEval(input string) (any, bool, error) {
	if value, ok := parseNumericDivision(input); ok {
		return value, true, nil
	}
	if value, ok, err := parseDateTimeEval(input); ok || err != nil {
		return value, ok, err
	}
	return nil, false, nil
}

func parseNumericDivision(input string) (float64, bool) {
	if strings.Count(input, "/") != 1 {
		return 0, false
	}
	parts := strings.Split(input, "/")
	left, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return 0, false
	}
	right, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil || right == 0 {
		return 0, false
	}
	return left / right, true
}

func parseDateTimeEval(input string) (any, bool, error) {
	if !strings.Contains(input, "DateTime") && !strings.Contains(input, "datetime") {
		return nil, false, nil
	}
	expr := stripOuterParens(strings.TrimSpace(input))
	format := ""
	if base, rawFormat, ok := splitSuffixCall(expr, ".strftime("); ok {
		expr = stripOuterParens(base)
		format = rawFormat
	}
	partName := ""
	for _, name := range []string{".year", ".month", ".day"} {
		if strings.HasSuffix(expr, name) {
			expr = stripOuterParens(strings.TrimSuffix(expr, name))
			partName = strings.TrimPrefix(name, ".")
			break
		}
	}
	weekdayOnly := false
	if strings.HasSuffix(expr, ".weekday()") {
		expr = stripOuterParens(strings.TrimSuffix(expr, ".weekday()"))
		weekdayOnly = true
	}
	dateOnly := false
	if strings.HasSuffix(expr, ".date()") {
		expr = stripOuterParens(strings.TrimSuffix(expr, ".date()"))
		dateOnly = true
	}
	value, ok, err := evalDateTimeExpression(expr)
	if err != nil || !ok {
		return nil, ok, err
	}
	if format != "" {
		return formatPythonTime(value, format), true, nil
	}
	switch partName {
	case "year":
		return int64(value.Year()), true, nil
	case "month":
		return int64(value.Month()), true, nil
	case "day":
		return int64(value.Day()), true, nil
	}
	if weekdayOnly {
		return int64(pythonWeekday(value)), true, nil
	}
	if dateOnly {
		return value.Format("2006-01-02"), true, nil
	}
	return value.Format("2006-01-02 15:04:05"), true, nil
}

func evalDateTimeExpression(input string) (time.Time, bool, error) {
	expr := stripOuterParens(strings.ReplaceAll(input, " ", ""))
	value, rest, ok, err := parseDateTimeBase(expr)
	if err != nil || !ok {
		return time.Time{}, ok, err
	}
	for rest != "" {
		sign := rest[0]
		if sign != '+' && sign != '-' {
			return time.Time{}, false, nil
		}
		rest = rest[1:]
		name, args, next, ok := parseDeltaCall(rest)
		if !ok {
			return time.Time{}, false, nil
		}
		delta, err := parseDeltaArgs(args)
		if err != nil {
			return time.Time{}, true, err
		}
		if sign == '-' {
			delta = delta.negate()
		}
		switch name {
		case "timedelta":
			value = value.Add(delta.duration()).AddDate(0, 0, delta.days)
		case "relativedelta":
			value = delta.apply(value)
		default:
			return time.Time{}, false, nil
		}
		rest = next
	}
	return value, true, nil
}

func parseDateTimeBase(expr string) (time.Time, string, bool, error) {
	now := time.Now().UTC().Truncate(time.Second)
	for _, prefix := range []string{"DateTime.now()", "datetime.now()", "DateTime.today()", "datetime.today()"} {
		if strings.HasPrefix(expr, prefix) {
			value, rest, err := applyDateTimeBaseSuffixes(now, expr[len(prefix):])
			return value, rest, true, err
		}
	}
	prefix := ""
	switch {
	case strings.HasPrefix(expr, "datetime("):
		prefix = "datetime"
	case strings.HasPrefix(expr, "DateTime("):
		prefix = "DateTime"
	default:
		return time.Time{}, "", false, nil
	}
	end := matchingParenIndex(expr, len(prefix))
	if end < 0 {
		return time.Time{}, "", true, fmt.Errorf("invalid datetime constructor")
	}
	year, month, day, hour, minute, second, err := parseDateTimeConstructorArgs(expr[len(prefix)+1 : end])
	if err != nil {
		return time.Time{}, "", true, err
	}
	value := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC)
	value, rest, err := applyDateTimeBaseSuffixes(value, expr[end+1:])
	return value, rest, true, err
}

func parseDateTimeConstructorArgs(raw string) (int, int, int, int, int, int, error) {
	parts := splitTopLevelComma(raw)
	if len(parts) == 0 {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("datetime constructor requires year, month, day")
	}
	positional := []int{}
	named := map[string]int{}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		name, valueExpr, hasName := strings.Cut(part, "=")
		if hasName {
			value, err := evalIntExpression(valueExpr)
			if err != nil {
				return 0, 0, 0, 0, 0, 0, err
			}
			named[strings.TrimSpace(name)] = value
			continue
		}
		value, err := evalIntExpression(part)
		if err != nil {
			return 0, 0, 0, 0, 0, 0, err
		}
		positional = append(positional, value)
	}
	hour, minute, second := 0, 0, 0
	if len(positional) < 1 {
		return 0, 0, 0, 0, 0, 0, fmt.Errorf("datetime constructor requires year")
	}
	year := positional[0]
	month := 1
	day := 1
	if len(positional) > 1 {
		month = positional[1]
	}
	if len(positional) > 2 {
		day = positional[2]
	}
	if len(positional) > 3 {
		hour = positional[3]
	}
	if len(positional) > 4 {
		minute = positional[4]
	}
	if len(positional) > 5 {
		second = positional[5]
	}
	for name, value := range named {
		switch name {
		case "year":
			year = value
		case "month":
			month = value
		case "day":
			day = value
		case "hour":
			hour = value
		case "minute":
			minute = value
		case "second":
			second = value
		default:
			return 0, 0, 0, 0, 0, 0, fmt.Errorf("unsupported datetime argument %s", name)
		}
	}
	return year, month, day, hour, minute, second, nil
}

func applyDateTimeBaseSuffixes(value time.Time, rest string) (time.Time, string, error) {
	for {
		switch {
		case strings.HasPrefix(rest, ".date()"):
			year, month, day := value.Date()
			value = time.Date(year, month, day, 0, 0, 0, 0, value.Location())
			rest = rest[len(".date()"):]
		case strings.HasPrefix(rest, ".replace("):
			open := len(".replace")
			end := matchingParenIndex(rest, open)
			if end < 0 {
				return time.Time{}, "", fmt.Errorf("invalid datetime replace")
			}
			next, err := applyDateTimeReplace(value, rest[open+1:end])
			if err != nil {
				return time.Time{}, "", err
			}
			value = next
			rest = rest[end+1:]
		default:
			return value, rest, nil
		}
	}
}

func applyDateTimeReplace(value time.Time, raw string) (time.Time, error) {
	args, err := parseNamedIntArgs(raw)
	if err != nil {
		return time.Time{}, err
	}
	year, month, day := value.Date()
	hour, minute, second := value.Clock()
	if v, ok := args["year"]; ok {
		year = v
	}
	if v, ok := args["month"]; ok {
		month = time.Month(v)
	}
	if v, ok := args["day"]; ok {
		day = v
	}
	if v, ok := args["hour"]; ok {
		hour = v
	}
	if v, ok := args["minute"]; ok {
		minute = v
	}
	if v, ok := args["second"]; ok {
		second = v
	}
	day = minInt(day, daysInMonth(year, month))
	return time.Date(year, month, day, hour, minute, second, 0, value.Location()), nil
}

func parseNamedIntArgs(raw string) (map[string]int, error) {
	out := map[string]int{}
	if strings.TrimSpace(raw) == "" {
		return out, nil
	}
	for _, part := range strings.Split(raw, ",") {
		pieces := strings.SplitN(part, "=", 2)
		if len(pieces) != 2 {
			return nil, fmt.Errorf("argument must be named")
		}
		name := strings.TrimSpace(pieces[0])
		value, err := strconv.Atoi(strings.TrimSpace(pieces[1]))
		if err != nil {
			return nil, err
		}
		out[name] = value
	}
	return out, nil
}

func parseDeltaCall(expr string) (string, string, string, bool) {
	for _, name := range []string{"timedelta", "relativedelta"} {
		prefix := name + "("
		if !strings.HasPrefix(expr, prefix) {
			continue
		}
		end := matchingParenIndex(expr, len(name))
		if end < 0 {
			return "", "", "", false
		}
		return name, expr[len(prefix):end], expr[end+1:], true
	}
	return "", "", "", false
}

type evalDelta struct {
	years     int
	months    int
	weeks     int
	days      int
	hours     int
	minutes   int
	seconds   int
	absMonth  int
	absDay    int
	absHour   *int
	absMinute *int
	absSecond *int
	weekday   *int
}

func parseDeltaArgs(input string) (evalDelta, error) {
	var out evalDelta
	if strings.TrimSpace(input) == "" {
		return out, nil
	}
	for _, raw := range splitTopLevelComma(input) {
		parts := strings.SplitN(raw, "=", 2)
		if len(parts) != 2 {
			return out, fmt.Errorf("delta argument must be named")
		}
		name := strings.TrimSpace(parts[0])
		value, err := evalIntExpression(parts[1])
		if err != nil {
			return out, err
		}
		switch name {
		case "years":
			out.years = value
		case "months":
			out.months = value
		case "weeks":
			out.weeks = value
		case "days":
			out.days = value
		case "hours":
			out.hours = value
		case "minutes":
			out.minutes = value
		case "seconds":
			out.seconds = value
		case "month":
			out.absMonth = value
		case "day":
			out.absDay = value
		case "hour":
			out.absHour = &value
		case "minute":
			out.absMinute = &value
		case "second":
			out.absSecond = &value
		case "weekday":
			out.weekday = &value
		default:
			return out, fmt.Errorf("unsupported delta argument %s", name)
		}
	}
	return out, nil
}

func evalIntExpression(raw string) (int, error) {
	expr := strings.TrimSpace(raw)
	if expr == "" {
		return 0, fmt.Errorf("empty integer expression")
	}
	if value, err := strconv.Atoi(expr); err == nil {
		return value, nil
	}
	terms, signs := splitTopLevelAddSub(expr)
	if len(terms) > 1 {
		total := 0
		for i, term := range terms {
			value, err := evalIntExpression(term)
			if err != nil {
				return 0, err
			}
			if signs[i] < 0 {
				total -= value
			} else {
				total += value
			}
		}
		return total, nil
	}
	value, err := parseEvalWithContext(expr, evalContext{})
	if err != nil {
		return 0, err
	}
	if number, ok := int64Value(value); ok {
		return int(number), nil
	}
	return 0, fmt.Errorf("integer expression must produce int")
}

func (d evalDelta) negate() evalDelta {
	d.years = -d.years
	d.months = -d.months
	d.weeks = -d.weeks
	d.days = -d.days
	d.hours = -d.hours
	d.minutes = -d.minutes
	d.seconds = -d.seconds
	return d
}

func (d evalDelta) duration() time.Duration {
	return time.Duration(d.hours)*time.Hour + time.Duration(d.minutes)*time.Minute + time.Duration(d.seconds)*time.Second
}

func (d evalDelta) apply(value time.Time) time.Time {
	value = value.AddDate(d.years, d.months, 0)
	year, month, day := value.Date()
	if d.absMonth > 0 {
		month = time.Month(d.absMonth)
	}
	if d.absDay > 0 {
		day = d.absDay
	}
	day = minInt(day, daysInMonth(year, month))
	hour, minute, second := value.Clock()
	if d.absHour != nil {
		hour = *d.absHour
	}
	if d.absMinute != nil {
		minute = *d.absMinute
	}
	if d.absSecond != nil {
		second = *d.absSecond
	}
	value = time.Date(year, month, day, hour, minute, second, 0, value.Location())
	value = value.AddDate(0, 0, d.weeks*7+d.days).Add(d.duration())
	if d.weekday != nil {
		target := *d.weekday
		if target < 0 || target > 6 {
			return value
		}
		current := int(value.Weekday()+6) % 7
		offset := (target - current + 7) % 7
		value = value.AddDate(0, 0, offset)
	}
	return value
}

func splitSuffixCall(expr string, suffix string) (string, string, bool) {
	start := strings.LastIndex(expr, suffix)
	if start < 0 || !strings.HasSuffix(expr, ")") {
		return "", "", false
	}
	raw := strings.TrimSpace(expr[start+len(suffix) : len(expr)-1])
	if len(raw) < 2 {
		return "", "", false
	}
	quote := raw[0]
	if (quote != '\'' && quote != '"') || raw[len(raw)-1] != quote {
		return "", "", false
	}
	return expr[:start], raw[1 : len(raw)-1], true
}

func stripOuterParens(input string) string {
	for {
		input = strings.TrimSpace(input)
		if len(input) < 2 || input[0] != '(' || input[len(input)-1] != ')' {
			return input
		}
		if matchingParenIndex(input, 0) != len(input)-1 {
			return input
		}
		input = input[1 : len(input)-1]
	}
}

func matchingParenIndex(input string, open int) int {
	depth := 0
	quote := byte(0)
	for i := open; i < len(input); i++ {
		ch := input[i]
		if quote != 0 {
			if ch == quote && (i == 0 || input[i-1] != '\\') {
				quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			continue
		}
		if ch == '(' {
			depth++
			continue
		}
		if ch == ')' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func matchingDelimitedIndex(input string, open int, openCh byte, closeCh byte) int {
	depth := 0
	quote := byte(0)
	for i := open; i < len(input); i++ {
		ch := input[i]
		if quote != 0 {
			if ch == quote && (i == 0 || input[i-1] != '\\') {
				quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			continue
		}
		if ch == openCh {
			depth++
			continue
		}
		if ch == closeCh {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func splitListComprehension(input string) (string, []evalComprehensionClause, bool) {
	forIndex := findTopLevelKeyword(input, " for ")
	if forIndex < 0 {
		return "", nil, false
	}
	expr := strings.TrimSpace(input[:forIndex])
	if expr == "" {
		return "", nil, false
	}
	clauseText := " for " + strings.TrimSpace(input[forIndex+len(" for "):])
	var clauses []evalComprehensionClause
	for strings.TrimSpace(clauseText) != "" {
		if !strings.HasPrefix(clauseText, " for ") {
			return "", nil, false
		}
		rest := clauseText[len(" for "):]
		inIndex := findTopLevelKeyword(rest, " in ")
		if inIndex < 0 {
			return "", nil, false
		}
		name := strings.TrimSpace(rest[:inIndex])
		if !isSimpleIdentifier(name) {
			return "", nil, false
		}
		remaining := strings.TrimSpace(rest[inIndex+len(" in "):])
		nextFor := findTopLevelKeyword(remaining, " for ")
		iterable := remaining
		if nextFor >= 0 {
			iterable = strings.TrimSpace(remaining[:nextFor])
			clauseText = remaining[nextFor:]
		} else {
			clauseText = ""
		}
		condition := ""
		if ifIndex := findTopLevelKeyword(iterable, " if "); ifIndex >= 0 {
			condition = strings.TrimSpace(iterable[ifIndex+len(" if "):])
			iterable = strings.TrimSpace(iterable[:ifIndex])
		}
		if iterable == "" {
			return "", nil, false
		}
		clauses = append(clauses, evalComprehensionClause{Name: name, Iterable: iterable, Condition: condition})
	}
	return expr, clauses, len(clauses) > 0
}

func splitTopLevelKeywordParts(input string, keyword string) []string {
	var parts []string
	remaining := input
	for {
		index := findTopLevelKeyword(remaining, keyword)
		if index < 0 {
			break
		}
		parts = append(parts, strings.TrimSpace(remaining[:index]))
		remaining = remaining[index+len(keyword):]
	}
	if len(parts) == 0 {
		return nil
	}
	parts = append(parts, strings.TrimSpace(remaining))
	for _, part := range parts {
		if part == "" {
			return nil
		}
	}
	return parts
}

func splitTopLevelPlus(input string) []string {
	var parts []string
	start := 0
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	quote := byte(0)
	for i := 0; i < len(input); i++ {
		ch := input[i]
		if quote != 0 {
			if ch == quote && (i == 0 || input[i-1] != '\\') {
				quote = 0
			}
			continue
		}
		switch ch {
		case '\'', '"':
			quote = ch
		case '(':
			parenDepth++
		case ')':
			parenDepth--
		case '[':
			bracketDepth++
		case ']':
			bracketDepth--
		case '{':
			braceDepth++
		case '}':
			braceDepth--
		case '+':
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && !isUnaryPlus(input, i) {
				parts = append(parts, strings.TrimSpace(input[start:i]))
				start = i + 1
			}
		}
	}
	if len(parts) == 0 {
		return nil
	}
	parts = append(parts, strings.TrimSpace(input[start:]))
	for _, part := range parts {
		if part == "" {
			return nil
		}
	}
	return parts
}

func splitTopLevelComma(input string) []string {
	var parts []string
	start := 0
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	quote := byte(0)
	for i := 0; i < len(input); i++ {
		ch := input[i]
		if quote != 0 {
			if ch == quote && (i == 0 || input[i-1] != '\\') {
				quote = 0
			}
			continue
		}
		switch ch {
		case '\'', '"':
			quote = ch
		case '(':
			parenDepth++
		case ')':
			parenDepth--
		case '[':
			bracketDepth++
		case ']':
			bracketDepth--
		case '{':
			braceDepth++
		case '}':
			braceDepth--
		case ',':
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 {
				parts = append(parts, strings.TrimSpace(input[start:i]))
				start = i + 1
			}
		}
	}
	parts = append(parts, strings.TrimSpace(input[start:]))
	return parts
}

func splitTopLevelPair(input string, sep byte) (string, string, bool) {
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	quote := byte(0)
	for i := 0; i < len(input); i++ {
		ch := input[i]
		if quote != 0 {
			if ch == quote && (i == 0 || input[i-1] != '\\') {
				quote = 0
			}
			continue
		}
		switch ch {
		case '\'', '"':
			quote = ch
		case '(':
			parenDepth++
		case ')':
			parenDepth--
		case '[':
			bracketDepth++
		case ']':
			bracketDepth--
		case '{':
			braceDepth++
		case '}':
			braceDepth--
		default:
			if ch == sep && parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 {
				return strings.TrimSpace(input[:i]), strings.TrimSpace(input[i+1:]), true
			}
		}
	}
	return "", "", false
}

func splitTopLevelAddSub(input string) ([]string, []int) {
	var parts []string
	var signs []int
	start := 0
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	quote := byte(0)
	currentSign := 1
	for i := 0; i < len(input); i++ {
		ch := input[i]
		if quote != 0 {
			if ch == quote && (i == 0 || input[i-1] != '\\') {
				quote = 0
			}
			continue
		}
		switch ch {
		case '\'', '"':
			quote = ch
		case '(':
			parenDepth++
		case ')':
			parenDepth--
		case '[':
			bracketDepth++
		case ']':
			bracketDepth--
		case '{':
			braceDepth++
		case '}':
			braceDepth--
		case '+', '-':
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && !isUnaryAddSub(input, i) {
				parts = append(parts, strings.TrimSpace(input[start:i]))
				signs = append(signs, currentSign)
				if ch == '-' {
					currentSign = -1
				} else {
					currentSign = 1
				}
				start = i + 1
			}
		}
	}
	if len(parts) == 0 {
		return nil, nil
	}
	parts = append(parts, strings.TrimSpace(input[start:]))
	signs = append(signs, currentSign)
	for _, part := range parts {
		if part == "" {
			return nil, nil
		}
	}
	return parts, signs
}

func isUnaryPlus(input string, index int) bool {
	return isUnaryAddSub(input, index)
}

func isUnaryAddSub(input string, index int) bool {
	for i := index - 1; i >= 0; i-- {
		switch input[i] {
		case ' ', '\t', '\n', '\r':
			continue
		case '(', '[', '{', ',', ':', '=':
			return true
		default:
			return false
		}
	}
	return true
}

func pythonWeekday(value time.Time) int {
	return (int(value.Weekday()) + 6) % 7
}

func findTopLevelKeyword(input string, keyword string) int {
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	quote := byte(0)
	for i := 0; i <= len(input)-len(keyword); i++ {
		ch := input[i]
		if quote != 0 {
			if ch == quote && (i == 0 || input[i-1] != '\\') {
				quote = 0
			}
			continue
		}
		switch ch {
		case '\'', '"':
			quote = ch
		case '(':
			parenDepth++
		case ')':
			parenDepth--
		case '[':
			bracketDepth++
		case ']':
			bracketDepth--
		case '{':
			braceDepth++
		case '}':
			braceDepth--
		}
		if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && strings.HasPrefix(input[i:], keyword) {
			return i
		}
	}
	return -1
}

func findTopLevelComparison(input string) (string, int) {
	operators := []string{" not in ", " in ", "==", "!=", ">=", "<=", ">", "<"}
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	quote := byte(0)
	for i := 0; i < len(input); i++ {
		ch := input[i]
		if quote != 0 {
			if ch == quote && (i == 0 || input[i-1] != '\\') {
				quote = 0
			}
			continue
		}
		switch ch {
		case '\'', '"':
			quote = ch
		case '(':
			parenDepth++
		case ')':
			parenDepth--
		case '[':
			bracketDepth++
		case ']':
			bracketDepth--
		case '{':
			braceDepth++
		case '}':
			braceDepth--
		}
		if parenDepth != 0 || bracketDepth != 0 || braceDepth != 0 {
			continue
		}
		for _, op := range operators {
			if strings.HasPrefix(input[i:], op) {
				return op, i
			}
		}
	}
	return "", -1
}

func findTopLevelOperator(input string, operators []string) int {
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	quote := byte(0)
	for i := 0; i < len(input); i++ {
		ch := input[i]
		if quote != 0 {
			if ch == quote && (i == 0 || input[i-1] != '\\') {
				quote = 0
			}
			continue
		}
		switch ch {
		case '\'', '"':
			quote = ch
		case '(':
			parenDepth++
		case ')':
			parenDepth--
		case '[':
			bracketDepth++
		case ']':
			bracketDepth--
		case '{':
			braceDepth++
		case '}':
			braceDepth--
		}
		if parenDepth != 0 || bracketDepth != 0 || braceDepth != 0 {
			continue
		}
		for _, op := range operators {
			if strings.HasPrefix(input[i:], op) {
				if op == "-" && isUnaryAddSub(input, i) {
					continue
				}
				return i
			}
		}
	}
	return -1
}

func isSimpleIdentifier(input string) bool {
	if input == "" || !isIdentStart(input[0]) {
		return false
	}
	for i := 1; i < len(input); i++ {
		if !isIdentPart(input[i]) || input[i] == '.' {
			return false
		}
	}
	return true
}

func compareEvalValues(left any, right any, op string) bool {
	switch op {
	case "==":
		return evalEqual(left, right)
	case "!=":
		return !evalEqual(left, right)
	case " in ":
		return evalContains(right, left)
	case " not in ":
		return !evalContains(right, left)
	case ">", ">=", "<", "<=":
		leftNumber, _, leftOK := numericEvalValue(left)
		rightNumber, _, rightOK := numericEvalValue(right)
		if leftOK && rightOK {
			switch op {
			case ">":
				return leftNumber > rightNumber
			case ">=":
				return leftNumber >= rightNumber
			case "<":
				return leftNumber < rightNumber
			case "<=":
				return leftNumber <= rightNumber
			}
		}
		leftText, leftString := left.(string)
		rightText, rightString := right.(string)
		if leftString && rightString {
			switch op {
			case ">":
				return leftText > rightText
			case ">=":
				return leftText >= rightText
			case "<":
				return leftText < rightText
			case "<=":
				return leftText <= rightText
			}
		}
	}
	return false
}

func evalEqual(left any, right any) bool {
	if leftID, ok := int64Value(left); ok {
		if rightID, ok := int64Value(right); ok {
			return leftID == rightID
		}
	}
	switch leftTyped := left.(type) {
	case bool:
		rightTyped, ok := right.(bool)
		return ok && leftTyped == rightTyped
	case string:
		rightTyped, ok := right.(string)
		return ok && leftTyped == rightTyped
	case evalRecordSet:
		rightIDs, ok := idsFromComparable(right)
		return ok && int64SlicesEqual(leftTyped.ids, rightIDs)
	}
	leftIDs, leftOK := idsFromComparable(left)
	rightIDs, rightOK := idsFromComparable(right)
	return leftOK && rightOK && int64SlicesEqual(leftIDs, rightIDs)
}

func evalContains(container any, needle any) bool {
	needleID, needleIsID := int64Value(needle)
	switch typed := container.(type) {
	case string:
		item, ok := needle.(string)
		return ok && strings.Contains(typed, item)
	case []any:
		for _, item := range typed {
			if evalEqual(item, needle) {
				return true
			}
		}
		return false
	case []int64:
		if !needleIsID {
			return false
		}
		for _, item := range typed {
			if item == needleID {
				return true
			}
		}
		return false
	case evalTuple:
		for _, item := range typed {
			if evalEqual(item, needle) {
				return true
			}
		}
		return false
	case evalRecordSet:
		if !needleIsID {
			return false
		}
		for _, item := range typed.ids {
			if item == needleID {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func idsFromComparable(value any) ([]int64, bool) {
	switch typed := value.(type) {
	case []int64:
		return typed, true
	case []any:
		ids := make([]int64, 0, len(typed))
		for _, item := range typed {
			id, ok := int64Value(item)
			if !ok {
				return nil, false
			}
			ids = append(ids, id)
		}
		return ids, true
	case evalRecordSet:
		return typed.ids, true
	default:
		return nil, false
	}
}

func int64SlicesEqual(left []int64, right []int64) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func evalTruthy(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case string:
		return typed != ""
	case []any:
		return len(typed) > 0
	case []int64:
		return len(typed) > 0
	case evalTuple:
		return len(typed) > 0
	case evalRecordSet:
		return len(typed.ids) > 0
	case map[string]any:
		return len(typed) > 0
	}
	if id, ok := int64Value(value); ok {
		return id != 0
	}
	if number, ok := value.(float64); ok {
		return number != 0
	}
	return true
}

func evalListValue(value any) ([]any, bool) {
	switch typed := value.(type) {
	case []any:
		return typed, true
	case []int64:
		return idsAsEvalList(typed), true
	case evalTuple:
		return []any(typed), true
	default:
		return nil, false
	}
}

func numericEvalValue(value any) (float64, bool, bool) {
	if id, ok := int64Value(value); ok {
		return float64(id), false, true
	}
	switch typed := value.(type) {
	case float32:
		return float64(typed), true, true
	case float64:
		return typed, true, true
	default:
		return 0, false, false
	}
}

func formatPythonTime(value time.Time, format string) string {
	var b strings.Builder
	for i := 0; i < len(format); i++ {
		if format[i] != '%' || i == len(format)-1 {
			b.WriteByte(format[i])
			continue
		}
		i++
		switch format[i] {
		case 'Y':
			b.WriteString(fmt.Sprintf("%04d", value.Year()))
		case 'm':
			b.WriteString(fmt.Sprintf("%02d", int(value.Month())))
		case 'B':
			b.WriteString(value.Month().String())
		case 'b':
			month := value.Month().String()
			if len(month) > 3 {
				month = month[:3]
			}
			b.WriteString(month)
		case 'd':
			b.WriteString(fmt.Sprintf("%02d", value.Day()))
		case 'H':
			b.WriteString(fmt.Sprintf("%02d", value.Hour()))
		case 'M':
			b.WriteString(fmt.Sprintf("%02d", value.Minute()))
		case 'S':
			b.WriteString(fmt.Sprintf("%02d", value.Second()))
		case '%':
			b.WriteByte('%')
		default:
			b.WriteByte('%')
			b.WriteByte(format[i])
		}
	}
	return b.String()
}

func formatPythonPercent(format string, value any) (string, error) {
	args := []any{value}
	if tuple, ok := value.(evalTuple); ok {
		args = []any(tuple)
	}
	if list, ok := value.([]any); ok && strings.Count(format, "%") > 1 {
		args = list
	}
	var out strings.Builder
	argIndex := 0
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			out.WriteByte(format[i])
			continue
		}
		if i == len(format)-1 {
			return "", fmt.Errorf("incomplete format")
		}
		i++
		if format[i] == '%' {
			out.WriteByte('%')
			continue
		}
		if argIndex >= len(args) {
			return "", fmt.Errorf("not enough format arguments")
		}
		arg := args[argIndex]
		argIndex++
		switch format[i] {
		case 's', 'r':
			out.WriteString(pythonRepr(arg))
		case 'd', 'i':
			id, ok := int64Value(arg)
			if !ok {
				return "", fmt.Errorf("integer format requires int")
			}
			out.WriteString(strconv.FormatInt(id, 10))
		default:
			return "", fmt.Errorf("unsupported format %%%c", format[i])
		}
	}
	return out.String(), nil
}

func pythonRepr(value any) string {
	switch typed := value.(type) {
	case nil:
		return "None"
	case bool:
		if typed {
			return "True"
		}
		return "False"
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, pythonLiteral(item))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case []int64:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, strconv.FormatInt(item, 10))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case evalTuple:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, pythonLiteral(item))
		}
		if len(parts) == 1 {
			return "(" + parts[0] + ",)"
		}
		return "(" + strings.Join(parts, ", ") + ")"
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, pythonLiteral(key)+": "+pythonLiteral(typed[key]))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case evalRecordSet:
		return pythonRepr(idsAsEvalList(typed.ids))
	default:
		return fmt.Sprint(value)
	}
}

func pythonLiteral(value any) string {
	if text, ok := value.(string); ok {
		return "'" + strings.ReplaceAll(text, "'", "\\'") + "'"
	}
	return pythonRepr(value)
}

func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}

func (p *evalParser) parseValue() (any, error) {
	p.skipSpace()
	if p.eof() {
		return nil, p.errorf("expected value")
	}
	switch ch := p.peek(); {
	case ch == '\'' || ch == '"':
		value, err := p.parseString()
		if err != nil {
			return nil, err
		}
		return p.parsePostfixChain(value)
	case (ch == 'f' || ch == 'F') && p.pos+1 < len(p.input) && (p.input[p.pos+1] == '\'' || p.input[p.pos+1] == '"'):
		value, err := p.parseFString()
		if err != nil {
			return nil, err
		}
		return p.parsePostfixChain(value)
	case ch == '[':
		if value, ok, err := p.parseListComprehension(); ok || err != nil {
			return value, err
		}
		return p.parseList('[', ']')
	case ch == '(':
		items, err := p.parseList('(', ')')
		if err != nil {
			return nil, err
		}
		return evalTuple(items), nil
	case ch == '{':
		return p.parseDict()
	case ch == '-' || ch == '+' || isDigit(ch):
		return p.parseNumber()
	case isIdentStart(ch):
		return p.parseIdentifier()
	default:
		return nil, p.errorf("unexpected character %q", ch)
	}
}

func (p *evalParser) parseString() (string, error) {
	quote := p.peek()
	p.pos++
	var out strings.Builder
	for !p.eof() {
		ch := p.peek()
		p.pos++
		if ch == quote {
			return out.String(), nil
		}
		if ch != '\\' {
			out.WriteByte(ch)
			continue
		}
		if p.eof() {
			return "", p.errorf("unterminated escape")
		}
		escaped := p.peek()
		p.pos++
		switch escaped {
		case '\\', '\'', '"':
			out.WriteByte(escaped)
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 't':
			out.WriteByte('\t')
		case 'b':
			out.WriteByte('\b')
		case 'f':
			out.WriteByte('\f')
		default:
			out.WriteByte(escaped)
		}
	}
	return "", p.errorf("unterminated string")
}

func (p *evalParser) parseFString() (string, error) {
	p.pos++
	template, err := p.parseString()
	if err != nil {
		return "", err
	}
	return evalFStringTemplate(template, p.ctx)
}

func evalFStringTemplate(template string, ctx evalContext) (string, error) {
	var out strings.Builder
	for i := 0; i < len(template); i++ {
		switch template[i] {
		case '{':
			if i+1 < len(template) && template[i+1] == '{' {
				out.WriteByte('{')
				i++
				continue
			}
			end := strings.IndexByte(template[i+1:], '}')
			if end < 0 {
				return "", fmt.Errorf("unterminated f-string expression")
			}
			expr := strings.TrimSpace(template[i+1 : i+1+end])
			if expr == "" {
				return "", fmt.Errorf("empty f-string expression")
			}
			value, err := parseEvalWithContext(expr, ctx)
			if err != nil {
				return "", err
			}
			out.WriteString(formatFStringValue(value))
			i += end + 1
		case '}':
			if i+1 < len(template) && template[i+1] == '}' {
				out.WriteByte('}')
				i++
				continue
			}
			return "", fmt.Errorf("single '}' in f-string")
		default:
			out.WriteByte(template[i])
		}
	}
	return out.String(), nil
}

func formatFStringValue(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return pythonRepr(value)
}

func (p *evalParser) parseList(open byte, close byte) ([]any, error) {
	if !p.consume(open) {
		return nil, p.errorf("expected %q", open)
	}
	var out []any
	p.skipSpace()
	if p.consume(close) {
		return out, nil
	}
	for {
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		out = append(out, value)
		p.skipSpace()
		if p.consume(',') {
			p.skipSpace()
			if p.consume(close) {
				return out, nil
			}
			continue
		}
		if p.consume(close) {
			return out, nil
		}
		return nil, p.errorf("expected ',' or %q", close)
	}
}

func (p *evalParser) parseListComprehension() (any, bool, error) {
	start := p.pos
	end := matchingDelimitedIndex(p.input, start, '[', ']')
	if end < 0 {
		return nil, true, p.errorf("unterminated list")
	}
	expr, clauses, ok := splitListComprehension(p.input[start+1 : end])
	if !ok {
		return nil, false, nil
	}
	out, err := p.evalComprehensionClauses(expr, clauses, p.ctx)
	if err != nil {
		return nil, true, err
	}
	p.pos = end + 1
	return out, true, nil
}

func (p *evalParser) evalComprehensionClauses(expr string, clauses []evalComprehensionClause, ctx evalContext) ([]any, error) {
	if len(clauses) == 0 {
		value, err := parseEvalWithContext(expr, ctx)
		if err != nil {
			return nil, err
		}
		return []any{value}, nil
	}
	clause := clauses[0]
	values, err := p.evalComprehensionIterableWithContext(clause.Iterable, ctx)
	if err != nil {
		return nil, err
	}
	var out []any
	for _, item := range values {
		itemCtx := ctx.withLocal(clause.Name, item)
		if clause.Condition != "" {
			keep, err := parseEvalWithContext(clause.Condition, itemCtx)
			if err != nil {
				return nil, err
			}
			if !evalTruthy(keep) {
				continue
			}
		}
		items, err := p.evalComprehensionClauses(expr, clauses[1:], itemCtx)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	return out, nil
}

func (p *evalParser) evalComprehensionIterable(raw string) ([]any, error) {
	return p.evalComprehensionIterableWithContext(raw, p.ctx)
}

func (p *evalParser) evalComprehensionIterableWithContext(raw string, ctx evalContext) ([]any, error) {
	value, err := parseEvalWithContext(raw, ctx)
	if err != nil {
		return nil, err
	}
	switch typed := value.(type) {
	case []any:
		return typed, nil
	case []int64:
		return idsAsEvalList(typed), nil
	case evalTuple:
		return []any(typed), nil
	case bool:
		if !typed {
			return []any{}, nil
		}
	}
	return []any{value}, nil
}

func (p *evalParser) parseDict() (map[string]any, error) {
	start := p.pos
	if !p.consume('{') {
		return nil, p.errorf("expected '{'")
	}
	end := matchingDelimitedIndex(p.input, start, '{', '}')
	if end < 0 {
		return nil, p.errorf("unterminated dict")
	}
	body := strings.TrimSpace(p.input[start+1 : end])
	out := map[string]any{}
	if body == "" {
		p.pos = end + 1
		return out, nil
	}
	for _, item := range splitTopLevelComma(body) {
		if strings.TrimSpace(item) == "" {
			continue
		}
		keyExpr, valueExpr, ok := splitTopLevelPair(item, ':')
		if !ok {
			return nil, p.errorf("dict item requires ':'")
		}
		rawKey, err := parseEvalWithContext(keyExpr, p.ctx)
		if err != nil {
			return nil, err
		}
		key, ok := rawKey.(string)
		if !ok {
			return nil, p.errorf("dict key must be string")
		}
		value, err := parseEvalWithContext(valueExpr, p.ctx)
		if err != nil {
			return nil, err
		}
		out[key] = value
	}
	p.pos = end + 1
	return out, nil
}

func (p *evalParser) parseNumber() (any, error) {
	start := p.pos
	if p.peek() == '-' || p.peek() == '+' {
		p.pos++
	}
	digits := 0
	for !p.eof() && isDigit(p.peek()) {
		p.pos++
		digits++
	}
	isFloat := false
	if !p.eof() && p.peek() == '.' {
		isFloat = true
		p.pos++
		for !p.eof() && isDigit(p.peek()) {
			p.pos++
			digits++
		}
	}
	if digits == 0 {
		return nil, p.errorf("invalid number")
	}
	if !p.eof() && (p.peek() == 'e' || p.peek() == 'E') {
		isFloat = true
		p.pos++
		if !p.eof() && (p.peek() == '-' || p.peek() == '+') {
			p.pos++
		}
		expDigits := 0
		for !p.eof() && isDigit(p.peek()) {
			p.pos++
			expDigits++
		}
		if expDigits == 0 {
			return nil, p.errorf("invalid exponent")
		}
	}
	text := p.input[start:p.pos]
	if isFloat {
		value, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return nil, err
		}
		return value, nil
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return nil, err
	}
	return value, nil
}

func (p *evalParser) parseIdentifier() (any, error) {
	start := p.pos
	p.pos++
	for !p.eof() && isIdentPart(p.peek()) {
		p.pos++
	}
	ident := p.input[start:p.pos]
	p.skipSpace()
	if p.consume('=') {
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		return evalKeywordArg{Name: ident, Value: value}, nil
	}
	if value, ok, err := p.localValue(ident); ok || err != nil {
		return value, err
	}
	switch ident {
	case "True", "true":
		return true, nil
	case "False", "false":
		return false, nil
	case "None", "none", "null":
		return nil, nil
	}
	if !p.consume('(') {
		return ident, nil
	}
	args, err := p.parseCallArgs()
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(ident, "Command.") {
		return p.parseCommandCall(ident, args)
	}
	if ident == "time.strftime" {
		return p.parseTimeStrftime(args)
	}
	if ident == "obj" {
		return p.parseObjCall(args)
	}
	switch ident {
	case "str":
		return p.parseBuiltinStr(args)
	case "dict":
		return p.parseBuiltinDict(args)
	case "list":
		return p.parseBuiltinList(args)
	case "range":
		return p.parseBuiltinRange(args)
	case "map":
		return p.parseBuiltinMap(args)
	}
	if ident != "ref" {
		return nil, p.errorf("unsupported function %s", ident)
	}
	if len(args) < 1 || len(args) > 2 {
		return nil, p.errorf("ref requires 1 or 2 arguments")
	}
	ref, ok := args[0].(string)
	if !ok {
		return nil, p.errorf("ref argument must be string")
	}
	recordRef, err := p.resolveEvalRef(ref)
	if err == nil {
		return recordRef.ID, nil
	}
	if len(args) == 2 && isFalseRefFallback(args[1]) {
		return false, nil
	}
	return nil, err
}

func (p *evalParser) parseTimeStrftime(args []any) (any, error) {
	if len(args) != 1 {
		return nil, p.errorf("time.strftime requires 1 argument")
	}
	format, ok := args[0].(string)
	if !ok {
		return nil, p.errorf("time.strftime argument must be string")
	}
	return formatPythonTime(time.Now().UTC(), format), nil
}

func (p *evalParser) parseBuiltinStr(args []any) (any, error) {
	if len(args) != 1 {
		return nil, p.errorf("str requires 1 argument")
	}
	return pythonRepr(args[0]), nil
}

func (p *evalParser) parseBuiltinDict(args []any) (any, error) {
	out := map[string]any{}
	for _, arg := range args {
		switch typed := arg.(type) {
		case map[string]any:
			for key, value := range typed {
				out[key] = value
			}
		case evalKeywordExpansion:
			values, ok := typed.Value.(map[string]any)
			if !ok {
				return nil, p.errorf("dict ** argument must be dict")
			}
			for key, value := range values {
				out[key] = value
			}
		case evalKeywordArg:
			out[typed.Name] = typed.Value
		default:
			return nil, p.errorf("dict argument must be dict or keyword")
		}
	}
	return out, nil
}

func (p *evalParser) parseBuiltinList(args []any) (any, error) {
	if len(args) != 1 {
		return nil, p.errorf("list requires 1 argument")
	}
	switch typed := args[0].(type) {
	case []any:
		return typed, nil
	case []int64:
		return idsAsEvalList(typed), nil
	case evalTuple:
		return []any(typed), nil
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out := make([]any, 0, len(keys))
		for _, key := range keys {
			out = append(out, key)
		}
		return out, nil
	case evalRecordSet:
		return idsAsEvalList(typed.ids), nil
	case string:
		out := make([]any, 0, len([]rune(typed)))
		for _, ch := range typed {
			out = append(out, string(ch))
		}
		return out, nil
	default:
		return nil, p.errorf("unsupported list argument")
	}
}

func (p *evalParser) parseBuiltinRange(args []any) (any, error) {
	if len(args) < 1 || len(args) > 3 {
		return nil, p.errorf("range requires 1 to 3 arguments")
	}
	values := make([]int64, 0, len(args))
	for _, arg := range args {
		value, ok := int64Value(arg)
		if !ok {
			return nil, p.errorf("range arguments must be ints")
		}
		values = append(values, value)
	}
	start, stop, step := int64(0), values[0], int64(1)
	if len(values) >= 2 {
		start = values[0]
		stop = values[1]
	}
	if len(values) == 3 {
		step = values[2]
	}
	if step == 0 {
		return nil, p.errorf("range step cannot be zero")
	}
	var out []any
	for value := start; (step > 0 && value < stop) || (step < 0 && value > stop); value += step {
		out = append(out, value)
	}
	return out, nil
}

func (p *evalParser) parseBuiltinMap(args []any) (any, error) {
	if len(args) != 2 {
		return nil, p.errorf("map requires 2 arguments")
	}
	funcName, ok := args[0].(string)
	if !ok || funcName != "str" {
		return nil, p.errorf("only map(str, iterable) is supported")
	}
	items, ok := evalListValue(args[1])
	if !ok {
		return nil, p.errorf("map iterable must be list-like")
	}
	out := make([]any, 0, len(items))
	for _, item := range items {
		out = append(out, pythonRepr(item))
	}
	return out, nil
}

func (p *evalParser) localValue(ident string) (any, bool, error) {
	if len(p.ctx.locals) == 0 {
		return nil, false, nil
	}
	if value, ok := p.ctx.locals[ident]; ok {
		return value, true, nil
	}
	parts := strings.Split(ident, ".")
	if len(parts) < 2 {
		return nil, false, nil
	}
	value, ok := p.ctx.locals[parts[0]]
	if !ok {
		return nil, false, nil
	}
	for _, attr := range parts[1:] {
		next, err := p.localAttrValue(value, attr)
		if err != nil {
			return nil, true, err
		}
		value = next
	}
	return value, true, nil
}

func (p *evalParser) localAttrValue(value any, attr string) (any, error) {
	if id, ok := int64Value(value); ok {
		switch attr {
		case "id":
			return id, nil
		case "ids":
			return []any{id}, nil
		default:
			return nil, p.errorf("unsupported local id attribute %s", attr)
		}
	}
	switch typed := value.(type) {
	case evalRecordSet:
		return p.getObjAttr(typed, attr)
	default:
		return nil, p.errorf("unsupported local attribute %s", attr)
	}
}

func (p *evalParser) parseObjCall(args []any) (any, error) {
	if len(args) > 1 {
		return nil, p.errorf("obj requires zero or one argument")
	}
	if p.ctx.env == nil {
		return nil, p.errorf("obj requires eval environment")
	}
	value := any(evalModelProxy{model: p.ctx.currentModel})
	if len(args) == 1 {
		if strings.TrimSpace(p.ctx.currentModel) == "" {
			return nil, p.errorf("obj current model is not available")
		}
		ids, err := idsFromEvalArg(args[0])
		if err != nil {
			return nil, err
		}
		value = evalRecordSet{model: p.ctx.currentModel, ids: ids}
	}
	return p.parseObjChain(value)
}

func (p *evalParser) parseObjChain(value any) (any, error) {
	return p.parsePostfixChain(value)
}

func (p *evalParser) parsePostfixChain(value any) (any, error) {
	for {
		p.skipSpace()
		if p.consume('[') {
			rawIndex, err := p.parseIndexValue()
			p.skipSpace()
			if !p.consume(']') {
				return nil, p.errorf("expected ']'")
			}
			if _, ok := value.(evalEnvProxy); ok {
				modelName, ok := rawIndex.(string)
				if !ok || strings.TrimSpace(modelName) == "" {
					return nil, p.errorf("env index must be a model name")
				}
				value = evalModelProxy{model: modelName}
				continue
			}
			value, err = p.indexObjValue(value, rawIndex)
			if err != nil {
				return nil, err
			}
			continue
		}
		if !p.consume('.') {
			if p.ctx.preserveRS {
				return value, nil
			}
			return finalizeEvalObject(value), nil
		}
		attr, err := p.parseAttrName()
		if err != nil {
			return nil, err
		}
		p.skipSpace()
		if p.consume('(') {
			args, err := p.parseCallArgs()
			if err != nil {
				return nil, err
			}
			value, err = p.callObjAttr(value, attr, args)
			if err != nil {
				return nil, err
			}
			continue
		}
		value, err = p.getObjAttr(value, attr)
		if err != nil {
			return nil, err
		}
	}
}

func (p *evalParser) parseAttrName() (string, error) {
	if p.eof() || !isIdentStart(p.peek()) {
		return "", p.errorf("expected attribute name")
	}
	start := p.pos
	p.pos++
	for !p.eof() && (isIdentStart(p.peek()) || isDigit(p.peek())) {
		p.pos++
	}
	return p.input[start:p.pos], nil
}

func (p *evalParser) parseIndexValue() (any, error) {
	start := p.pos
	depth := 0
	quote := byte(0)
	for !p.eof() {
		ch := p.peek()
		if quote != 0 {
			p.pos++
			if ch == quote && (p.pos < 2 || p.input[p.pos-2] != '\\') {
				quote = 0
			}
			continue
		}
		switch ch {
		case '\'', '"':
			quote = ch
			p.pos++
		case '[', '(', '{':
			depth++
			p.pos++
		case ']', ')', '}':
			if depth == 0 {
				if ch == ']' {
					raw := strings.TrimSpace(p.input[start:p.pos])
					return p.evalIndexExpression(raw)
				}
				return nil, p.errorf("unexpected closing delimiter")
			}
			depth--
			p.pos++
		default:
			p.pos++
		}
	}
	return nil, p.errorf("unterminated index")
}

func (p *evalParser) evalIndexExpression(raw string) (any, error) {
	if strings.Contains(raw, ":") {
		parts := strings.Split(raw, ":")
		if len(parts) != 2 {
			return nil, p.errorf("unsupported slice")
		}
		slice := evalSlice{}
		if strings.TrimSpace(parts[0]) != "" {
			start, err := parseSliceBound(parts[0])
			if err != nil {
				return nil, err
			}
			slice.Start = &start
		}
		if strings.TrimSpace(parts[1]) != "" {
			stop, err := parseSliceBound(parts[1])
			if err != nil {
				return nil, err
			}
			slice.Stop = &stop
		}
		return slice, nil
	}
	return parseEvalWithContext(raw, p.ctx)
}

func parseSliceBound(raw string) (int, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("slice bound must be int")
	}
	return int(value), nil
}

func (p *evalParser) getObjAttr(value any, attr string) (any, error) {
	switch typed := value.(type) {
	case evalModelProxy:
		if attr == "env" {
			return evalEnvProxy{}, nil
		}
		return nil, p.errorf("unsupported obj model attribute %s", attr)
	case evalEnvProxy:
		switch attr {
		case "company":
			id, err := p.currentCompanyID()
			if err != nil {
				return nil, err
			}
			return evalRecordSet{model: "res.company", ids: idSlice(id)}, nil
		case "user":
			return evalRecordSet{model: "res.users", ids: idSlice(p.ctx.env.Context().UserID)}, nil
		}
		return nil, p.errorf("unsupported env attribute %s", attr)
	case evalRecordSet:
		switch attr {
		case "id":
			if len(typed.ids) == 0 {
				return false, nil
			}
			return typed.ids[0], nil
		case "ids":
			return idsAsEvalList(typed.ids), nil
		default:
			return p.evalRecordField(typed, attr)
		}
	case string:
		return nil, p.errorf("unsupported string attribute %s", attr)
	default:
		return nil, p.errorf("unsupported attribute %s", attr)
	}
}

func (p *evalParser) callObjAttr(value any, attr string, args []any) (any, error) {
	switch typed := value.(type) {
	case evalEnvProxy:
		if attr != "ref" {
			return nil, p.errorf("unsupported env function %s", attr)
		}
		if len(args) != 1 {
			return nil, p.errorf("env.ref requires 1 argument")
		}
		refName, ok := args[0].(string)
		if !ok {
			return nil, p.errorf("env.ref argument must be string")
		}
		ref, err := p.resolveEvalRef(refName)
		if err != nil {
			return nil, err
		}
		return evalRecordSet{model: ref.Model, ids: []int64{ref.ID}}, nil
	case evalUserProxy:
		if attr != "has_group" {
			return nil, p.errorf("unsupported user function %s", attr)
		}
		if len(args) != 1 {
			return nil, p.errorf("user.has_group requires 1 argument")
		}
		groupRef, ok := args[0].(string)
		if !ok {
			return nil, p.errorf("user.has_group argument must be string")
		}
		return p.evalUserHasGroup(groupRef)
	case evalModelProxy:
		return p.callModelAttr(typed.model, attr, args)
	case evalRecordSet:
		if typed.model == "res.users" && attr == "has_group" {
			return p.callUserHasGroup(args)
		}
		return p.callRecordsetAttr(typed, attr, args)
	case string:
		return p.callStringAttr(typed, attr, args)
	default:
		return nil, p.errorf("unsupported function %s", attr)
	}
}

func (p *evalParser) callUserHasGroup(args []any) (any, error) {
	if len(args) != 1 {
		return nil, p.errorf("user.has_group requires 1 argument")
	}
	groupRef, ok := args[0].(string)
	if !ok {
		return nil, p.errorf("user.has_group argument must be string")
	}
	return p.evalUserHasGroup(groupRef)
}

func (p *evalParser) callStringAttr(value string, attr string, args []any) (any, error) {
	switch attr {
	case "replace":
		if len(args) != 2 {
			return nil, p.errorf("replace requires 2 arguments")
		}
		old, ok := args[0].(string)
		if !ok {
			return nil, p.errorf("replace old argument must be string")
		}
		newValue, ok := args[1].(string)
		if !ok {
			return nil, p.errorf("replace new argument must be string")
		}
		return strings.ReplaceAll(value, old, newValue), nil
	case "lower":
		if len(args) != 0 {
			return nil, p.errorf("lower requires no arguments")
		}
		return strings.ToLower(value), nil
	case "upper":
		if len(args) != 0 {
			return nil, p.errorf("upper requires no arguments")
		}
		return strings.ToUpper(value), nil
	case "split":
		if len(args) > 1 {
			return nil, p.errorf("split requires zero or one argument")
		}
		separator := ""
		if len(args) == 1 {
			var ok bool
			separator, ok = args[0].(string)
			if !ok {
				return nil, p.errorf("split separator must be string")
			}
		}
		parts := strings.Fields(value)
		if len(args) == 1 {
			parts = strings.Split(value, separator)
		}
		out := make([]any, 0, len(parts))
		for _, part := range parts {
			out = append(out, part)
		}
		return out, nil
	case "join":
		if len(args) != 1 {
			return nil, p.errorf("join requires 1 argument")
		}
		items, ok := evalListValue(args[0])
		if !ok {
			return nil, p.errorf("join argument must be list-like")
		}
		parts := make([]string, 0, len(items))
		for _, item := range items {
			text, ok := item.(string)
			if !ok {
				return nil, p.errorf("join items must be strings")
			}
			parts = append(parts, text)
		}
		return strings.Join(parts, value), nil
	case "format":
		out := value
		for _, arg := range args {
			index := strings.Index(out, "{}")
			if index < 0 {
				return nil, p.errorf("not enough format placeholders")
			}
			out = out[:index] + pythonRepr(arg) + out[index+2:]
		}
		if strings.Contains(out, "{}") {
			return nil, p.errorf("not enough format arguments")
		}
		return out, nil
	default:
		return nil, p.errorf("unsupported string function %s", attr)
	}
}

func (p *evalParser) indexObjValue(value any, rawIndex any) (any, error) {
	if slice, ok := rawIndex.(evalSlice); ok {
		return p.sliceObjValue(value, slice)
	}
	index64, ok := int64Value(rawIndex)
	if !ok {
		return nil, p.errorf("index must be int")
	}
	index := int(index64)
	switch typed := value.(type) {
	case []any:
		if index < 0 {
			index = len(typed) + index
		}
		if index < 0 || index >= len(typed) {
			return nil, p.errorf("list index out of range")
		}
		return typed[index], nil
	case evalTuple:
		items := []any(typed)
		if index < 0 {
			index = len(items) + index
		}
		if index < 0 || index >= len(items) {
			return nil, p.errorf("tuple index out of range")
		}
		return items[index], nil
	case string:
		runes := []rune(typed)
		if index < 0 {
			index = len(runes) + index
		}
		if index < 0 || index >= len(runes) {
			return nil, p.errorf("string index out of range")
		}
		return string(runes[index]), nil
	default:
		return nil, p.errorf("unsupported index target")
	}
}

func (p *evalParser) sliceObjValue(value any, slice evalSlice) (any, error) {
	switch typed := value.(type) {
	case []any:
		start, stop := normalizeSliceBounds(len(typed), slice)
		out := append([]any(nil), typed[start:stop]...)
		return out, nil
	case evalTuple:
		items := []any(typed)
		start, stop := normalizeSliceBounds(len(items), slice)
		return evalTuple(append([]any(nil), items[start:stop]...)), nil
	case string:
		runes := []rune(typed)
		start, stop := normalizeSliceBounds(len(runes), slice)
		return string(runes[start:stop]), nil
	case evalRecordSet:
		start, stop := normalizeSliceBounds(len(typed.ids), slice)
		return evalRecordSet{model: typed.model, ids: append([]int64(nil), typed.ids[start:stop]...)}, nil
	default:
		return nil, p.errorf("unsupported slice target")
	}
}

func normalizeSliceBounds(length int, slice evalSlice) (int, int) {
	start := 0
	stop := length
	if slice.Start != nil {
		start = *slice.Start
		if start < 0 {
			start = length + start
		}
	}
	if slice.Stop != nil {
		stop = *slice.Stop
		if stop < 0 {
			stop = length + stop
		}
	}
	if start < 0 {
		start = 0
	}
	if stop < 0 {
		stop = 0
	}
	if start > length {
		start = length
	}
	if stop > length {
		stop = length
	}
	if stop < start {
		stop = start
	}
	return start, stop
}

func (p *evalParser) evalUserHasGroup(groupRef string) (bool, error) {
	ref, err := p.resolveEvalRef(groupRef)
	if err != nil {
		return false, err
	}
	groupID := ref.ID
	if groupID == 0 {
		return false, nil
	}
	for _, id := range evalIDsFromValues([]any{p.ctx.env.Context().Values["group_ids"]}) {
		if id == groupID {
			return true, nil
		}
	}
	userID := p.ctx.env.Context().UserID
	if userID == 0 {
		return false, nil
	}
	rows, err := p.ctx.env.Model("res.users").Browse(userID).Read("groups_id")
	if err != nil {
		return false, err
	}
	if len(rows) == 0 {
		return false, nil
	}
	for _, id := range evalIDsFromValues([]any{rows[0]["groups_id"]}) {
		if id == groupID {
			return true, nil
		}
	}
	return false, nil
}

func (p *evalParser) callModelAttr(modelName string, attr string, args []any) (any, error) {
	switch attr {
	case "sudo", "with_context":
		return evalModelProxy{model: modelName}, nil
	case "browse":
		if strings.TrimSpace(modelName) == "" {
			return nil, p.errorf("obj current model is not available")
		}
		if len(args) != 1 {
			return nil, p.errorf("browse requires 1 argument")
		}
		ids, err := idsFromEvalArg(args[0])
		if err != nil {
			return nil, err
		}
		return evalRecordSet{model: modelName, ids: ids}, nil
	case "search":
		if strings.TrimSpace(modelName) == "" {
			return nil, p.errorf("obj current model is not available")
		}
		return p.evalSearch(modelName, args)
	case "create":
		if len(args) != 1 {
			return nil, p.errorf("create requires 1 argument")
		}
		values, ok := args[0].(map[string]any)
		if !ok {
			return nil, p.errorf("create argument must be dict")
		}
		id, err := p.ctx.env.Model(modelName).Create(normalizeEvalTuples(values).(map[string]any))
		if err != nil {
			return nil, err
		}
		return evalRecordSet{model: modelName, ids: []int64{id}}, nil
	case "fields_get":
		if len(args) > 0 {
			return nil, p.errorf("fields_get arguments are unsupported")
		}
		fields, err := p.ctx.env.Model(modelName).FieldsGet(nil, nil)
		if err != nil {
			return nil, err
		}
		out := make(map[string]any, len(fields))
		for key, value := range fields {
			out[key] = value
		}
		return out, nil
	case "default_get":
		if len(args) != 1 {
			return nil, p.errorf("default_get requires 1 argument")
		}
		names, err := stringListFromEval(args[0])
		if err != nil {
			return nil, err
		}
		return p.ctx.env.Model(modelName).DefaultGet(names, nil)
	case "_get_id":
		if len(args) != 1 {
			return nil, p.errorf("_get_id requires 1 argument")
		}
		target, ok := args[0].(string)
		if !ok {
			return nil, p.errorf("_get_id argument must be string")
		}
		return p.evalModelID(target)
	default:
		return nil, p.errorf("unsupported obj function %s", attr)
	}
}

func (p *evalParser) callRecordsetAttr(rs evalRecordSet, attr string, args []any) (any, error) {
	switch attr {
	case "sudo", "with_context":
		return rs, nil
	case "mapped":
		if len(args) != 1 {
			return nil, p.errorf("mapped requires 1 argument")
		}
		path, ok := args[0].(string)
		if !ok {
			return nil, p.errorf("mapped path must be string")
		}
		return p.evalMapped(rs, path)
	case "filtered":
		if len(args) != 1 {
			return nil, p.errorf("filtered requires 1 argument")
		}
		lambda, ok := args[0].(evalLambda)
		if !ok {
			return nil, p.errorf("filtered argument must be lambda")
		}
		return p.evalFiltered(rs, lambda)
	default:
		return p.callModelAttr(rs.model, attr, args)
	}
}

func (p *evalParser) evalMapped(rs evalRecordSet, path string) (any, error) {
	parts := strings.Split(strings.TrimSpace(path), ".")
	if len(parts) == 0 || parts[0] == "" {
		return nil, p.errorf("mapped path is empty")
	}
	var out []any
	recordsetOut := evalRecordSet{}
	for _, id := range rs.ids {
		value := any(evalRecordSet{model: rs.model, ids: []int64{id}})
		for _, part := range parts {
			next, err := p.getObjAttr(value, part)
			if err != nil {
				return nil, err
			}
			value = next
		}
		if mapped, ok := value.(evalRecordSet); ok {
			if recordsetOut.model == "" {
				recordsetOut.model = mapped.model
			}
			if recordsetOut.model == mapped.model {
				recordsetOut.ids = append(recordsetOut.ids, mapped.ids...)
				continue
			}
		}
		out = append(out, value)
	}
	if len(out) == 0 && recordsetOut.model != "" {
		return recordsetOut, nil
	}
	if recordsetOut.model != "" {
		out = append(idsAsEvalList(recordsetOut.ids), out...)
	}
	return out, nil
}

func (p *evalParser) evalFiltered(rs evalRecordSet, lambda evalLambda) (evalRecordSet, error) {
	out := evalRecordSet{model: rs.model}
	for _, id := range rs.ids {
		local := evalRecordSet{model: rs.model, ids: []int64{id}}
		keep, err := parseEvalWithContext(lambda.Expr, p.ctx.withLocal(lambda.Name, local))
		if err != nil {
			return evalRecordSet{}, err
		}
		if evalTruthy(keep) {
			out.ids = append(out.ids, id)
		}
	}
	return out, nil
}

func (p *evalParser) currentCompanyID() (int64, error) {
	if p.ctx.env == nil {
		return 0, p.errorf("env.company requires eval environment")
	}
	ctx := p.ctx.env.Context()
	if ctx.CompanyID != 0 {
		return ctx.CompanyID, nil
	}
	for _, id := range ctx.CompanyIDs {
		if id != 0 {
			return id, nil
		}
	}
	found, err := p.ctx.env.Model("res.company").SearchWithOptions(domain.And(), record.SearchOptions{Limit: 1})
	if err != nil {
		if strings.Contains(err.Error(), "unknown model res.company") {
			return 0, nil
		}
		return 0, err
	}
	ids := found.IDs()
	if len(ids) == 0 {
		return 0, nil
	}
	return ids[0], nil
}

func idSlice(id int64) []int64 {
	if id == 0 {
		return nil
	}
	return []int64{id}
}

func (p *evalParser) evalSearch(modelName string, args []any) (evalRecordSet, error) {
	if p.ctx.env == nil {
		return evalRecordSet{}, p.errorf("search requires eval environment")
	}
	if len(args) == 0 {
		return evalRecordSet{}, p.errorf("search requires a domain")
	}
	node, err := domain.Parse(normalizeEvalTuples(args[0]))
	if err != nil {
		return evalRecordSet{}, err
	}
	opts, err := searchOptionsFromEvalArgs(args[1:])
	if err != nil {
		return evalRecordSet{}, err
	}
	found, err := p.ctx.env.Model(modelName).SearchWithOptions(node, opts)
	if err != nil {
		return evalRecordSet{}, err
	}
	return evalRecordSet{model: modelName, ids: found.IDs()}, nil
}

func (p *evalParser) evalModelID(modelName string) (any, error) {
	if p.ctx.env == nil {
		return nil, p.errorf("_get_id requires eval environment")
	}
	found, err := p.ctx.env.Model("ir.model").Search(domain.Cond("model", "=", modelName))
	if err != nil {
		if strings.Contains(err.Error(), "unknown model ir.model") {
			return false, nil
		}
		return nil, err
	}
	ids := found.IDs()
	if len(ids) == 0 {
		return false, nil
	}
	return ids[0], nil
}

func (p *evalParser) evalRecordField(rs evalRecordSet, fieldName string) (any, error) {
	if p.ctx.env == nil {
		return nil, p.errorf("field access requires eval environment")
	}
	if strings.TrimSpace(rs.model) == "" {
		return nil, p.errorf("record model is not available")
	}
	if len(rs.ids) == 0 {
		return false, nil
	}
	fields, err := p.ctx.env.Model(rs.model).FieldsGet([]string{fieldName}, nil)
	if err != nil {
		return nil, err
	}
	info, ok := fields[fieldName]
	if !ok {
		return nil, p.errorf("unknown field %s.%s", rs.model, fieldName)
	}
	rows, err := p.ctx.env.Model(rs.model).Browse(rs.ids...).Read(fieldName)
	if err != nil {
		return nil, err
	}
	values := make([]any, 0, len(rows))
	for _, row := range rows {
		values = append(values, row[fieldName])
	}
	relation, _ := info["relation"].(string)
	if relation != "" {
		return evalRecordSet{model: relation, ids: evalIDsFromValues(values)}, nil
	}
	if len(values) == 0 {
		return false, nil
	}
	if len(values) == 1 {
		return values[0], nil
	}
	return values, nil
}

func (p *evalParser) resolveEvalRef(raw string) (evalRef, error) {
	if p.ctx.resolveRecord != nil {
		return p.ctx.resolveRecord(raw)
	}
	if p.ctx.resolveID == nil {
		return evalRef{}, p.errorf("ref resolver is not available")
	}
	id, err := p.ctx.resolveID(raw)
	if err != nil {
		return evalRef{}, err
	}
	return evalRef{ID: id}, nil
}

func (p *evalParser) parseCallArgs() ([]any, error) {
	var args []any
	open := p.pos - 1
	if open < 0 || p.input[open] != '(' {
		return nil, p.errorf("call parser missing opening paren")
	}
	end := matchingParenIndex(p.input, open)
	if end < 0 {
		return nil, p.errorf("unterminated call")
	}
	body := strings.TrimSpace(p.input[p.pos:end])
	if body == "" {
		p.pos = end + 1
		return args, nil
	}
	for _, part := range splitTopLevelComma(body) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.HasPrefix(part, "**") {
			value, err := parseEvalWithContext(strings.TrimSpace(part[2:]), p.ctx)
			if err != nil {
				return nil, err
			}
			args = append(args, evalKeywordExpansion{Value: value})
			continue
		}
		if strings.HasPrefix(part, "lambda ") {
			lambda, err := parseEvalLambda(part)
			if err != nil {
				return nil, err
			}
			args = append(args, lambda)
			continue
		}
		arg, err := parseEvalWithContext(part, p.ctx)
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
	}
	p.pos = end + 1
	return args, nil
}

func parseEvalLambda(raw string) (evalLambda, error) {
	body := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "lambda "))
	name, expr, ok := splitTopLevelPair(body, ':')
	if !ok {
		return evalLambda{}, fmt.Errorf("lambda requires ':'")
	}
	name = strings.TrimSpace(name)
	if !isSimpleIdentifier(name) {
		return evalLambda{}, fmt.Errorf("lambda argument must be a simple identifier")
	}
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return evalLambda{}, fmt.Errorf("lambda expression is empty")
	}
	return evalLambda{Name: name, Expr: expr}, nil
}

func (p *evalParser) parseCommandCall(ident string, args []any) (any, error) {
	switch ident {
	case "Command.create":
		if len(args) != 1 {
			return nil, p.errorf("Command.create requires 1 argument")
		}
		return evalTuple{int64(0), int64(0), args[0]}, nil
	case "Command.update":
		if len(args) != 2 {
			return nil, p.errorf("Command.update requires 2 arguments")
		}
		return evalTuple{int64(1), args[0], args[1]}, nil
	case "Command.delete":
		if len(args) != 1 {
			return nil, p.errorf("Command.delete requires 1 argument")
		}
		return evalTuple{int64(2), args[0]}, nil
	case "Command.unlink":
		if len(args) != 1 {
			return nil, p.errorf("Command.unlink requires 1 argument")
		}
		return evalTuple{int64(3), args[0]}, nil
	case "Command.link":
		if len(args) != 1 {
			return nil, p.errorf("Command.link requires 1 argument")
		}
		return evalTuple{int64(4), args[0]}, nil
	case "Command.clear":
		if len(args) != 0 {
			return nil, p.errorf("Command.clear requires no arguments")
		}
		return evalTuple{int64(5)}, nil
	case "Command.set":
		if len(args) != 1 {
			return nil, p.errorf("Command.set requires 1 argument")
		}
		return evalTuple{int64(6), int64(0), args[0]}, nil
	default:
		return nil, p.errorf("unsupported function %s", ident)
	}
}

func isFalseRefFallback(value any) bool {
	switch typed := value.(type) {
	case bool:
		return !typed
	case evalKeywordArg:
		if typed.Name != "raise_if_not_found" {
			return false
		}
		flag, ok := typed.Value.(bool)
		return ok && !flag
	default:
		return false
	}
}

func finalizeEvalObject(value any) any {
	switch typed := value.(type) {
	case evalRecordSet:
		if len(typed.ids) == 0 {
			return false
		}
		if len(typed.ids) == 1 {
			return typed.ids[0]
		}
		return idsAsEvalList(typed.ids)
	default:
		return value
	}
}

func idsAsEvalList(ids []int64) []any {
	out := make([]any, 0, len(ids))
	for _, id := range ids {
		out = append(out, id)
	}
	return out
}

func idsFromEvalArg(value any) ([]int64, error) {
	if id, ok := int64Value(value); ok {
		return []int64{id}, nil
	}
	switch typed := value.(type) {
	case []int64:
		return append([]int64(nil), typed...), nil
	case []any:
		ids, ok := int64List(typed)
		if !ok {
			return nil, fmt.Errorf("record id list must contain ints")
		}
		return ids, nil
	case evalTuple:
		ids, ok := int64List([]any(typed))
		if !ok {
			return nil, fmt.Errorf("record id list must contain ints")
		}
		return ids, nil
	case evalRecordSet:
		return append([]int64(nil), typed.ids...), nil
	default:
		return nil, fmt.Errorf("record ids must be int or list")
	}
}

func stringListFromEval(value any) ([]string, error) {
	switch typed := value.(type) {
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("list must contain strings")
			}
			out = append(out, text)
		}
		return out, nil
	case evalTuple:
		return stringListFromEval([]any(typed))
	case []string:
		return append([]string(nil), typed...), nil
	default:
		return nil, fmt.Errorf("value must be a string list")
	}
}

func evalIDsFromValues(values []any) []int64 {
	ids := []int64{}
	for _, value := range values {
		if id, ok := int64Value(value); ok {
			if id > 0 {
				ids = append(ids, id)
			}
			continue
		}
		switch typed := value.(type) {
		case []int64:
			ids = append(ids, typed...)
		case []any:
			if parsed, ok := int64List(typed); ok {
				ids = append(ids, parsed...)
			}
		case evalTuple:
			if parsed, ok := int64List([]any(typed)); ok {
				ids = append(ids, parsed...)
			}
		case evalRecordSet:
			ids = append(ids, typed.ids...)
		}
	}
	return ids
}

func searchOptionsFromEvalArgs(args []any) (record.SearchOptions, error) {
	var opts record.SearchOptions
	for _, arg := range args {
		kw, ok := arg.(evalKeywordArg)
		if !ok {
			return opts, fmt.Errorf("search options must be keyword arguments")
		}
		switch kw.Name {
		case "limit":
			id, ok := int64Value(kw.Value)
			if !ok {
				return opts, fmt.Errorf("search limit must be int")
			}
			opts.Limit = int(id)
		case "offset":
			id, ok := int64Value(kw.Value)
			if !ok {
				return opts, fmt.Errorf("search offset must be int")
			}
			opts.Offset = int(id)
		case "order":
			order, ok := kw.Value.(string)
			if !ok {
				return opts, fmt.Errorf("search order must be string")
			}
			opts.Order = order
		default:
			return opts, fmt.Errorf("unsupported search option %s", kw.Name)
		}
	}
	return opts, nil
}

func (p *evalParser) skipSpace() {
	for !p.eof() {
		switch p.peek() {
		case ' ', '\t', '\n', '\r':
			p.pos++
		default:
			return
		}
	}
}

func (p *evalParser) consume(ch byte) bool {
	if p.eof() || p.peek() != ch {
		return false
	}
	p.pos++
	return true
}

func (p *evalParser) peek() byte {
	return p.input[p.pos]
}

func (p *evalParser) eof() bool {
	return p.pos >= len(p.input)
}

func (p *evalParser) errorf(format string, args ...any) error {
	return fmt.Errorf(format+" at byte %d", append(args, p.pos)...)
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isIdentStart(ch byte) bool {
	return (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || ch == '_'
}

func isIdentPart(ch byte) bool {
	return isIdentStart(ch) || isDigit(ch) || ch == '.'
}

func normalizeEvalTuples(value any) any {
	switch typed := value.(type) {
	case evalTuple:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, normalizeEvalTuples(item))
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, normalizeEvalTuples(item))
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = normalizeEvalTuples(item)
		}
		return out
	default:
		return value
	}
}

func x2manyIDs(value any) ([]int64, bool, error) {
	switch typed := value.(type) {
	case evalTuple:
		ids, err := applyX2ManyCommand(nil, []any(typed))
		return ids, true, err
	case []any:
		if len(typed) == 0 {
			return []int64{}, true, nil
		}
		if containsTuple(typed) {
			ids := []int64{}
			for _, item := range typed {
				command, ok := item.(evalTuple)
				if !ok {
					return nil, true, fmt.Errorf("mixed x2many command list")
				}
				var err error
				ids, err = applyX2ManyCommand(ids, []any(command))
				if err != nil {
					return nil, true, err
				}
			}
			return ids, true, nil
		}
		ids, ok := int64List(typed)
		return ids, ok, nil
	default:
		return nil, false, nil
	}
}

func containsTuple(items []any) bool {
	for _, item := range items {
		if _, ok := item.(evalTuple); ok {
			return true
		}
	}
	return false
}

func applyX2ManyCommand(ids []int64, command []any) ([]int64, error) {
	if len(command) == 0 {
		return nil, fmt.Errorf("empty x2many command")
	}
	code, ok := int64Value(command[0])
	if !ok {
		return nil, fmt.Errorf("x2many command code must be int")
	}
	switch code {
	case 0:
		return nil, fmt.Errorf("x2many create command requires relation create support")
	case 1:
		return nil, fmt.Errorf("x2many update command requires relation write support")
	case 2, 3:
		if len(command) < 2 {
			return nil, fmt.Errorf("x2many unlink command requires id")
		}
		id, ok := int64Value(command[1])
		if !ok {
			return nil, fmt.Errorf("x2many unlink id must be int")
		}
		return removeInt64(ids, id), nil
	case 4:
		if len(command) < 2 {
			return nil, fmt.Errorf("x2many link command requires id")
		}
		id, ok := int64Value(command[1])
		if !ok {
			return nil, fmt.Errorf("x2many link id must be int")
		}
		if !containsInt64(ids, id) {
			ids = append(ids, id)
		}
		return ids, nil
	case 5:
		return []int64{}, nil
	case 6:
		if len(command) < 3 {
			return nil, fmt.Errorf("x2many set command requires id list")
		}
		next, err := idsFromValue(command[2])
		if err != nil {
			return nil, err
		}
		return next, nil
	default:
		return nil, fmt.Errorf("unsupported x2many command %d", code)
	}
}

func idsFromValue(value any) ([]int64, error) {
	switch typed := value.(type) {
	case []int64:
		return append([]int64(nil), typed...), nil
	case []any:
		ids, ok := int64List(typed)
		if !ok {
			return nil, fmt.Errorf("x2many id list must contain ints")
		}
		return ids, nil
	case evalTuple:
		ids, ok := int64List([]any(typed))
		if !ok {
			return nil, fmt.Errorf("x2many id list must contain ints")
		}
		return ids, nil
	default:
		return nil, fmt.Errorf("x2many id list must be list")
	}
}

func int64List(items []any) ([]int64, bool) {
	out := make([]int64, 0, len(items))
	for _, item := range items {
		id, ok := int64Value(item)
		if !ok {
			return nil, false
		}
		out = append(out, id)
	}
	return out, true
}

func int64Value(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int8:
		return int64(typed), true
	case int16:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case uint:
		return int64(typed), true
	case uint8:
		return int64(typed), true
	case uint16:
		return int64(typed), true
	case uint32:
		return int64(typed), true
	case uint64:
		if typed > uint64(^uint64(0)>>1) {
			return 0, false
		}
		return int64(typed), true
	case float64:
		if typed != float64(int64(typed)) {
			return 0, false
		}
		return int64(typed), true
	default:
		return 0, false
	}
}

func containsInt64(ids []int64, id int64) bool {
	for _, item := range ids {
		if item == id {
			return true
		}
	}
	return false
}

func removeInt64(ids []int64, id int64) []int64 {
	out := ids[:0]
	for _, item := range ids {
		if item != id {
			out = append(out, item)
		}
	}
	return out
}
