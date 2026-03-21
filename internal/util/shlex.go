package util

import (
	"fmt"
	"strings"
	"unicode"
)

func SplitLine(line string) ([]string, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, nil
	}

	var (
		tokens        []string
		current       strings.Builder
		inSingleQuote bool
		inDoubleQuote bool
		escaped       bool
	)

	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}

	for _, r := range line {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\' && !inSingleQuote:
			escaped = true
		case r == '\'' && !inDoubleQuote:
			inSingleQuote = !inSingleQuote
		case r == '"' && !inSingleQuote:
			inDoubleQuote = !inDoubleQuote
		case unicode.IsSpace(r) && !inSingleQuote && !inDoubleQuote:
			flush()
		default:
			current.WriteRune(r)
		}
	}

	if escaped || inSingleQuote || inDoubleQuote {
		return nil, fmt.Errorf("unterminated quoted string")
	}

	flush()
	return tokens, nil
}

func ContainsForbiddenOperator(line string) bool {
	var inSingle, inDouble, escaped bool

	for _, r := range line {
		switch {
		case escaped:
			escaped = false
		case r == '\\' && !inSingle:
			escaped = true
		case r == '\'' && !inDouble:
			inSingle = !inSingle
		case r == '"' && !inSingle:
			inDouble = !inDouble
		case !inSingle && !inDouble && strings.ContainsRune("|;&><", r):
			return true
		}
	}

	return false
}
