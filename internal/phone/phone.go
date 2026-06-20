package phone

import (
	"strconv"
	"strings"

	"github.com/nyaruka/phonenumbers"
)

type Country struct {
	Code      string
	PhoneCode int64
}

func NormalizeE164(value string, country Country) string {
	if formatted, ok := FormatE164(value, country); ok {
		return formatted
	}
	return fallbackE164(value, country)
}

func FormatE164(value string, country Country) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	region := strings.ToUpper(strings.TrimSpace(country.Code))
	for _, candidate := range parseCandidates(value) {
		number, err := phonenumbers.Parse(candidate, region)
		if err != nil {
			continue
		}
		if !phonenumbers.IsPossibleNumber(number) || !phonenumbers.IsValidNumber(number) {
			continue
		}
		formatted := phonenumbers.Format(number, phonenumbers.E164)
		if formatted == "" || formatted == "+" {
			continue
		}
		return formatted, true
	}
	return "", false
}

func parseCandidates(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	candidates := []string{value}
	compact := compactPhone(value)
	if strings.HasPrefix(compact, "00") && len(compact) > 2 {
		candidates = append(candidates, "+"+strings.TrimPrefix(compact, "00"))
	}
	if !strings.HasPrefix(compact, "+") && compact != "" {
		candidates = append(candidates, "+"+compact)
	}
	return uniqueNonEmpty(candidates)
}

func fallbackE164(value string, country Country) string {
	compact := compactPhone(value)
	if compact == "" || compact == "+" {
		return ""
	}
	if strings.HasPrefix(compact, "+") {
		digits := digitsOnly(compact)
		if digits == "" {
			return ""
		}
		return "+" + digits
	}
	if strings.HasPrefix(compact, "00") && len(compact) > 2 {
		digits := strings.TrimPrefix(compact, "00")
		if digits == "" {
			return ""
		}
		return "+" + digits
	}
	if country.PhoneCode > 0 {
		callingCode := strconv.FormatInt(country.PhoneCode, 10)
		if strings.HasPrefix(compact, callingCode) && len(compact) > len(callingCode) {
			return "+" + compact
		}
		local := strings.TrimLeft(compact, "0")
		if local == "" {
			return ""
		}
		return "+" + callingCode + local
	}
	return compact
}

func compactPhone(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	for index, r := range value {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '+' && index == 0:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func digitsOnly(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func uniqueNonEmpty(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
