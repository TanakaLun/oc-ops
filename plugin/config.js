// oc-ops opencode plugin — config resolution.
// The plugin reads a single `oc-ops` block from opencode.json. Unknown keys are
// rejected so a typo cannot silently disable a component.

export const DEFAULT_CONFIG = {
  binaryPath: "oc-ops",
  required: false,
  toolCallTimeoutMs: 300000,
  components: {
    file: true,
    map: true,
    background: false,
    android: true,
    shell: true,
    route: true,
  },
  shellEnv: true,
  routeSection: true,
  maxOutputLines: 1000,
  maxFileBytes: 32 * 1024 * 1024,
  extraGuidance: "",
}

const COMPONENT_KEYS = ["file", "map", "background", "android", "shell", "route"]
const TOP_KEYS = [
  "binaryPath", "required", "toolCallTimeoutMs", "components",
  "shellEnv", "routeSection", "maxOutputLines", "maxFileBytes", "extraGuidance",
]

export class ConfigError extends Error {
  constructor(message) {
    super(`oc-ops config: ${message}`)
    this.name = "ConfigError"
  }
}

function bool(value, key, fallback) {
  if (value === undefined || value === null) return fallback
  if (typeof value !== "boolean") throw new ConfigError(`${key} must be a boolean, got ${JSON.stringify(value)}`)
  return value
}

function int(value, key, fallback, min, max) {
  if (value === undefined || value === null) return fallback
  const n = Number(value)
  if (!Number.isInteger(n) || n < min || n > max) {
    throw new ConfigError(`${key} must be an integer ${min}..${max}, got ${JSON.stringify(value)}`)
  }
  return n
}

function str(value, key, fallback) {
  if (value === undefined || value === null) return fallback
  if (typeof value !== "string" || value.trim() === "") {
    throw new ConfigError(`${key} must be a non-empty string, got ${JSON.stringify(value)}`)
  }
  return value
}

// Resolve and validate a config object.
export function resolveConfig(raw = {}) {
  if (raw === null || typeof raw !== "object" || Array.isArray(raw)) {
    throw new ConfigError("config must be an object")
  }
  const unknown = Object.keys(raw).filter((k) => !TOP_KEYS.includes(k))
  if (unknown.length) {
    throw new ConfigError(`unknown key(s) ${unknown.map((k) => JSON.stringify(k)).join(", ")}; accepted keys are ${TOP_KEYS.map((k) => JSON.stringify(k)).join(", ")}`)
  }
  const rawComponents = raw.components ?? {}
  const unknownComponents = Object.keys(rawComponents).filter((k) => !COMPONENT_KEYS.includes(k))
  if (unknownComponents.length) {
    throw new ConfigError(`unknown component(s) ${unknownComponents.map((k) => JSON.stringify(k)).join(", ")}; accepted components are ${COMPONENT_KEYS.join(", ")}`)
  }
  const components = {}
  for (const k of COMPONENT_KEYS) components[k] = bool(rawComponents[k], `components.${k}`, DEFAULT_CONFIG.components[k])
  return {
    binaryPath: str(raw.binaryPath, "binaryPath", DEFAULT_CONFIG.binaryPath),
    required: bool(raw.required, "required", DEFAULT_CONFIG.required),
    toolCallTimeoutMs: int(raw.toolCallTimeoutMs, "toolCallTimeoutMs", DEFAULT_CONFIG.toolCallTimeoutMs, 1, 2_147_483_647),
    components,
    shellEnv: bool(raw.shellEnv, "shellEnv", DEFAULT_CONFIG.shellEnv),
    routeSection: bool(raw.routeSection, "routeSection", DEFAULT_CONFIG.routeSection),
    maxOutputLines: int(raw.maxOutputLines, "maxOutputLines", DEFAULT_CONFIG.maxOutputLines, 1, 100000),
    maxFileBytes: int(raw.maxFileBytes, "maxFileBytes", DEFAULT_CONFIG.maxFileBytes, 1024, 2_147_483_647),
    extraGuidance: typeof raw.extraGuidance === "string" ? raw.extraGuidance : "",
  }
}
