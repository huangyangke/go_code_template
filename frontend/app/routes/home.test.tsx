import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import Home from "./home";

describe("Home", () => {
  it("renders heading", () => {
    render(<Home />);
    expect(screen.getByRole("heading", { name: "Home" })).toBeInTheDocument();
  });
});
