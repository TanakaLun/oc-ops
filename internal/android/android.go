// Package android provides lightweight APK/dex inspection helpers for the
// oc-ops backend. It shells out only to tools that are already installed
// (aapt2, unzip) and never mutates the caller's env.
package android

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/oc-ops/oc-ops/internal/contract"
)

// Item is a shorthand for a compact result item.
type Item = contract.Item

func Handler(req contract.Request) (contract.Response, error) {
	switch req.Tool {
	case "android_apk":
		return apk(req)
	case "android_dex":
		return dex(req)
	default:
		return contract.Fail("INVALID_ARGUMENT", fmt.Sprintf("unknown android capability %q", req.Tool), false)
	}
}

func apk(req contract.Request) (contract.Response, error) {
	apk := argString(req.Args, "apk", "")
	if apk == "" {
		return contract.Fail("INVALID_ARGUMENT", "apk path is required", false)
	}
	abs, err := contract.ResolvePath(req.Cwd, apk)
	if err != nil {
		return contract.Fail("INVALID_ARGUMENT", err.Error(), false)
	}
	if _, err := os.Stat(abs); err != nil {
		return contract.Reject("ACCESS_DENIED", fmt.Sprintf("apk not found: %s", apk))
	}
	aapt2, err := exec.LookPath("aapt2")
	if err != nil {
		return contract.Fail("EXECUTOR_UNAVAILABLE", "aapt2 not found on PATH; install build-tools or set PATH", false)
	}
	out, err := exec.Command(aapt2, "dump", "badging", abs).Output()
	if err != nil {
		return contract.Fail("RESULT_UNKNOWN", fmt.Sprintf("aapt2 badging failed: %v", err), true)
	}
	items := parseBadging(string(out))
	return contract.Response{
		Status: "completed", Phase: "observe", Complete: true, Items: items,
	}, nil
}

func parseBadging(text string) []json.RawMessage {
	var items []json.RawMessage
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "activity-alias") {
			continue
		}
		if strings.HasPrefix(line, "package:") || strings.HasPrefix(line, "application-label:") ||
			strings.HasPrefix(line, "sdkVersion:") || strings.HasPrefix(line, "targetSdkVersion:") ||
			strings.HasPrefix(line, "uses-permission:") {
			items = append(items, contract.ToRaw(line))
		}
	}
	return items
}

func dex(req contract.Request) (contract.Response, error) {
	apk := argString(req.Args, "apk", "")
	if apk == "" {
		return contract.Fail("INVALID_ARGUMENT", "apk path is required", false)
	}
	abs, err := contract.ResolvePath(req.Cwd, apk)
	if err != nil {
		return contract.Fail("INVALID_ARGUMENT", err.Error(), false)
	}
	if _, err := os.Stat(abs); err != nil {
		return contract.Reject("ACCESS_DENIED", fmt.Sprintf("apk not found: %s", apk))
	}
	// dex2jar: com.android.tools... a simple wrapper that names classes.
	// We report the dex file list as compact lines; full dex parsing is left to
	// the user's jadx/dex2jar workflow to keep output bounded.
	out, err := exec.Command("unzip", "-l", abs).CombinedOutput()
	if err != nil {
		return contract.Fail("RESULT_UNKNOWN", fmt.Sprintf("unzip failed: %v", err), true)
	}
	var items []json.RawMessage
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 4 && strings.HasSuffix(fields[len(fields)-1], ".dex") {
			items = append(items, contract.ToRaw(Item{"dex": fields[len(fields)-1]}))
		}
	}
	return contract.Response{
		Status: "completed", Phase: "observe", Complete: true, Items: items,
	}, nil
}

func argString(args map[string]any, key, def string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return def
}
