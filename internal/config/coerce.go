package config

import (
	"fmt"
	"strconv"
)

// asFloat coerces a decoded YAML value to a float, warning and falling back
// when it is not one.
//
// A YAML document is untrusted input and temp_c = "hot" is a typo someone
// will make. A boolean is rejected explicitly: YAML's true is not a number,
// and temp_c = true becoming 1.0 is a silent, very confusing threshold. An
// integer is accepted and widened.
func asFloat(value any, def float64, where string, warnings *[]string) float64 {
	switch v := value.(type) {
	case bool:
		warn(warnings, where, "a number", "a boolean", def)
		return def
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case uint64:
		return float64(v)
	case float64:
		return v
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	warn(warnings, where, "a number", typeName(value), def)
	return def
}

// asInt coerces a decoded YAML value to an int, warning and falling back when
// it is not one. A float with an integer value is accepted (interval_ms: 250.0
// is unambiguous); a fractional float is not, because flooring it would be a
// guess.
func asInt(value any, def int, where string, warnings *[]string) int {
	switch v := value.(type) {
	case bool:
		warn(warnings, where, "an integer", "a boolean", def)
		return def
	case int:
		return v
	case int64:
		return int(v)
	case uint64:
		return int(v)
	case float64:
		if v == float64(int(v)) {
			return int(v)
		}
	case string:
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	warn(warnings, where, "an integer", typeName(value), def)
	return def
}

// asBool coerces a decoded YAML value to a bool. Only a real boolean is
// accepted; a string "yes" is a value someone typed and its intent is not
// unambiguous enough to honour silently.
func asBool(value any, def bool, where string, warnings *[]string) bool {
	if b, ok := value.(bool); ok {
		return b
	}
	warn(warnings, where, "a boolean", typeName(value), def)
	return def
}

// asStr coerces a decoded YAML value to a string. A number is accepted,
// because process_filter = 42 is a plausible thing to type and its intent --
// filter on the text "42" -- is unambiguous. A bool is not: it would become
// "true", which no one meant as a filter or a name.
func asStr(value any, def, where string, warnings *[]string) string {
	switch v := value.(type) {
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64)
	}
	warn(warnings, where, "a string", typeName(value), def)
	return def
}

// asStrList coerces a decoded YAML value to a list of strings, warning about
// each bad element.
//
// A bare string is not accepted as a one-element list: devices = card0 is the
// kind of thing a user writes, and quietly reading it as [card0] would make it
// the only list-shaped key that accepts a bare scalar, so the next one they
// wrote would be rejected for no visible reason. Each bad element is reported
// against its own position so the message points at the entry that is wrong.
func asStrList(value any, where string, warnings *[]string) []string {
	items, ok := value.([]any)
	if !ok {
		warn(warnings, where, "a list of names", typeName(value), nil)
		return nil
	}
	var out []string
	for position, item := range items {
		switch v := item.(type) {
		case string:
			if text := trimSpace(v); text != "" {
				out = append(out, text)
			} else {
				*warnings = append(*warnings, fmt.Sprintf("%s[%d]: empty entry, ignoring it", where, position))
			}
		case int, int64, uint64, float64:
			out = append(out, fmt.Sprint(item))
		default:
			*warnings = append(*warnings, fmt.Sprintf("%s[%d]: expected a name, got %s, ignoring it", where, position, typeName(item)))
		}
	}
	return out
}

// asStrMap coerces a decoded YAML value to a string->string map. Keys and
// values are both stringified, so a bare integer key does the obvious thing
// rather than nothing. A value that is not a mapping is ignored wholesale.
func asStrMap(value any, where string, warnings *[]string) map[string]string {
	m, ok := value.(map[string]any)
	if !ok {
		if value != nil {
			warn(warnings, where, "a table", typeName(value), nil)
		}
		return map[string]string{}
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = asStr(v, k, where+" value", warnings)
	}
	return out
}

// typeName names the dynamic type of a decoded value for a warning, mirroring
// the reference's type(value).__name__.
func typeName(value any) string {
	switch value.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case int, int64, uint64:
		return "integer"
	case float64:
		return "float"
	case string:
		return "string"
	case []any:
		return "list"
	case map[string]any:
		return "table"
	default:
		return fmt.Sprintf("%T", value)
	}
}

// warn records a coercion fallback. The default is shown so the message says
// what will actually be used.
func warn(warnings *[]string, where, expected, got string, def any) {
	*warnings = append(*warnings, fmt.Sprintf("%s: expected %s, got %s, using %v", where, expected, got, def))
}
