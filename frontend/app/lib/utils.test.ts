import { describe, expect, it } from "vitest";
import { cn } from "./utils";

describe("cn", () => {
  it("merges class names", () => {
    expect(cn("foo", "bar")).toBe("foo bar");
  });

  it("deduplicates conflicting tailwind classes", () => {
    expect(cn("p-2", "p-4")).toBe("p-4");
  });

  it("ignores falsy values", () => {
    expect(cn("foo", false, undefined, null, "bar")).toBe("foo bar");
  });

  it("handles conditional classes", () => {
    expect(cn("base", { active: true, disabled: false })).toBe("base active");
  });
});
