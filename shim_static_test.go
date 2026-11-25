package main_test

// R-14's lexical check over the tracked shim's text, and the seeded copies it
// must reject. The check guards against accidental edits to a trust-boundary
// script; it is not a shell parser and does not claim to catch every way of
// turning tracing or export on (R-14 says which forms it leaves to the runtime
// tests). It fails closed: syntax it does not understand is a violation.

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// shimTokenVar is the token variable the static check declares and tracks. The
// inherited-export test seeds a variable of this name.
const shimTokenVar = "_sgs_tok"

// Anchors into the tracked script, used to build seeded copies.
const (
	shimHandoffLine = `GH_TOKEN=$` + shimTokenVar + ` GH_CONFIG_DIR=$HOME/.config/sigil/gh XDG_DATA_HOME=$HOME/.local/share/sigil/gh-data GH_PROMPT_DISABLED=1 exec "$_sgs_real" "$@"` + "\n"
	shimCaptureLine = shimTokenVar + `=$(sigil-helper token --landing "$_sgs_repo" </dev/null) || _sgs_mint_rc=$?` + "\n"
	shimUnsetLine   = "unset BASHOPTS BASH_XTRACEFD GITHUB_TOKEN GH_HOST GH_ENTERPRISE_TOKEN GITHUB_ENTERPRISE_TOKEN GODEBUG SIGIL_GITHUB_API_URL\n"
	shimFirstStmt   = "set +x +v +a\n"
	// shimUnsetFCommand is the first command once the guard is removed; the
	// "moved to second" seed puts the guard directly after it, so it is second.
	shimUnsetFCommand = "unset -f command\n"
)

type shTok struct {
	op, nl bool
	text   string
}

func shLex(s string) ([]shTok, error) {
	var toks []shTok
	i, n := 0, len(s)
	for i < n {
		c := s[i]
		switch {
		case c == ' ' || c == '\t':
			i++
		case c == '\r':
			return nil, fmt.Errorf("carriage return in the script")
		case c == '\n':
			toks = append(toks, shTok{nl: true})
			i++
		case c == '#':
			for i < n && s[i] != '\n' {
				i++
			}
		case c == ';' || c == '&' || c == '|' || c == '(' || c == ')':
			j := i + 1
			if (c == ';' || c == '&' || c == '|') && j < n && s[j] == c {
				j++
			}
			toks = append(toks, shTok{op: true, text: s[i:j]})
			i = j
		default:
			j, err := shWordEnd(s, i)
			if err != nil {
				return nil, err
			}
			toks = append(toks, shTok{text: s[i:j]})
			i = j
		}
	}
	return toks, nil
}

// shWordEnd returns the index after the word starting at i. Quotes, $(...) and
// ${...} are consumed whole, so operators inside them do not split the word.
func shWordEnd(s string, i int) (int, error) {
	n := len(s)
	j := i
	for j < n {
		c := s[j]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == ';' || c == '|' || c == '(' || c == ')':
			return j, nil
		case c == '&':
			if j > i && (s[j-1] == '>' || s[j-1] == '<') {
				j++
				continue
			}
			return j, nil
		case c == '\r':
			return 0, fmt.Errorf("carriage return in the script")
		case c == '`':
			return 0, fmt.Errorf("backtick substitution is not supported by the check")
		case c == '\\':
			if j+1 < n && s[j+1] == '\n' {
				return 0, fmt.Errorf("line continuation is not supported by the check")
			}
			j += 2
		case c == '\'':
			k := strings.IndexByte(s[j+1:], '\'')
			if k < 0 {
				return 0, fmt.Errorf("unterminated single quote")
			}
			j += k + 2
		case c == '"':
			k, err := shSkipDouble(s, j+1)
			if err != nil {
				return 0, err
			}
			j = k
		case c == '$' && j+1 < n && s[j+1] == '(':
			k, err := shSkipParen(s, j+2)
			if err != nil {
				return 0, err
			}
			j = k
		case c == '$' && j+1 < n && s[j+1] == '{':
			k, err := shSkipBrace(s, j+2)
			if err != nil {
				return 0, err
			}
			j = k
		default:
			j++
		}
	}
	return j, nil
}

func shSkipDouble(s string, k int) (int, error) {
	n := len(s)
	for k < n {
		switch {
		case s[k] == '\\':
			k += 2
		case s[k] == '"':
			return k + 1, nil
		case s[k] == '`':
			return 0, fmt.Errorf("backtick substitution is not supported by the check")
		case s[k] == '$' && k+1 < n && s[k+1] == '(':
			e, err := shSkipParen(s, k+2)
			if err != nil {
				return 0, err
			}
			k = e
		case s[k] == '$' && k+1 < n && s[k+1] == '{':
			e, err := shSkipBrace(s, k+2)
			if err != nil {
				return 0, err
			}
			k = e
		default:
			k++
		}
	}
	return 0, fmt.Errorf("unterminated double quote")
}

func shSkipParen(s string, k int) (int, error) {
	n, depth := len(s), 1
	for k < n {
		switch s[k] {
		case '\\':
			k += 2
		case '\'':
			e := strings.IndexByte(s[k+1:], '\'')
			if e < 0 {
				return 0, fmt.Errorf("unterminated single quote")
			}
			k += e + 2
		case '"':
			e, err := shSkipDouble(s, k+1)
			if err != nil {
				return 0, err
			}
			k = e
		case '`':
			return 0, fmt.Errorf("backtick substitution is not supported by the check")
		case '(':
			depth++
			k++
		case ')':
			depth--
			k++
			if depth == 0 {
				return k, nil
			}
		default:
			k++
		}
	}
	return 0, fmt.Errorf("unterminated command substitution")
}

func shSkipBrace(s string, k int) (int, error) {
	n, depth := len(s), 1
	for k < n {
		switch s[k] {
		case '\\':
			k += 2
		case '\'':
			e := strings.IndexByte(s[k+1:], '\'')
			if e < 0 {
				return 0, fmt.Errorf("unterminated single quote")
			}
			k += e + 2
		case '"':
			e, err := shSkipDouble(s, k+1)
			if err != nil {
				return 0, err
			}
			k = e
		case '{':
			depth++
			k++
		case '}':
			depth--
			k++
			if depth == 0 {
				return k, nil
			}
		default:
			k++
		}
	}
	return 0, fmt.Errorf("unterminated parameter expansion")
}

// shCommands splits tokens into simple commands (runs of words between
// operators and newlines).
func shCommands(toks []shTok) [][]string {
	var cmds [][]string
	var cur []string
	flush := func() {
		if len(cur) > 0 {
			cmds = append(cmds, cur)
			cur = nil
		}
	}
	for _, t := range toks {
		if t.op || t.nl {
			flush()
		} else {
			cur = append(cur, t.text)
		}
	}
	flush()
	return cmds
}

var (
	shAssignRe  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\+?=`)
	shEnvRe     = regexp.MustCompile(`(^|[^A-Za-z0-9_.-])env($|[^A-Za-z0-9_.-])`)
	shPS4Re     = regexp.MustCompile(`(^|[^A-Za-z0-9_])PS4\+?=`)
	shGHTokenRe = regexp.MustCompile(`(^|[^A-Za-z0-9_])GH_TOKEN\+?=`)
	shNameRe    = regexp.MustCompile(`(^|[^A-Za-z0-9_])` + shimTokenVar + `($|[^A-Za-z0-9_])`)
	// The capture: one command substitution running sigil-helper and nothing
	// else, assigned to the tracked variable.
	shCaptureRe   = regexp.MustCompile(`^` + shimTokenVar + `=\$\(sigil-helper(?: +(?:[A-Za-z0-9_.-]+|"\$[A-Za-z_][A-Za-z0-9_]*"|</dev/null))*\)$`)
	shXVClusterRe = regexp.MustCompile(`^-[A-Za-z]*[xv]`)
)

var shReserved = map[string]bool{"if": true, "then": true, "else": true, "elif": true, "fi": true, "do": true, "done": true, "while": true, "until": true, "!": true, "{": true, "}": true, "time": true}

// shimStaticCheck returns the violations found in the shim's text; none means
// it passes.
func shimStaticCheck(text string) []string {
	var v []string
	first, _, _ := strings.Cut(text, "\n")
	if first != "#!/bin/sh" {
		v = append(v, fmt.Sprintf("shebang: first line is %q, want exactly #!/bin/sh", first))
	}
	toks, err := shLex(text)
	if err != nil {
		return append(v, "lexical: "+err.Error())
	}
	cmds := shCommands(toks)
	if len(cmds) == 0 || strings.Join(cmds[0], " ") != "set +x +v +a" {
		v = append(v, "first statement is not exactly `set +x +v +a`")
	}
	var captures, cases, handoffs int
	for _, raw := range cmds {
		words := raw
		for len(words) > 0 && shReserved[words[0]] {
			words = words[1:]
		}
		if len(words) == 0 {
			continue
		}
		k := 0
		for k < len(words) && shAssignRe.MatchString(words[k]) {
			k++
		}
		assigns, rest := words[:k], words[k:]
		cmdWord := ""
		var args []string
		if len(rest) > 0 {
			cmdWord, args = rest[0], rest[1:]
		}
		isAssign := func(w string) bool {
			for _, a := range assigns {
				if a == w {
					return true
				}
			}
			return false
		}
		handoff := cmdWord == "exec"
		for _, w := range words {
			if shEnvRe.MatchString(w) {
				v = append(v, "env invocation: "+w)
			}
			if shPS4Re.MatchString(w) {
				v = append(v, "PS4 assignment: "+w)
			}
			if shGHTokenRe.MatchString(w) {
				if !(handoff && isAssign(w) && (w == "GH_TOKEN=$"+shimTokenVar || w == `GH_TOKEN="$`+shimTokenVar+`"`)) {
					v = append(v, "GH_TOKEN assigned outside the exec hand-off: "+w)
				}
			}
			if !shNameRe.MatchString(w) {
				continue
			}
			switch {
			case handoff && isAssign(w) && (w == "GH_TOKEN=$"+shimTokenVar || w == `GH_TOKEN="$`+shimTokenVar+`"`):
				handoffs++
				if len(args) == 0 || args[0] != `"$_sgs_real"` {
					v = append(v, "the exec hand-off does not start \"$_sgs_real\"")
				}
			case cmdWord == "unset" && w == shimTokenVar:
			case len(words) == 1 && shCaptureRe.MatchString(w):
				captures++
			case cmdWord == "case" && len(args) >= 2 && args[1] == "in" && w == args[0] && (w == "$"+shimTokenVar || w == `"$`+shimTokenVar+`"`):
				cases++
			default:
				v = append(v, "tracked variable referenced outside its capture, validity case and hand-off: "+w)
			}
		}
		if cmdWord == "set" {
			for i, a := range args {
				if shXVClusterRe.MatchString(a) {
					v = append(v, "set enables tracing: "+strings.Join(rest, " "))
				}
				if a == "-o" && i+1 < len(args) && (args[i+1] == "xtrace" || args[i+1] == "verbose") {
					v = append(v, "set enables tracing: "+strings.Join(rest, " "))
				}
			}
		}
	}
	for name, got := range map[string]int{"capture": captures, "validity case": cases, "exec hand-off": handoffs} {
		if got != 1 {
			v = append(v, fmt.Sprintf("structure: expected exactly one %s of the tracked variable, found %d", name, got))
		}
	}
	return v
}

// R-14: the check passes the tracked script.
func TestShim_R14_StaticCheck_PassesTheTrackedScript(t *testing.T) {
	if v := shimStaticCheck(shimTrackedText(t)); len(v) != 0 {
		t.Errorf("the tracked shim fails its static check:\n  %s", strings.Join(v, "\n  "))
	}
}

// R-14: the check ignores comments and passes `unset GH_TOKEN`, which is not an
// assignment. These are the controls that show the check is not simply
// rejecting everything.
func TestShim_R14_StaticCheck_IgnoresCommentsAndUnsetOfGHToken(t *testing.T) {
	text := shimEdit(t, shimTrackedText(t),
		[2]string{shimUnsetLine, shimUnsetLine + "# set -x and env GH_TOKEN=x and PS4=y in a comment\nunset GH_TOKEN\n"})
	if v := shimStaticCheck(text); len(v) != 0 {
		t.Errorf("comments and `unset GH_TOKEN` should pass, got:\n  %s", strings.Join(v, "\n  "))
	}
}

type shimSeed struct {
	name  string
	edits [][2]string
	want  string // a substring of the violation the seed must produce
}

func shimSeeds() []shimSeed {
	afterUnset := func(extra string) [][2]string {
		return [][2]string{{shimUnsetLine, shimUnsetLine + extra}}
	}
	beforeHandoff := func(extra string) [][2]string {
		return [][2]string{{shimHandoffLine, extra + shimHandoffLine}}
	}
	tok := `"$` + shimTokenVar + `"`
	return []shimSeed{
		{"exec env GH_TOKEN=...", [][2]string{{shimHandoffLine, `exec env GH_TOKEN=` + tok + ` "$_sgs_real" "$@"` + "\n"}}, "env invocation"},
		{"exec /usr/bin/env GH_TOKEN=...", [][2]string{{shimHandoffLine, `exec /usr/bin/env GH_TOKEN=` + tok + ` "$_sgs_real" "$@"` + "\n"}}, "env invocation"},
		{"first statement moved to second", [][2]string{{"#!/bin/sh\n" + shimFirstStmt, "#!/bin/sh\n"}, {shimUnsetFCommand, shimUnsetFCommand + shimFirstStmt}}, "first statement"},
		{"first statement replaced by :", [][2]string{{"#!/bin/sh\n" + shimFirstStmt, "#!/bin/sh\n:\n"}}, "first statement"},
		{"first statement set +x +v without +a", [][2]string{{"#!/bin/sh\n" + shimFirstStmt, "#!/bin/sh\nset +x +v\n"}}, "first statement"},
		{"a later set -x", afterUnset("set -x\n"), "set enables tracing"},
		{"a later set -eux", afterUnset("set -eux\n"), "set enables tracing"},
		{"a later set -v", afterUnset("set -v\n"), "set enables tracing"},
		{"a later set -o xtrace", afterUnset("set -o xtrace\n"), "set enables tracing"},
		{"a later set -o verbose", afterUnset("set -o verbose\n"), "set enables tracing"},
		{"shebang with -ex", [][2]string{{"#!/bin/sh\n", "#!/bin/sh -ex\n"}}, "shebang"},
		{"shebang with -v", [][2]string{{"#!/bin/sh\n", "#!/bin/sh -v\n"}}, "shebang"},
		{"shebang /bin/bash", [][2]string{{"#!/bin/sh\n", "#!/bin/bash\n"}}, "shebang"},
		{"a PS4 assignment", afterUnset("PS4='+ '\n"), "PS4 assignment"},
		{"a standalone GH_TOKEN= assignment and an export before the exec", beforeHandoff("GH_TOKEN=$" + shimTokenVar + "\nexport GH_TOKEN\n"), "GH_TOKEN assigned outside"},
		{"the tracked variable referenced in another statement", afterUnset("scratch=$" + shimTokenVar + "\n"), "tracked variable referenced"},
		{"a print inside a validity case arm", [][2]string{{"*\"$_sgs_nl\"*) fail_recorded mint-failed mint-failed 'sigil-helper did not print exactly one token line'", "*\"$_sgs_nl\"*) printf '%s\\n' " + tok + " >&2; fail_recorded mint-failed mint-failed 'sigil-helper did not print exactly one token line'"}}, "tracked variable referenced"},
		{"a reference in an || branch after the capture", [][2]string{{shimCaptureLine, strings.TrimSuffix(shimCaptureLine, "\n") + " || printf '%s\\n' " + tok + "\n"}}, "tracked variable referenced"},
		{"a reference in the body of an if whose condition is the capture", [][2]string{{shimCaptureLine, "if " + shimTokenVar + `=$(sigil-helper token --landing "$_sgs_repo" </dev/null); then` + "\n  printf '%s\\n' " + tok + " >&2\nfi\n"}}, "tracked variable referenced"},
		{"the helper's output captured straight into GH_TOKEN", [][2]string{{shimCaptureLine, `GH_TOKEN=$(sigil-helper token --landing "$_sgs_repo" </dev/null) || _sgs_mint_rc=$?` + "\n"}}, "GH_TOKEN assigned outside"},
	}
}

// R-14: the check fails on each of the 20 seeded copies, for the reason the
// seed plants. A seed whose anchor is absent from the tracked script fails the
// test, because a copy that mutated nothing would be a control that cannot fail.
func TestShim_R14_StaticCheck_RejectsEachSeededCopy(t *testing.T) {
	seeds := shimSeeds()
	if len(seeds) != 20 {
		t.Fatalf("R-14 lists 20 seeded copies, this test holds %d", len(seeds))
	}
	for _, s := range seeds {
		t.Run(s.name, func(t *testing.T) {
			text := shimEdit(t, shimTrackedText(t), s.edits...)
			v := shimStaticCheck(text)
			if len(v) == 0 {
				t.Fatalf("the static check passed a seeded copy: %s", s.name)
			}
			if !strings.Contains(strings.Join(v, "\n"), s.want) {
				t.Errorf("the static check failed the copy for the wrong reason, want %q in:\n  %s", s.want, strings.Join(v, "\n  "))
			}
		})
	}
	t.Run("the helper's output captured into GH_TOKEN with the normal capture kept", func(t *testing.T) {
		text := shimEdit(t, shimTrackedText(t), [2]string{shimCaptureLine, shimCaptureLine + `GH_TOKEN=$(sigil-helper token --landing "$_sgs_repo" </dev/null)` + "\n"})
		if v := shimStaticCheck(text); !strings.Contains(strings.Join(v, "\n"), "GH_TOKEN assigned outside") {
			t.Errorf("expected a GH_TOKEN violation, got %q", v)
		}
	})
}
