import { describe, it, expect, vi, beforeEach } from "vitest";
import { within } from "@testing-library/react";
import { render, screen, waitFor, fireEvent } from "@/test/test-utils";
import { PortsPage } from "./PortsPage";

const mockListeners = [
  { port: 8000, pid: 111, proc: "python", managed: false }, // foreign squatter
  { port: 9000, pid: 222, proc: "air", managed: true, owner: "wt/main" },
];

vi.mock("@/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/api")>();
  return {
    ...actual,
    portsApi: {
      ...actual.portsApi,
      listeners: vi.fn(),
      stale: vi.fn(),
      prune: vi.fn(),
    },
  };
});

describe("PortsPage (machine-wide port doctor)", () => {
  beforeEach(async () => {
    const { portsApi } = await import("@/api");
    vi.mocked(portsApi.listeners).mockResolvedValue(mockListeners as never);
    vi.mocked(portsApi.stale).mockResolvedValue([] as never);
    vi.mocked(portsApi.prune).mockResolvedValue({
      removed: [],
      count: 0,
    } as never);
  });

  it("lists all listening ports with proc and pid", async () => {
    render(<PortsPage />);
    await waitFor(() => {
      expect(screen.getByText("8000")).toBeInTheDocument();
    });
    expect(screen.getByText("python")).toBeInTheDocument();
    expect(screen.getByText("9000")).toBeInTheDocument();
  });

  it("classifies wt-managed vs foreign", async () => {
    render(<PortsPage />);
    await waitFor(() => {
      expect(screen.getByText("8000")).toBeInTheDocument();
    });
    const table = screen.getByRole("table");
    // Only the 8000 squatter is foreign; the 9000 wt-managed row is not.
    expect(within(table).getAllByText("foreign")).toHaveLength(1);
    expect(
      within(table).getByText((c) => c.includes("wt/main"))
    ).toBeInTheDocument();
  });

  it("lists ghost entries and prunes them on confirm", async () => {
    const { portsApi } = await import("@/api");
    vi.mocked(portsApi.stale).mockResolvedValue([
      { repo: "marchedb", wt_name: "marchedb--issue-9", port_base: 9200, port_range: "9200-9204" },
    ] as never);
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);

    render(<PortsPage />);
    await waitFor(() => {
      expect(screen.getByText("marchedb--issue-9")).toBeInTheDocument();
    });
    expect(screen.getByText("9200-9204")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /掃除して回収/ }));
    expect(confirmSpy).toHaveBeenCalled();
    await waitFor(() => {
      expect(vi.mocked(portsApi.prune)).toHaveBeenCalled();
    });
    confirmSpy.mockRestore();
  });

  it("shows an empty state when there are no ghost entries", async () => {
    render(<PortsPage />);
    await waitFor(() => {
      expect(screen.getByText("幽霊エントリはありません。")).toBeInTheDocument();
    });
  });
});
