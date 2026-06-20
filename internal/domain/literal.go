package domain

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func ParseLiteral(text string) (Node, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return And(), nil
	}
	var value any
	if err := json.Unmarshal([]byte(text), &value); err == nil {
		return parseLiteralDomainValue(value)
	}
	parsed, err := newLiteralParser(text).parse()
	if err != nil {
		return Node{}, err
	}
	return parseLiteralDomainValue(parsed)
}

func ParseLiteralValue(text string) (any, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return []any{}, nil
	}
	var value any
	if err := json.Unmarshal([]byte(text), &value); err == nil {
		return value, nil
	}
	return newLiteralParser(text).parse()
}

func parseLiteralDomainValue(value any) (Node, error) {
	if typed, ok := value.(bool); ok {
		return Bool(typed), nil
	}
	return Parse(value)
}

type literalParser struct {
	input string
	pos   int
}

func newLiteralParser(input string) *literalParser {
	return &literalParser{input: input}
}

func (p *literalParser) parse() (any, error) {
	p.skipSpace()
	value, err := p.parseValue()
	if err != nil {
		return nil, err
	}
	p.skipSpace()
	if !p.eof() {
		return nil, p.errorf("unexpected trailing input")
	}
	return value, nil
}

func (p *literalParser) parseValue() (any, error) {
	p.skipSpace()
	if p.eof() {
		return nil, p.errorf("unexpected end of literal")
	}
	switch ch := p.peek(); ch {
	case '[':
		return p.parseSequence('[', ']')
	case '(':
		return p.parseSequence('(', ')')
	case '\'', '"':
		return p.parseString()
	case '-', '+':
		return p.parseNumber()
	default:
		if isLiteralDigit(ch) {
			return p.parseNumber()
		}
		if isLiteralIdentStart(ch) {
			return p.parseIdentifier()
		}
		return nil, p.errorf("unexpected character %q", ch)
	}
}

func (p *literalParser) parseSequence(open byte, close byte) ([]any, error) {
	if !p.consume(open) {
		return nil, p.errorf("expected %q", open)
	}
	out := []any{}
	for {
		p.skipSpace()
		if p.consume(close) {
			return out, nil
		}
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		out = append(out, value)
		p.skipSpace()
		if p.consume(close) {
			return out, nil
		}
		if !p.consume(',') {
			return nil, p.errorf("expected ',' or %q", close)
		}
	}
}

func (p *literalParser) parseString() (string, error) {
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

func (p *literalParser) parseNumber() (any, error) {
	start := p.pos
	if p.peek() == '-' || p.peek() == '+' {
		p.pos++
	}
	digits := 0
	for !p.eof() && isLiteralDigit(p.peek()) {
		p.pos++
		digits++
	}
	isFloat := false
	if !p.eof() && p.peek() == '.' {
		isFloat = true
		p.pos++
		for !p.eof() && isLiteralDigit(p.peek()) {
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
		for !p.eof() && isLiteralDigit(p.peek()) {
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

func (p *literalParser) parseIdentifier() (any, error) {
	start := p.pos
	p.pos++
	for !p.eof() && isLiteralIdentPart(p.peek()) {
		p.pos++
	}
	ident := p.input[start:p.pos]
	switch ident {
	case "True", "true":
		return true, nil
	case "False", "false":
		return false, nil
	case "None", "none", "null":
		return nil, nil
	default:
		return ident, nil
	}
}

func (p *literalParser) skipSpace() {
	for !p.eof() {
		switch p.peek() {
		case ' ', '\n', '\r', '\t':
			p.pos++
		default:
			return
		}
	}
}

func (p *literalParser) consume(ch byte) bool {
	if p.eof() || p.peek() != ch {
		return false
	}
	p.pos++
	return true
}

func (p *literalParser) eof() bool {
	return p.pos >= len(p.input)
}

func (p *literalParser) peek() byte {
	return p.input[p.pos]
}

func (p *literalParser) errorf(format string, args ...any) error {
	return fmt.Errorf("literal domain at byte %d: %s", p.pos, fmt.Sprintf(format, args...))
}

func isLiteralDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}

func isLiteralIdentStart(ch byte) bool {
	return (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || ch == '_'
}

func isLiteralIdentPart(ch byte) bool {
	return isLiteralIdentStart(ch) || isLiteralDigit(ch) || ch == '.'
}
