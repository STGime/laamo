// Stack validation for the builder's live hints. A port of
// backend/internal/catalog/validate.go: both run the cases in
// fixtures/validate.json, so the builder and the server agree.
import {
  brick as catalogBrick,
  catalog,
  matches,
  withDefaults,
} from "./index.ts";
import type { Brick, Setting, Stack, StackBrick } from "./index.ts";

export type Level = "error" | "hint";

export interface IssueFix {
  insert: string;
  config?: Record<string, unknown>;
  before: string;
}

export interface Issue {
  brick?: string;
  index: number;
  level: Level;
  code: string;
  setting?: string;
  field?: string;
  message: string;
  fix?: IssueFix;
}

export interface Pill {
  raw: string;
  kind: "field" | "profile" | "context" | "";
  key: string;
}

const keyRe = /^[a-z][a-z0-9_]*$/;
const timeRe = /^([01][0-9]|2[0-3]):[0-5][0-9]$/;
const emailRe = /^[^\s@<>()",;:]+@[^\s@<>()",;:]+\.[^\s@<>()",;:]+$/;
const weekdays = ["mon", "tue", "wed", "thu", "fri", "sat", "sun"];

/** The {…} placeholders in s. Bad ones have kind "". */
export function pills(s: string): Pill[] {
  const out: Pill[] = [];
  for (const m of s.matchAll(/\{([^{}]*)\}/g)) {
    const raw = m[1].trim();
    let p: Pill;
    if (raw.startsWith("my "))
      p = {
        raw,
        kind: "profile",
        key: raw.slice(3).trim().replaceAll(" ", "_"),
      };
    else if (raw === "answer" || raw.startsWith("trigger."))
      p = { raw, kind: "context", key: raw };
    else p = { raw, kind: "field", key: raw };
    if (!keyRe.test(p.key.replace(/^trigger\./, ""))) p.kind = "";
    out.push(p);
  }
  return out;
}

const isEmpty = (v: unknown) =>
  v === undefined ||
  v === null ||
  v === "" ||
  (Array.isArray(v) && v.length === 0);

const hasProfile = (key: string) => catalog.profile.some((p) => p.key === key);

function fixFor(key: string, before: string): IssueFix | undefined {
  const f = catalog.fixes[key];
  return f ? { insert: f.brick, config: f.config, before } : undefined;
}

function addsFor(
  b: Brick,
  cfg: Record<string, unknown>,
): { fields: string[]; open: boolean } {
  const fields = [...(b.adds ?? [])];
  let open = false;
  for (const r of b.adds_if ?? []) {
    if (matches(r.if, cfg)) {
      fields.push(...(r.fields ?? []));
      open ||= !!r.open;
    }
  }
  if (b.adds_from) {
    const v = cfg[b.adds_from];
    if (Array.isArray(v))
      fields.push(...v.filter((x): x is string => typeof x === "string"));
  }
  return { fields, open };
}

function providesFor(b: Brick, cfg: Record<string, unknown>): string[] {
  const keys = [...(b.provides ?? [])];
  for (const r of b.provides_if ?? [])
    if (matches(r.if, cfg)) keys.push(...r.keys);
  return keys;
}

function shapeFor(b: Brick, cfg: Record<string, unknown>) {
  return b.shape_if?.find((r) => matches(r.if, cfg))?.shape ?? b.shape;
}

function stringList(v: unknown): string[] | null {
  if (v === undefined || v === null) return [];
  if (!Array.isArray(v) || v.some((x) => typeof x !== "string")) return null;
  return v as string[];
}

function checkValue(set: Setting, v: unknown): string {
  switch (set.type) {
    case "text":
    case "longtext":
    case "url":
    case "email":
    case "time":
    case "place":
    case "list": {
      if (typeof v !== "string") return "must be text";
      if (set.max_length && [...v].length > set.max_length)
        return `is longer than ${set.max_length} characters`;
      if (set.type === "time" && !timeRe.test(v))
        return "must be a time like 08:00";
      if (set.type === "email" && pills(v).length === 0 && !emailRe.test(v))
        return "must be an email address";
      if (set.type === "url") {
        try {
          const u = new URL(v);
          if ((u.protocol !== "http:" && u.protocol !== "https:") || !u.host)
            throw new Error();
        } catch {
          return "must be a web address (https://…)";
        }
      }
      return "";
    }
    case "select": {
      const o = set.options?.find((o) => o.value === v);
      if (!o) return `has no option ${String(v)}`;
      if (o.status === "later")
        return `option ${String(v)} is not available yet`;
      return "";
    }
    case "number":
    case "slider": {
      if (typeof v !== "number" || Number.isNaN(v)) return "must be a number";
      if (
        (set.min !== undefined && v < set.min) ||
        (set.max !== undefined && v > set.max)
      )
        return `must be between ${set.min ?? "∞"} and ${set.max ?? "∞"}`;
      return "";
    }
    case "toggle":
      return typeof v === "boolean" ? "" : "must be on or off";
    case "days": {
      const days = stringList(v);
      if (!days || days.length === 0) return "needs at least one day";
      const bad = days.find((d) => !weekdays.includes(d));
      return bad ? `has an unknown day "${bad}"` : "";
    }
    case "tags":
      return stringList(v) ? "" : "must be a list of words";
    case "fields": {
      const keys = stringList(v);
      if (!keys || keys.length === 0) return "needs at least one detail";
      const bad = keys.find((k) => !keyRe.test(k));
      return bad ? `has a bad detail name "${bad}" (use lower_case)` : "";
    }
  }
  return "";
}

export interface ValidateOptions {
  /** Settings filled later, e.g. by a template's setup questions ("b5.place"). */
  pending?: string[];
  /** Called before each brick with what exists there (the builder's pills). */
  at?: (
    index: number,
    fields: ReadonlySet<string>,
    context: ReadonlySet<string>,
  ) => void;
}

/** Checks a stack top to bottom, like catalog.Validate in Go. */
export function validate(st: Stack, opts: ValidateOptions = {}): Issue[] {
  const issues: Issue[] = [];
  const pending = new Set(opts.pending ?? []);
  if (st.schema !== 1)
    issues.push({
      index: -1,
      level: "error",
      code: "bad_schema",
      message: `stack schema ${st.schema} is not supported`,
    });
  if (!st.bricks?.length) {
    issues.push({
      index: -1,
      level: "error",
      code: "empty_stack",
      message: "the stack has no bricks",
    });
    return issues;
  }
  issues.push(...validateLimits(st.limits));

  const seen = new Set<string>();
  let fields = new Set<string>();
  let open = false;
  const context = new Set<string>();
  let check: { brick: string; index: number; goesAhead: boolean } | null = null;

  st.bricks.forEach((sb: StackBrick, i) => {
    const at = (level: Level, code: string, message: string): Issue => ({
      brick: sb.id,
      index: i,
      level,
      code,
      message,
    });
    if (!sb.id || seen.has(sb.id))
      issues.push(
        at(
          "error",
          "duplicate_id",
          `brick id "${sb.id}" is empty or used twice`,
        ),
      );
    seen.add(sb.id);

    const b = catalogBrick(sb.type);
    if (!b) {
      issues.push(at("error", "unknown_brick", `unknown brick "${sb.type}"`));
      return;
    }
    if (b.status !== "mvp")
      issues.push(
        at("error", "not_available", `${b.title.en} is not available yet`),
      );
    if (i === 0 && b.group !== "when")
      issues.push({
        ...at("error", "start_first", "a helper starts with a When brick"),
        fix: { insert: "when.tap", before: sb.id },
      });
    else if (i > 0 && b.group === "when")
      issues.push(
        at("error", "start_only_first", "a When brick can only sit on top"),
      );

    opts.at?.(i, fields, context);
    const raw = sb.config ?? {};
    const cfg = withDefaults(b, raw);

    // settings
    for (const key of Object.keys(raw))
      if (!b.settings.some((s) => s.key === key))
        issues.push({
          ...at(
            "error",
            "unknown_setting",
            `${b.title.en} has no setting "${key}"`,
          ),
          setting: key,
        });
    for (const set of b.settings) {
      if (!matches(set.show_if, cfg)) continue;
      const v = cfg[set.key];
      if (isEmpty(v)) {
        if (set.required && !pending.has(`${sb.id}.${set.key}`))
          issues.push({
            ...at(
              "error",
              "missing_setting",
              `${b.title.en}: "${set.label.en}" is required`,
            ),
            setting: set.key,
          });
        continue;
      }
      const msg = checkValue(set, v);
      if (msg)
        issues.push({
          ...at(
            "error",
            "bad_value",
            `${b.title.en}: "${set.label.en}" ${msg}`,
          ),
          setting: set.key,
        });
    }

    // needs
    const needFields = [...(b.needs ?? [])];
    const needContext: string[] = [];
    for (const r of b.needs_if ?? [])
      if (matches(r.if, cfg)) {
        needFields.push(...(r.fields ?? []));
        needContext.push(...(r.context ?? []));
      }
    for (const set of b.settings) {
      const text = cfg[set.key];
      if (!set.pills || !matches(set.show_if, cfg) || typeof text !== "string")
        continue;
      for (const p of pills(text)) {
        if (p.kind === "field") needFields.push(p.key);
        else if (p.kind === "context") needContext.push(p.key);
        else if (p.kind === "profile") {
          if (!hasProfile(p.key))
            issues.push({
              ...at(
                "error",
                "unknown_profile",
                `{${p.raw}}: there is no About me fact "${p.key}"`,
              ),
              setting: set.key,
              field: p.key,
            });
        } else
          issues.push({
            ...at("error", "bad_pill", `{${p.raw}} is not a valid placeholder`),
            setting: set.key,
          });
      }
    }
    const reported = new Set<string>();
    for (const f of needFields) {
      if (fields.has(f) || open || reported.has(f)) continue;
      reported.add(f);
      const fix = fixFor(f, sb.id);
      let message = `${b.title.en} needs {${f}}, but no brick above provides it`;
      if (fix)
        message += `. Add "${catalogBrick(fix.insert)?.title.en}" first?`;
      issues.push({
        ...at("error", "missing_field", message),
        field: f,
        ...(fix ? { fix } : {}),
      });
    }
    for (const k of needContext) {
      if (context.has(k) || reported.has(`ctx:${k}`)) continue;
      reported.add(`ctx:${k}`);
      let message = `${b.title.en} uses {${k}}, but nothing above provides it`;
      if (k === "answer")
        message += ' (add "Ask me something", or a question to "Tap Go")';
      else if (k.startsWith("trigger."))
        message +=
          ' (only available when the helper starts with "Email this helper")';
      issues.push({ ...at("error", "missing_context", message), field: k });
    }

    // Check first
    if (b.check) {
      const required =
        !!b.check.required_if &&
        Object.keys(b.check.required_if).length > 0 &&
        matches(b.check.required_if, cfg);
      const fix = fixFor("check", sb.id);
      if (required && !check)
        issues.push({
          ...at(
            "error",
            "check_required",
            `${b.title.en} goes to other people, so it needs "Check first" above it`,
          ),
          ...(fix ? { fix } : {}),
        });
      else if (required && check?.goesAhead)
        issues.push({
          brick: check.brick,
          index: check.index,
          level: "error",
          code: "check_goes_ahead",
          setting: "on_timeout",
          message: `"Check first" must skip, not go ahead, when you don't answer: ${b.title.en} goes to other people`,
        });
      else if (b.check.default && !check)
        issues.push({
          ...at(
            "hint",
            "check_suggested",
            `${b.title.en} leaves Laamo. Check with you first?`,
          ),
          ...(fix ? { fix } : {}),
        });
    }

    // apply the brick's output
    const adds = addsFor(b, cfg);
    const shape = shapeFor(b, cfg);
    if (shape === "start" || shape === "expand" || shape === "combine") {
      fields = new Set();
      open = adds.open;
    } else open ||= adds.open;
    for (const f of adds.fields) fields.add(f);
    for (const k of providesFor(b, cfg)) context.add(k);
    if (b.id === "ask.check")
      check = { brick: sb.id, index: i, goesAhead: cfg.on_timeout === "go" };
  });
  return issues;
}

function validateLimits(l: Stack["limits"]): Issue[] {
  if (!l) return [];
  const out: Issue[] = [];
  const bad = (setting: string, message: string) =>
    out.push({
      index: -1,
      level: "error",
      code: "bad_limit",
      setting,
      message,
    });
  if (l.max_cards !== undefined && (l.max_cards < 1 || l.max_cards > 500))
    bad("max_cards", "max_cards must be between 1 and 500");
  if (
    l.max_external_emails !== undefined &&
    (l.max_external_emails < 0 || l.max_external_emails > 100)
  )
    bad("max_external_emails", "max_external_emails must be between 0 and 100");
  if (
    l.max_cost_eur !== undefined &&
    (l.max_cost_eur <= 0 || l.max_cost_eur > 10)
  )
    bad("max_cost_eur", "max_cost_eur must be above 0 and at most 10");
  return out;
}

/** Only the error-level issues. */
export const errors = (issues: Issue[]) =>
  issues.filter((i) => i.level === "error");

/** Card fields and context keys ({answer}) that exist right before brick index. */
export function available(
  st: Stack,
  index: number,
): { fields: string[]; context: string[] } {
  let out = { fields: [] as string[], context: [] as string[] };
  validate(st, {
    at: (i, fields, context) => {
      if (i === index) out = { fields: [...fields], context: [...context] };
    },
  });
  return out;
}
