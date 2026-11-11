// Runs the shared validator cases (also run by Go in
// backend/internal/catalog) through the TypeScript port.
import { describe, expect, it } from "vitest";
import fixtures from "./fixtures/validate.json" with { type: "json" };
import type { Stack } from "./index.ts";
import { available, pills, validate } from "./validate.ts";

interface Fixture {
  name: string;
  stack: Stack;
  pending?: string[];
  issues: string[];
  fixes?: Record<string, string>;
}

describe("validate (shared fixtures)", () => {
  for (const fx of fixtures as unknown as Fixture[]) {
    it(fx.name, () => {
      const issues = validate(fx.stack, { pending: fx.pending });
      const got = issues.map((i) => `${i.brick ?? ""}:${i.code}`).sort();
      expect(got).toEqual([...fx.issues].sort());
      for (const i of issues) {
        const key = `${i.brick ?? ""}:${i.code}`;
        expect(i.fix?.insert ?? "", key).toBe(fx.fixes?.[key] ?? "");
      }
    });
  }
});

describe("pills", () => {
  it("matches the Go parser", () => {
    expect(
      pills(
        "Hi {name}, about {my business} and {trigger.subject} {answer} {Bad Pill}",
      ),
    ).toEqual([
      { raw: "name", kind: "field", key: "name" },
      { raw: "my business", kind: "profile", key: "business" },
      { raw: "trigger.subject", kind: "context", key: "trigger.subject" },
      { raw: "answer", kind: "context", key: "answer" },
      { raw: "Bad Pill", kind: "", key: "Bad Pill" },
    ]);
  });
});

describe("available", () => {
  it("lists what bricks above provide", () => {
    const st = {
      schema: 1 as const,
      bricks: [
        { id: "b1", type: "when.tap", config: { question: "Which city?" } },
        {
          id: "b2",
          type: "look.search",
          config: { query: "cafés in {answer}" },
        },
        { id: "b3", type: "think.keep", config: { criteria: "quiet" } },
      ],
    };
    expect(available(st, 1).fields).toEqual([]);
    expect(available(st, 1).context).toContain("answer");
    expect(available(st, 2).fields).toContain("title");
  });
});
