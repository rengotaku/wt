import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@/test/test-utils";
import { PortsPage } from "./PortsPage";

const mockPorts = [
  {
    repo: "myrepo",
    wt_name: "myrepo--feat-issue-1-abc",
    branch: "feat/issue-1-abc",
    port_base: 9000,
    port_range: "9000-9004",
    ports: [
      { port: 9000, listening: true, pid: 12345, proc: "air" },
      { port: 9001, listening: false },
      { port: 9002, listening: false },
      { port: 9003, listening: false },
      { port: 9004, listening: false },
    ],
  },
  {
    repo: "myrepo",
    wt_name: "myrepo--feat-issue-2-xyz",
    branch: "feat/issue-2-xyz",
    port_base: 9005,
    port_range: "9005-9009",
    ports: [
      { port: 9005, listening: false },
      { port: 9006, listening: false },
    ],
  },
  {
    repo: "otherrepo",
    wt_name: "otherrepo",
    branch: "main",
    port_base: 0,
    ports: [],
  },
];

vi.mock("@/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/api")>();
  return {
    ...actual,
    portsApi: {
      ...actual.portsApi,
      list: vi.fn(),
    },
  };
});

describe("PortsPage", () => {
  beforeEach(async () => {
    const { portsApi } = await import("@/api");
    vi.mocked(portsApi.list).mockResolvedValue(mockPorts as never);
  });

  it("renders allocated port ranges and the listening port for a running worktree", async () => {
    render(<PortsPage />);

    await waitFor(() => {
      expect(screen.getByText("9000-9004")).toBeInTheDocument();
    });
    expect(screen.getByText("9005-9009")).toBeInTheDocument();
    // Listening port shows its proc + pid.
    expect(screen.getByText("9000 air(12345)")).toBeInTheDocument();
    // The all-down worktree shows "idle".
    expect(screen.getByText("idle")).toBeInTheDocument();
  });

  it("shows an em dash for an unallocated worktree", async () => {
    render(<PortsPage />);

    await waitFor(() => {
      expect(screen.getByText("9000-9004")).toBeInTheDocument();
    });
    // Unallocated row (port_base 0) renders dashes for both the range and live cells.
    const dashes = screen.getAllByText("—");
    expect(dashes.length).toBeGreaterThanOrEqual(2);
  });
});
