// Package resolve implements reference substitution for workflow node inputs.
//
// It mirrors sim's executor/variables resolver: before a node executes, every
// string in its params is scanned for references which are replaced with
// values drawn from upstream block outputs, the active loop/parallel scope,
// workflow variables, and environment variables.
//
// Reference forms (aligned with sim's executor/constants.ts):
//
//	<blockName.field.nested>   upstream block output (by block display name or id)
//	<loop.index>               current loop iteration index (0-based)
//	<loop.item>                current forEach item
//	<parallel.index>           current parallel branch index
//	<parallel.currentItem>     current parallel branch item
//	<variable.name>            workflow-level variable
//	{{ENV_VAR}}                environment variable
//
// A reference that occupies the ENTIRE string resolves to the native typed
// value (number/object/array preserved). A reference embedded in surrounding
// text is stringified and spliced in.
package resolve

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Scope carries everything needed to resolve references for one node.
type Scope struct {
	// BlockOutputs maps block id -> that block's output object.
	BlockOutputs map[string]map[string]any
	// BlockNameToID maps a block's display name to its id (sim allows
	// references by human-readable name; ids are the fallback).
	BlockNameToID map[string]string
	// Loop is the active loop scope (nil when not inside a loop).
	Loop *LoopScope
	// Parallel is the active parallel branch scope (nil when not inside one).
	Parallel *ParallelScope
	// Variables are workflow-level variables.
	Variables map[string]any
	// Env are environment variables available to the workflow.
	Env map[string]string
}

// LoopScope is the per-iteration loop state visible to <loop.*> references.
type LoopScope struct {
	Index int
	Item  any
}

// ParallelScope is the per-branch parallel state visible to <parallel.*>.
type ParallelScope struct {
	Index       int
	CurrentItem any
}

// special reference prefixes that are NOT block references.
const (
	prefixLoop     = "loop"
	prefixParallel = "parallel"
	prefixVariable = "variable"
)

// angleRef matches <...> references. The body excludes < > to avoid greedy
// spanning across multiple references.
var angleRef = regexp.MustCompile(`<([^<>]+)>`)

// envRef matches {{NAME}} environment references.
var envRef = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)

// Params resolves every value in a params map (recursively for nested maps and
// slices). It never mutates the input.
func (s *Scope) Params(params map[string]any) map[string]any {
	out := make(map[string]any, len(params))
	for k, v := range params {
		out[k] = s.Value(v)
	}
	return out
}

// Value resolves a single value. Strings are scanned for references; maps and
// slices are walked recursively; other types pass through unchanged.
func (s *Scope) Value(v any) any {
	switch t := v.(type) {
	case string:
		return s.resolveString(t)
	case map[string]any:
		m := make(map[string]any, len(t))
		for k, vv := range t {
			m[k] = s.Value(vv)
		}
		return m
	case []any:
		arr := make([]any, len(t))
		for i, vv := range t {
			arr[i] = s.Value(vv)
		}
		return arr
	default:
		return v
	}
}

// resolveString applies env substitution then angle-reference substitution.
// When the whole string is exactly one angle reference, the native typed value
// is returned (not stringified).
func (s *Scope) resolveString(in string) any {
	// {{ENV}} substitution is always textual.
	in = envRef.ReplaceAllStringFunc(in, func(m string) string {
		name := envRef.FindStringSubmatch(m)[1]
		if val, ok := s.Env[name]; ok {
			return val
		}
		return m // leave unresolved env refs intact
	})

	// Whole-string single reference -> typed value.
	if loc := angleRef.FindStringIndex(in); loc != nil && loc[0] == 0 && loc[1] == len(in) {
		body := angleRef.FindStringSubmatch(in)[1]
		val, ok := s.resolveRef(body)
		if ok {
			return val
		}
		return in
	}

	// Embedded references -> stringified splice.
	return angleRef.ReplaceAllStringFunc(in, func(m string) string {
		body := angleRef.FindStringSubmatch(m)[1]
		val, ok := s.resolveRef(body)
		if !ok {
			return m // leave unresolved refs intact (matches sim)
		}
		return stringify(val)
	})
}

// resolveRef resolves one reference body (without the angle brackets).
func (s *Scope) resolveRef(body string) (any, bool) {
	parts := splitPath(body)
	if len(parts) == 0 {
		return nil, false
	}
	head, rest := parts[0], parts[1:]

	switch head {
	case prefixLoop:
		if s.Loop == nil {
			return nil, false
		}
		switch firstOr(rest, "") {
		case "index":
			return s.Loop.Index, true
		case "item":
			return navigate(s.Loop.Item, rest[1:]), true
		}
		return nil, false

	case prefixParallel:
		if s.Parallel == nil {
			return nil, false
		}
		switch firstOr(rest, "") {
		case "index":
			return s.Parallel.Index, true
		case "currentItem":
			return navigate(s.Parallel.CurrentItem, rest[1:]), true
		}
		return nil, false

	case prefixVariable:
		if len(rest) == 0 {
			return nil, false
		}
		val, ok := s.Variables[rest[0]]
		if !ok {
			return nil, false
		}
		return navigate(val, rest[1:]), true

	default:
		// Block reference: head is a block name or id.
		id := head
		if mapped, ok := s.BlockNameToID[head]; ok {
			id = mapped
		}
		out, ok := s.BlockOutputs[id]
		if !ok {
			return nil, false
		}
		return navigate(any(out), rest), true
	}
}

// navigate walks a dotted path into a value (maps by key, slices by index).
func navigate(v any, path []string) any {
	cur := v
	for _, p := range path {
		switch c := cur.(type) {
		case map[string]any:
			next, ok := c[p]
			if !ok {
				return nil
			}
			cur = next
		case []any:
			idx, err := strconv.Atoi(p)
			if err != nil || idx < 0 || idx >= len(c) {
				return nil
			}
			cur = c[idx]
		default:
			return nil
		}
	}
	return cur
}

// splitPath splits a reference body on dots, trimming whitespace.
func splitPath(body string) []string {
	raw := strings.Split(body, ".")
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		r = strings.TrimSpace(r)
		if r != "" {
			out = append(out, r)
		}
	}
	return out
}

func firstOr(s []string, def string) string {
	if len(s) == 0 {
		return def
	}
	return s[0]
}

// stringify renders a resolved value for embedding in a larger string.
// Scalars use their natural form; composite values use compact JSON.
func stringify(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'g', -1, 64)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}
