import { describe, it, expect, vi, beforeEach } from "vitest";
import { within } from "@testing-library/react";
import { render, screen, waitFor, fireEvent } from "@/test/test-utils";
import { MaintenancePage } from "./MaintenancePage";

const mockListeners = [
  { port: 8000, pid: 111, proc: "python", managed: false }, // foreign squatter
  { port: 9000, pid: 222, proc: "air", managed: true, owner: "wt/main" },
  { port: 7000, pid: 333, proc: "", managed: false }, // unknown proc
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
    treesApi: {
      ...actual.treesApi,
      gc: vi.fn(),
    },
  };
});

describe("MaintenancePage (Ports + GC combined)", () => {
  beforeEach(async () => {
    const { portsApi } = await import("@/api");
    vi.mocked(portsApi.listeners).mockResolvedValue(mockListeners as never);
    vi.mocked(portsApi.stale).mockResolvedValue([] as never);
    vi.mocked(portsApi.prune).mockResolvedValue({
      removed: [],
      count: 0,
    } as never);
  });

  it("renders the Ports and GC cards", async () => {
    render(<MaintenancePage />);
    await waitFor(() => {
      expect(screen.getByText("8000")).toBeInTheDocument();
    });
    expect(screen.getByText("稼働中ポート")).toBeInTheDocument();
    expect(screen.getByText("GC オプション")).toBeInTheDocument();
  });

  it("lists known-proc listening ports and classifies wt-managed vs foreign", async () => {
    render(<MaintenancePage />);
    await waitFor(() => {
      expect(screen.getByText("8000")).toBeInTheDocument();
    });
    const table = screen.getByRole("table");
    expect(within(table).getAllByText("foreign")).toHaveLength(1);
    expect(
      within(table).getByText((c) => c.includes("wt/main"))
    ).toBeInTheDocument();
  });

  it("hides unknown-proc rows by default and reveals them via the toggle", async () => {
    render(<MaintenancePage />);
    await waitFor(() => {
      expect(screen.getByText("8000")).toBeInTheDocument();
    });
    // port 7000 (proc: "") is hidden by default
    expect(screen.queryByText("7000")).not.toBeInTheDocument();
    expect(screen.getByText(/プロセス不明の行も表示する/)).toBeInTheDocument();

    fireEvent.click(
      screen.getByRole("checkbox", { name: /プロセス不明の行も表示する/ })
    );

    await waitFor(() => {
      expect(screen.getByText("7000")).toBeInTheDocument();
    });
  });

  it("lists ghost entries and prunes them on confirm", async () => {
    const { portsApi } = await import("@/api");
    vi.mocked(portsApi.stale).mockResolvedValue([
      { repo: "marchedb", wt_name: "marchedb--issue-9", port_base: 9200, port_range: "9200-9204" },
    ] as never);
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);

    render(<MaintenancePage />);
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

  it("keeps the ghost-port card open to show the success message after pruning (regression)", async () => {
    const { portsApi } = await import("@/api");
    // 初回取得は1件、掃除後の再取得（invalidateQueries）では0件に変わる。
    vi.mocked(portsApi.stale)
      .mockResolvedValueOnce([
        { repo: "marchedb", wt_name: "marchedb--issue-9", port_base: 9200, port_range: "9200-9204" },
      ] as never)
      .mockResolvedValue([] as never);
    vi.mocked(portsApi.prune).mockResolvedValue({ removed: [], count: 1 } as never);
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);

    render(<MaintenancePage />);
    await waitFor(() => {
      expect(screen.getByText("marchedb--issue-9")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /掃除して回収/ }));

    // stale が 0件に変わってもカードが自動で閉じず、成功メッセージが見える
    await waitFor(() => {
      expect(
        screen.getByText("1 件を掃除し、1 ブロックを回収しました。")
      ).toBeInTheDocument();
    });
    confirmSpy.mockRestore();
  });

  it("keeps the ghost-port card collapsed by default (0件) and shows the empty state once opened", async () => {
    render(<MaintenancePage />);
    await waitFor(() => {
      expect(screen.getByText("8000")).toBeInTheDocument();
    });
    // 0件のときは閉じたまま、件数バッジも出ない
    expect(screen.queryByText(/^\d+件$/)).not.toBeInTheDocument();
    expect(screen.queryByText("幽霊エントリはありません。")).not.toBeInTheDocument();

    fireEvent.click(screen.getByText("幽霊ポート（削除済み worktree の残骸）"));

    await waitFor(() => {
      expect(screen.getByText("幽霊エントリはありません。")).toBeInTheDocument();
    });
  });

  it("auto-expands the ghost-port card when entries are detected, with a count badge", async () => {
    const { portsApi } = await import("@/api");
    vi.mocked(portsApi.stale).mockResolvedValue([
      { repo: "marchedb", wt_name: "marchedb--issue-9", port_base: 9200, port_range: "9200-9204" },
    ] as never);

    render(<MaintenancePage />);
    await waitFor(() => {
      expect(screen.getByText("marchedb--issue-9")).toBeInTheDocument();
    });
    expect(screen.getByText("1件")).toBeInTheDocument();
  });

  it("runs a GC dry-run preview and shows the output", async () => {
    const { treesApi } = await import("@/api");
    vi.mocked(treesApi.gc).mockResolvedValue({
      output: "would remove: marchedb--issue-9",
    } as never);

    render(<MaintenancePage />);
    fireEvent.click(screen.getByRole("button", { name: /プレビュー/ }));

    await waitFor(() => {
      expect(
        screen.getByText("would remove: marchedb--issue-9")
      ).toBeInTheDocument();
    });
    expect(vi.mocked(treesApi.gc).mock.calls[0][0]).toMatchObject({
      dry_run: true,
      yes: false,
    });
    expect(screen.getByText("プレビュー結果")).toBeInTheDocument();
  });

  it("labels the output as GC実行結果 after an actual (non-dry-run) execution", async () => {
    const { treesApi } = await import("@/api");
    vi.mocked(treesApi.gc).mockResolvedValue({
      output: "removed: marchedb--issue-9",
    } as never);

    render(<MaintenancePage />);
    fireEvent.click(screen.getByRole("button", { name: "GC 実行" }));

    await waitFor(() => {
      expect(screen.getByText("removed: marchedb--issue-9")).toBeInTheDocument();
    });
    const calls = vi.mocked(treesApi.gc).mock.calls;
    expect(calls[calls.length - 1][0]).toMatchObject({
      dry_run: false,
      yes: true,
    });
    expect(screen.getByText("GC実行結果")).toBeInTheDocument();
    expect(screen.queryByText("プレビュー結果")).not.toBeInTheDocument();
  });
});
