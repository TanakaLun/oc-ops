// Package android provides Android toolchain environment resolution and
// lightweight APK/dex inspection helpers. It shells out only to tools that are
// already installed (aapt2, dex2jar) and never mutates the caller's env.
package android

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/oc-ops/oc-ops/internal/contract"
)

// Item is a shorthand for a compact result item.
type Item = contract.Item

// Env describes the resolved Android toolchain environment.
type Env struct {
	AndroidHome  string `json:"ANDROID_HOME"`
	AndroidNdk   string `json:"ANDROID_NDK_HOME"`
	NdkVersion   string `json:"NDK_VERSION"`
	JavaHome     string `json:"JAVA_HOME"`
	NdkToolchain string `json:"NDK_TOOLCHAIN"`
	Available    bool   `json:"available"`
}

func Handler(req contract.Request) (contract.Response, error) {
	switch req.Tool {
	case "android_env":
		return env(req)
	case "android_apk":
		return apk(req)
	case "android_dex":
		return dex(req)
	default:
		return contract.Fail("INVALID_ARGUMENT", fmt.Sprintf("unknown android capability %q", req.Tool), false)
	}
}

// ResolveEnv probes the machine for the Android toolchain.
func ResolveEnv(home string) Env {
	env := Env{}
	// ANDROID_HOME
	if ah := firstNonEmpty(os.Getenv("ANDROID_HOME"), os.Getenv("ANDROID_SDK_ROOT"), sdkAt(home)); ah != "" {
		env.AndroidHome = ah
	}
	// NDK: resolve the highest version under <sdk>/ndk
	if env.AndroidHome != "" {
		ndkRoot := filepath.Join(env.AndroidHome, "ndk")
		if ver := highestVersion(ndkRoot); ver != "" {
			env.AndroidNdk = filepath.Join(ndkRoot, ver)
			env.NdkVersion = ver
			env.NdkToolchain = filepath.Join(env.AndroidNdk, "toolchains", "llvm", "prebuilt", hostPrebuilt(), "bin")
		}
	}
	// JAVA_HOME
	if jh := os.Getenv("JAVA_HOME"); jh != "" {
		env.JavaHome = jh
	} else if p, err := exec.LookPath("java"); err == nil {
		env.JavaHome = filepath.Dir(filepath.Dir(p))
	}
	env.Available = env.AndroidHome != "" || env.AndroidNdk != "" || env.JavaHome != ""
	return env
}

func env(req contract.Request) (contract.Response, error) {
	e := ResolveEnv(req.Cwd)
	return contract.Response{
		Status: "completed", Phase: "observe", Complete: true,
		Items: []json.RawMessage{contract.ToRaw(e)},
	}, nil
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

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// hostPrebuilt maps the running GOARCH to the NDK prebuilt directory name.
func hostPrebuilt() string {
	switch runtime.GOARCH {
	case "arm64":
		return "linux-aarch64"
	case "arm":
		return "linux-arm"
	case "386":
		return "linux-x86"
	default:
		return "linux-x86_64"
	}
}

func sdkAt(home string) string {
	candidates := []string{
		filepath.Join(home, "Android", "Sdk"),
		"/data/data/com.termux/files/home/Android/Sdk",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func highestVersion(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var versions []string
	for _, e := range entries {
		if e.IsDir() {
			versions = append(versions, e.Name())
		}
	}
	if len(versions) == 0 {
		return ""
	}
	// Use a simple semver-ish comparison (strings with numbers sort by segments).
	sortVersions(versions)
	return versions[len(versions)-1]
}

func sortVersions(v []string) {
	// insertion sort by numeric segments; good enough for ndk versions.
	less := func(a, b string) bool {
		return segmentLess(a, b)
	}
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && less(v[j], v[j-1]); j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}

func segmentLess(a, b string) bool {
	pa := strings.Split(a, ".")
	pb := strings.Split(b, ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		na, ea := strconvAtoi(pa[i])
		nb, eb := strconvAtoi(pb[i])
		if ea == nil && eb == nil {
			if na != nb {
				return na < nb
			}
		} else if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	return len(pa) < len(pb)
}

func strconvAtoi(s string) (int, error) {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a number")
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

func argString(args map[string]any, key, def string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return def
}

var _ = runtime.GOOS
