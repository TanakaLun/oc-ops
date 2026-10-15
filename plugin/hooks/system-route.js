// Routing section: a compact table telling the model which tool for which task,
// reflecting only the components that are actually enabled. This mirrors the
// dsh-ops "repository-tooling" prompt section and is injected into the system
// prompt so the model routes correctly without thrashing between tools.
export function renderRouting(components, extraGuidance = "") {
  const rows = []
  if (components.file) {
    rows.push("| Search contents | ops_grep (summary-first) |")
    rows.push("| Find paths | ops_glob |")
    rows.push("| Read a file / hex | ops_inspect |")
    rows.push("| Batch replacement | ops_replace (apply:true to write) |")
  }
  if (components.map) {
    rows.push("| Explore an unfamiliar repo | ops_map |")
  }
  if (components.android) {
    rows.push("| Resolve Android/NDK/JAVA env | ops_android_env |")
    rows.push("| Inspect an APK | ops_android_apk |")
  }
  if (components.background) {
    rows.push("| Long-running builds | ops_bg_start / ops_bg_output / ops_bg_list / ops_bg_kill |")
  }
  if (rows.length === 0) return ""
  const head = ["# oc-ops tool routing", "", "| Task | Tool |", "| --- | --- |"]
  const lines = [...head, ...rows]
  if (components.file) {
    lines.push("", "- ops_* tools are token-efficient and bounded: prefer them for searches and exploration.")
    lines.push("- On a tool error, correct the arguments and retry; do not switch to a shell workaround.")
    lines.push("- Use ops_grep summary/count/files_with_matches to avoid large output.")
  }
  if (extraGuidance.trim()) lines.push("", extraGuidance.trim())
  return lines.join("\n")
}

// Compose a whole routing section with a fixed order slot.
export function routeSectionHook(components, extraGuidance) {
  return async (_input, output) => {
    const text = renderRouting(components, extraGuidance)
    if (text) output.system.push(text)
  }
}
