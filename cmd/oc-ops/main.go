// oc-ops is a token-efficiency and capability layer for opencode on
// Termux/Android. It is a single Go binary with subcommands that reads JSON
// args from argv or stdin and writes a JSON response — a pure function of
// (subcommand, args, cwd, env). No session state, no daemon (except bg jobs).
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/oc-ops/oc-ops/internal/android"
	"github.com/oc-ops/oc-ops/internal/batch"
	"github.com/oc-ops/oc-ops/internal/bg"
	"github.com/oc-ops/oc-ops/internal/contract"
	"github.com/oc-ops/oc-ops/internal/file"
	"github.com/oc-ops/oc-ops/internal/repomap"
	"github.com/oc-ops/oc-ops/internal/result"
)

// version is stamped at build time via -ldflags.
var version = "0.1.0-dev"

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}
	switch args[0] {
	case "version", "--version", "-v":
		fmt.Fprintln(os.Stdout, version)
		return
	case "help", "--help", "-h":
		usage()
		return
	case "__monitor":
		// Detached job monitor entry point, spawned by `bg start`.
		if len(args) >= 3 {
			bg.MonitorMain(args[1], args[2])
		}
		os.Exit(1)
	}

	tool := args[0]
	// Optional JSON arg as the second positional, e.g.:
	//   oc-ops grep '{"path":".","pattern":"x"}'
	//   oc-ops file grep '{"path":...}'
	req, err := buildRequest(tool, args[1:])
	if err != nil {
		writeError(err)
		os.Exit(2)
	}
	resp, err := dispatch(req)
	if err != nil {
		resp, _ = contract.Fail("INTERNAL", err.Error(), true)
	}
	writeResponse(resp)
}

// buildRequest normalizes a JSON arg and the cwd/env into a Request.
func buildRequest(tool string, rest []string) (contract.Request, error) {
	req := contract.Request{
		Tool:     tool,
		Args:     map[string]any{},
		Cwd:      cwdOrHome(),
		Deadline: time.Now().Add(300 * time.Second),
	}
	// Namespace tools accept a bare subcommand positional: `oc-ops file grep ...`,
	// `oc-ops bg start ...`, `oc-ops android env`. Translate it to args.tool.
	if isNamespace(tool) && len(rest) > 0 && !stringsHasPrefix(rest[0], "--") && !stringsHasPrefix(rest[0], "{") {
		req.Args["tool"] = rest[0]
		rest = rest[1:]
	}
	if len(rest) == 0 {
		return req, nil
	}
	// First positional may be a JSON object (plugin form) or flags (CLI form).
	raw := rest[0]
	if stringsHasPrefix(raw, "{") {
		if err := json.Unmarshal([]byte(raw), &req.Args); err != nil {
			return req, fmt.Errorf("invalid args JSON: %w", err)
		}
	} else {
		// Flag form: --key value pairs.
		if err := parseFlags(rest, req.Args); err != nil {
			return req, err
		}
	}
	// Lift transport fields out of the args object (the plugin overlays them
	// onto the tool args). They never reach the tool schema.
	if v, ok := req.Args["cwd"].(string); ok {
		req.Cwd = v
		delete(req.Args, "cwd")
	}
	if v, ok := req.Args["read_only"].(bool); ok {
		req.ReadOnly = v
	}
	delete(req.Args, "read_only")
	return req, nil
}

func isNamespace(tool string) bool {
	switch tool {
	case "file", "bg", "android":
		return true
	}
	return false
}

func parseFlags(flags []string, out map[string]any) error {
	i := 0
	for i < len(flags) {
		f := flags[i]
		if !stringsHasPrefix(f, "--") {
			return fmt.Errorf("unexpected positional: %s", f)
		}
		key := f[2:]
		i++
		if i >= len(flags) || stringsHasPrefix(flags[i], "--") {
			out[key] = true
			continue
		}
		val := flags[i]
		i++
		// Store as string; numeric coercion happens in handlers via argInt.
		out[key] = val
	}
	return nil
}

func cwdOrHome() string {
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "."
}

// dispatch routes a request to its capability handler.
func dispatch(req contract.Request) (contract.Response, error) {
	switch req.Tool {
	// file capabilities (also callable as `oc-ops file grep ...`)
	case "grep", "glob", "inspect", "replace":
		return file.Handler(req)
	case "file":
		if len(req.Args) == 0 {
			return contract.Fail("INVALID_ARGUMENT", "usage: oc-ops file <grep|glob|inspect|replace>", false)
		}
		sub := req.Args["tool"].(string)
		req.Tool = sub
		return file.Handler(req)
	case "map":
		return repomap.Handler(req)
	case "bg_start", "bg_output", "bg_list", "bg_kill", "bg":
		if req.Tool == "bg" {
			sub := req.Args["tool"].(string)
			req.Tool = "bg_" + sub
		}
		return bg.Handler(req)
	case "batch":
		return batch.Handler(req)
	case "result":
		return result.Handler(req)
	case "android_env", "android_apk", "android_dex", "android":
		if req.Tool == "android" {
			sub := req.Args["tool"].(string)
			req.Tool = "android_" + sub
		}
		return android.Handler(req)
	default:
		return contract.Fail("INVALID_ARGUMENT", fmt.Sprintf("unknown capability %q", req.Tool), false)
	}
}

func writeResponse(resp contract.Response) {
	b, _ := resp.Encode()
	_, _ = os.Stdout.Write(b)
	fmt.Fprintln(os.Stdout)
}

func writeError(err error) {
	resp, _ := contract.Fail("INVALID_ARGUMENT", err.Error(), false)
	b, _ := resp.Encode()
	_, _ = os.Stdout.Write(b)
	fmt.Fprintln(os.Stdout)
}

func usage() {
	fmt.Fprint(io.Discard, "") // keep io referenced
	fmt.Fprintln(os.Stdout, `oc-ops - token-efficient ops for opencode (Termux/Android)

Usage:
  oc-ops <capability> [--key value ... | '{"key":"value"}']
  oc-ops file <grep|glob|inspect|replace> ...
  oc-ops bg <start|output|list|kill> ...
  oc-ops android <env|apk|dex> ...

Capabilities:
  grep       Search contents, summary-first
  glob       Find paths by pattern
  inspect    Read bounded lines or hex
  replace    Batch replacement (dry-run by default; --apply to write)
  map        Repository structure map + symbol index
  bg_start   Start a managed background job
  bg_output  Read job output
  bg_list    List owned jobs
  bg_kill    Kill a job
  android_env  Resolve NDK/JAVA/ANDROID_HOME env
  android_apk  Inspect an APK (aapt2 badging)
  android_dex  List dex entries in an APK

Examples:
  oc-ops grep '{"path":".","pattern":"foo","outputMode":"summary"}'
  oc-ops map --path . --mode both --depth 6
  oc-ops android env`)
}

func stringsHasPrefix(s, p string) bool {
	return len(s) >= len(p) && s[:len(p)] == p
}
