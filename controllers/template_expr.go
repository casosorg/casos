package controllers

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

// The template placeholder language.
//
// sealos evaluates `${{ ... }}` with a sandboxed JavaScript interpreter. Almost
// nothing in the market needs that: across the published templates the whole
// vocabulary is a dotted lookup, a string literal, a comparison, and the two
// functions random() and base64(). This is that vocabulary, evaluated directly,
// which keeps a template from being able to run code on the way in.

var (
	placeholderPattern = regexp.MustCompile(`\$\{\{\s*(.*?)\s*\}\}`)
	conditionalPattern = regexp.MustCompile(`^\s*\$\{\{\s*(if|elif|else|endif)\((.*?)\)\s*\}\}\s*$`)
)

const randomAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

// templateData is everything a placeholder may refer to: the template's own
// defaults, whatever the form was filled in with, and the platform's own
// values (SEALOS_NAMESPACE and friends), which are referred to unqualified.
type templateData struct {
	Defaults map[string]string
	Inputs   map[string]string
	Env      map[string]string
}

func (d templateData) lookup(path []string) (string, bool) {
	if len(path) == 2 {
		switch path[0] {
		case "defaults":
			value, ok := d.Defaults[path[1]]
			return value, ok
		case "inputs":
			value, ok := d.Inputs[path[1]]
			return value, ok
		}
	}
	if len(path) == 1 {
		if value, ok := d.Env[path[0]]; ok {
			return value, true
		}
		if value, ok := d.Defaults[path[0]]; ok {
			return value, true
		}
		if value, ok := d.Inputs[path[0]]; ok {
			return value, true
		}
	}
	return "", false
}

func randomString(length int) string {
	if length <= 0 {
		length = 8
	}
	if length > 64 {
		length = 64
	}
	limit := big.NewInt(int64(len(randomAlphabet)))
	builder := strings.Builder{}
	for i := 0; i < length; i++ {
		index, err := rand.Int(rand.Reader, limit)
		if err != nil {
			// A name that cannot be random is worse than one that is short.
			return strings.Repeat("x", length)
		}
		builder.WriteByte(randomAlphabet[index.Int64()])
	}
	return builder.String()
}

// renderTemplateText resolves the conditional blocks first and the
// placeholders second, which is the order sealos uses: a placeholder inside a
// branch that was cut must never be evaluated.
func renderTemplateText(text string, data templateData) string {
	return substitutePlaceholders(applyConditionals(text, data), data)
}

func substitutePlaceholders(text string, data templateData) string {
	return placeholderPattern.ReplaceAllStringFunc(text, func(match string) string {
		expression := placeholderPattern.FindStringSubmatch(match)[1]
		value, err := evaluateTemplateExpression(expression, data)
		if err != nil {
			// An expression nobody can evaluate resolves to nothing, the same
			// way it does in sealos; failing the deploy over a stray
			// placeholder would make one bad template break its whole app.
			return ""
		}
		return valueToString(value)
	})
}

// applyConditionals keeps the lines inside whichever branch of an
// if/elif/else/endif block is true, and drops the directive lines themselves.
func applyConditionals(text string, data templateData) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	kept := make([]string, 0, len(lines))

	// Each open block remembers whether the branch being read is live, and
	// whether any earlier branch already matched.
	type block struct {
		active   bool
		taken    bool
		enclosed bool
	}
	var stack []block

	live := func() bool {
		for _, item := range stack {
			if !item.active || !item.enclosed {
				return false
			}
		}
		return true
	}

	for _, line := range lines {
		directive := conditionalPattern.FindStringSubmatch(line)
		if directive == nil {
			if live() {
				kept = append(kept, line)
			}
			continue
		}

		keyword, expression := directive[1], directive[2]
		switch keyword {
		case "if":
			enclosed := live()
			result := enclosed && truthy(evaluateOrEmpty(expression, data))
			stack = append(stack, block{active: result, taken: result, enclosed: enclosed})
		case "elif", "else":
			if len(stack) == 0 {
				continue
			}
			current := &stack[len(stack)-1]
			if current.taken {
				current.active = false
				continue
			}
			result := current.enclosed
			if keyword == "elif" {
				result = result && truthy(evaluateOrEmpty(expression, data))
			}
			current.active = result
			current.taken = current.taken || result
		case "endif":
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}

	return strings.Join(kept, "\n")
}

func evaluateOrEmpty(expression string, data templateData) any {
	value, err := evaluateTemplateExpression(expression, data)
	if err != nil {
		return nil
	}
	return value
}

func truthy(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case float64:
		return typed != 0
	case string:
		return typed != "" && typed != "false"
	}
	return false
}

func valueToString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case bool:
		return strconv.FormatBool(typed)
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case string:
		return typed
	}
	return fmt.Sprintf("%v", value)
}

// --- the expression parser -------------------------------------------------

type exprToken struct {
	kind  string // "ident", "string", "number", "op"
	value string
}

func tokenizeExpression(input string) ([]exprToken, error) {
	tokens := []exprToken{}
	runes := []rune(input)
	for index := 0; index < len(runes); {
		char := runes[index]
		switch {
		case char == ' ' || char == '\t' || char == '\n':
			index++
		case char == '\'' || char == '"':
			quote := char
			index++
			start := index
			for index < len(runes) && runes[index] != quote {
				index++
			}
			if index >= len(runes) {
				return nil, fmt.Errorf("unterminated string")
			}
			tokens = append(tokens, exprToken{kind: "string", value: string(runes[start:index])})
			index++
		case char >= '0' && char <= '9':
			start := index
			for index < len(runes) && (runes[index] == '.' || (runes[index] >= '0' && runes[index] <= '9')) {
				index++
			}
			tokens = append(tokens, exprToken{kind: "number", value: string(runes[start:index])})
		case isIdentifierRune(char):
			start := index
			for index < len(runes) && (isIdentifierRune(runes[index]) || (runes[index] >= '0' && runes[index] <= '9')) {
				index++
			}
			tokens = append(tokens, exprToken{kind: "ident", value: string(runes[start:index])})
		default:
			operators := []string{"===", "!==", "==", "!=", ">=", "<=", "&&", "||", "!", ">", "<", "+", "(", ")", ",", "."}
			rest := string(runes[index:])
			matched := ""
			for _, candidate := range operators {
				if strings.HasPrefix(rest, candidate) {
					matched = candidate
					break
				}
			}
			if matched == "" {
				return nil, fmt.Errorf("unexpected character %q", string(char))
			}
			tokens = append(tokens, exprToken{kind: "op", value: matched})
			index += len([]rune(matched))
		}
	}
	return tokens, nil
}

func isIdentifierRune(char rune) bool {
	return char == '_' || char == '$' || char == '-' ||
		(char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z')
}

type exprParser struct {
	tokens []exprToken
	pos    int
	data   templateData
}

func evaluateTemplateExpression(expression string, data templateData) (any, error) {
	tokens, err := tokenizeExpression(expression)
	if err != nil {
		return nil, err
	}
	if len(tokens) == 0 {
		return nil, nil
	}
	parser := &exprParser{tokens: tokens, data: data}
	value, err := parser.parseOr()
	if err != nil {
		return nil, err
	}
	if parser.pos != len(parser.tokens) {
		return nil, fmt.Errorf("unexpected %q", parser.tokens[parser.pos].value)
	}
	return value, nil
}

func (p *exprParser) peek() (exprToken, bool) {
	if p.pos >= len(p.tokens) {
		return exprToken{}, false
	}
	return p.tokens[p.pos], true
}

func (p *exprParser) acceptOp(values ...string) (string, bool) {
	token, ok := p.peek()
	if !ok || token.kind != "op" {
		return "", false
	}
	for _, value := range values {
		if token.value == value {
			p.pos++
			return value, true
		}
	}
	return "", false
}

func (p *exprParser) parseOr() (any, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for {
		if _, ok := p.acceptOp("||"); !ok {
			return left, nil
		}
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		if truthy(left) {
			continue
		}
		left = right
	}
}

func (p *exprParser) parseAnd() (any, error) {
	left, err := p.parseComparison()
	if err != nil {
		return nil, err
	}
	for {
		if _, ok := p.acceptOp("&&"); !ok {
			return left, nil
		}
		right, err := p.parseComparison()
		if err != nil {
			return nil, err
		}
		if !truthy(left) {
			continue
		}
		left = right
	}
}

func (p *exprParser) parseComparison() (any, error) {
	left, err := p.parseSum()
	if err != nil {
		return nil, err
	}
	operator, ok := p.acceptOp("===", "!==", "==", "!=", ">=", "<=", ">", "<")
	if !ok {
		return left, nil
	}
	right, err := p.parseSum()
	if err != nil {
		return nil, err
	}
	return compareValues(operator, left, right)
}

func compareValues(operator string, left, right any) (any, error) {
	leftNumber, leftIsNumber := toNumber(left)
	rightNumber, rightIsNumber := toNumber(right)
	if leftIsNumber && rightIsNumber {
		switch operator {
		case ">=":
			return leftNumber >= rightNumber, nil
		case "<=":
			return leftNumber <= rightNumber, nil
		case ">":
			return leftNumber > rightNumber, nil
		case "<":
			return leftNumber < rightNumber, nil
		}
	}

	leftText, rightText := valueToString(left), valueToString(right)
	switch operator {
	case "===", "==":
		return leftText == rightText, nil
	case "!==", "!=":
		return leftText != rightText, nil
	case ">":
		return leftText > rightText, nil
	case "<":
		return leftText < rightText, nil
	case ">=":
		return leftText >= rightText, nil
	case "<=":
		return leftText <= rightText, nil
	}
	return nil, fmt.Errorf("unknown operator %q", operator)
}

func toNumber(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case string:
		parsed, err := strconv.ParseFloat(typed, 64)
		return parsed, err == nil
	}
	return 0, false
}

func (p *exprParser) parseSum() (any, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		if _, ok := p.acceptOp("+"); !ok {
			return left, nil
		}
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		leftNumber, leftIsNumber := toNumber(left)
		rightNumber, rightIsNumber := toNumber(right)
		if leftIsNumber && rightIsNumber {
			left = leftNumber + rightNumber
			continue
		}
		left = valueToString(left) + valueToString(right)
	}
}

func (p *exprParser) parseUnary() (any, error) {
	if _, ok := p.acceptOp("!"); ok {
		value, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return !truthy(value), nil
	}
	return p.parsePrimary()
}

func (p *exprParser) parsePrimary() (any, error) {
	token, ok := p.peek()
	if !ok {
		return nil, fmt.Errorf("unexpected end of expression")
	}

	switch token.kind {
	case "string":
		p.pos++
		return token.value, nil
	case "number":
		p.pos++
		parsed, err := strconv.ParseFloat(token.value, 64)
		if err != nil {
			return nil, err
		}
		return parsed, nil
	case "op":
		if token.value == "(" {
			p.pos++
			value, err := p.parseOr()
			if err != nil {
				return nil, err
			}
			if _, ok := p.acceptOp(")"); !ok {
				return nil, fmt.Errorf("missing closing parenthesis")
			}
			return value, nil
		}
		return nil, fmt.Errorf("unexpected %q", token.value)
	}

	// An identifier: a function call, a dotted lookup, or a bare name.
	p.pos++
	name := token.value
	if _, ok := p.acceptOp("("); ok {
		arguments := []any{}
		if _, closed := p.acceptOp(")"); !closed {
			for {
				argument, err := p.parseOr()
				if err != nil {
					return nil, err
				}
				arguments = append(arguments, argument)
				if _, more := p.acceptOp(","); more {
					continue
				}
				if _, closed := p.acceptOp(")"); closed {
					break
				}
				return nil, fmt.Errorf("missing closing parenthesis after %s(", name)
			}
		}
		return callTemplateFunction(name, arguments)
	}

	path := []string{name}
	for {
		if _, ok := p.acceptOp("."); !ok {
			break
		}
		next, ok := p.peek()
		if !ok || next.kind != "ident" {
			return nil, fmt.Errorf("expected a name after %q", strings.Join(path, "."))
		}
		p.pos++
		path = append(path, next.value)
	}

	switch strings.Join(path, ".") {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	value, _ := p.data.lookup(path)
	return value, nil
}

func callTemplateFunction(name string, arguments []any) (any, error) {
	switch name {
	case "random":
		length := 8
		if len(arguments) > 0 {
			if parsed, ok := toNumber(arguments[0]); ok {
				length = int(parsed)
			}
		}
		return randomString(length), nil
	case "base64":
		if len(arguments) == 0 {
			return "", nil
		}
		return base64.StdEncoding.EncodeToString([]byte(valueToString(arguments[0]))), nil
	case "base64Decode":
		if len(arguments) == 0 {
			return "", nil
		}
		decoded, err := base64.StdEncoding.DecodeString(valueToString(arguments[0]))
		if err != nil {
			return "", nil
		}
		return string(decoded), nil
	}
	return nil, fmt.Errorf("unknown function %q", name)
}
