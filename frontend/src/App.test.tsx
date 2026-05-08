import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import App from "./App";

describe("App", () => {
  it("renders navigation links", () => {
    render(<App />);
    expect(screen.getByRole("link", { name: "Worktrees" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Repos" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "GC" })).toBeInTheDocument();
  });
});
