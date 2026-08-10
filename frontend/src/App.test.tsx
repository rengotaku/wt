import { render, screen, waitFor } from "@testing-library/react";
import { describe, it, expect, afterEach } from "vitest";
import App from "./App";

describe("App", () => {
  afterEach(() => {
    window.history.pushState({}, "", "/");
  });

  it("renders navigation links", () => {
    render(<App />);
    expect(screen.getByRole("link", { name: "Worktrees" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Repos" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Maintenance" })).toBeInTheDocument();
  });

  it.each(["/gc", "/ports"])(
    "redirects the legacy %s URL to /maintenance",
    async (legacyPath) => {
      window.history.pushState({}, "", legacyPath);
      render(<App />);
      await waitFor(() => {
        expect(window.location.pathname).toBe("/maintenance");
      });
      expect(screen.getByText("GC オプション")).toBeInTheDocument();
    },
  );
});
