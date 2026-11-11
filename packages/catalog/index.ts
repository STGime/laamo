// Typed access to the brick catalog for the web builder.
// The Go side (types, loader, stack validator) is backend/internal/catalog;
// both read the same bricks.json.
import data from "./bricks.json" with { type: "json" };

export type Lang = "en" | "de";
export type Text = Record<Lang, string>;

export type GroupId = "when" | "look" | "think" | "do" | "ask";
export type Color = "yellow" | "blue" | "purple" | "green" | "red";
export type Shape =
  "start" | "expand" | "enrich" | "filter" | "combine" | "act";
export type Scalar = string | number | boolean;
export type PlaceType =
  "discord" | "slack" | "teams" | "telegram" | "email" | "posta";

/** Every key must match: a list of allowed values, or "set" for a non-empty value. */
export type Condition = Record<string, Scalar[] | "set">;

export interface Group {
  id: GroupId;
  color: Color;
  title: Text;
  question: Text;
}

export interface ProfileFact {
  key: string;
  label: Text;
  question: Text;
}

export interface Field {
  key: string;
  label: Text;
  pick?: boolean;
}

export interface Fix {
  brick: string;
  config?: Record<string, unknown>;
}

export type SettingType =
  | "text"
  | "longtext"
  | "url"
  | "email"
  | "select"
  | "days"
  | "time"
  | "number"
  | "slider"
  | "toggle"
  | "tags"
  | "fields"
  | "place"
  | "list";

export interface Option {
  value: Scalar;
  label: Text;
  status?: "mvp" | "later";
}

export interface Setting {
  key: string;
  type: SettingType;
  label: Text;
  help?: Text;
  placeholder?: Text;
  required?: boolean;
  /** Behind "More" */
  advanced?: boolean;
  default?: unknown;
  show_if?: Condition;
  /** Text may contain {field}, {my …}, {answer} pills */
  pills?: boolean;
  options?: Option[];
  min?: number;
  max?: number;
  step?: number;
  max_length?: number;
  ends?: [Text, Text];
  place_types?: PlaceType[];
  create?: boolean;
}

export interface Requires {
  brain?: boolean;
  /** Any one of these place types */
  places?: PlaceType[];
  plan?: "paid";
}

export interface Brick {
  id: string;
  group: GroupId;
  shape: Shape;
  shape_if?: { if: Condition; shape: Shape }[];
  status: "mvp" | "later";
  trigger?: "schedule" | "manual" | "email" | "watch" | "webhook";
  title: Text;
  does: Text;
  /** First matching entry wins; [key] is replaced by the setting's display value. */
  sentence: { if?: Condition; text: Text }[];
  settings: Setting[];
  card_kind?: string;
  needs?: string[];
  needs_if?: { if: Condition; fields?: string[]; context?: string[] }[];
  adds?: string[];
  adds_if?: { if: Condition; fields?: string[]; open?: boolean }[];
  adds_from?: string;
  provides?: string[];
  provides_if?: { if: Condition; keys: string[] }[];
  requires?: Requires;
  requires_if?: (Requires & { if: Condition })[];
  check?: { default?: boolean; required_if?: Condition };
  /** Card fields an Act brick sends to; checked against trust at run time. */
  targets_if?: { if: Condition; fields: string[] }[];
}

export interface Catalog {
  schema: 1;
  groups: Group[];
  profile: ProfileFact[];
  fields: Field[];
  fixes: Record<string, Fix>;
  bricks: Brick[];
}

export interface StackBrick {
  id: string;
  type: string;
  config: Record<string, unknown>;
}

export interface Stack {
  schema: 1;
  bricks: StackBrick[];
  limits?: {
    max_cards?: number;
    max_external_emails?: number;
    max_cost_eur?: number;
  };
  /** About me facts just for this helper ("Just for this helper"). */
  profile?: Record<string, string>;
}

export const catalog = data as unknown as Catalog;

const byId = new Map(catalog.bricks.map((b) => [b.id, b]));

export function brick(id: string): Brick | undefined {
  return byId.get(id);
}

export function group(id: GroupId): Group {
  const g = catalog.groups.find((g) => g.id === id);
  if (!g) throw new Error(`unknown group ${id}`);
  return g;
}

/** Config with defaults filled in for missing settings. */
export function withDefaults(
  b: Brick,
  config: Record<string, unknown>,
): Record<string, unknown> {
  const out = { ...config };
  for (const s of b.settings)
    if (!(s.key in out) && s.default !== undefined) out[s.key] = s.default;
  return out;
}

/** Does config (with defaults) satisfy the condition? An empty condition matches. */
export function matches(
  cond: Condition | undefined,
  config: Record<string, unknown>,
): boolean {
  if (!cond) return true;
  return Object.entries(cond).every(([key, want]) => {
    const v = config[key];
    if (want === "set")
      return (
        v !== undefined &&
        v !== null &&
        v !== "" &&
        !(Array.isArray(v) && v.length === 0)
      );
    return want.some((w) => w === v);
  });
}
