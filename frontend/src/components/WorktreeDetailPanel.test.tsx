import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@/test/test-utils";
import { WorktreeDetailPanel, type WorktreeDetail } from "./WorktreeDetailPanel";
import type { TreeItem, PortItem } from "@/api";

function makeTree(overrides: Partial<TreeItem> = {}): TreeItem {
  return {
    wt_name: "myrepo--feat-1",
    repo: "myrepo",
    label: "[feat] feat/1",
    path: "/home/user/Workspace/myrepo/myrepo--feat-1",
    created_at: "2024-01-01",
    diff_count: 0,
    has_tmux: false,
    is_main: false,
    branch: "feat/1",
    pinned: false,
    ...overrides,
  };
}

function renderPanel(tree: TreeItem, onDelete = vi.fn(), port?: PortItem) {
  const detail: WorktreeDetail = { tree, issueURL: null, port };
  render(
    <WorktreeDetailPanel
      detail={detail}
      onClose={vi.fn()}
      onServe={vi.fn()}
      onDown={vi.fn()}
      onEditConfig={vi.fn()}
      onShowLogs={vi.fn()}
      onUpdate={vi.fn()}
      onDelete={onDelete}
      updating={false}
      deleting={false}
      portBusy={false}
    />,
  );
  return { onDelete };
}

describe("WorktreeDetailPanel 削除", () => {
  it("clean な worktree は確認後そのまま削除でき force=false を渡す", () => {
    const { onDelete } = renderPanel(makeTree({ diff_count: 0 }));

    fireEvent.click(screen.getByRole("button", { name: "worktree を削除" }));

    const del = screen.getByRole("button", { name: "削除" });
    expect(del).toBeEnabled();
    fireEvent.click(del);
    expect(onDelete).toHaveBeenCalledWith(false);
  });

  it("dirty な worktree は名前入力が一致するまで強制削除できない", () => {
    const tree = makeTree({ diff_count: 3, wt_name: "myrepo--dirty" });
    const { onDelete } = renderPanel(tree);

    fireEvent.click(screen.getByRole("button", { name: "worktree を削除" }));

    const forceBtn = screen.getByRole("button", { name: "強制削除" });
    expect(forceBtn).toBeDisabled();

    const input = screen.getByLabelText("削除確認のため worktree 名を入力");
    fireEvent.change(input, { target: { value: "wrong" } });
    expect(forceBtn).toBeDisabled();

    fireEvent.change(input, { target: { value: "myrepo--dirty" } });
    expect(forceBtn).toBeEnabled();

    fireEvent.click(forceBtn);
    expect(onDelete).toHaveBeenCalledWith(true);
  });
});

describe("WorktreeDetailPanel 縮退稼働", () => {
  it("degraded な port では縮退稼働の警告を表示する", () => {
    const port: PortItem = {
      repo: "myrepo",
      wt_name: "myrepo--feat-1",
      port_base: 9000,
      port_range: "9000-9001",
      ports: [],
      has_dev_config: true,
      running: true,
      degraded: true,
    };
    renderPanel(makeTree(), vi.fn(), port);
    expect(
      screen.getByText(/一部のサービスが正常に稼働していません（縮退稼働）/),
    ).toBeInTheDocument();
  });
});

describe("WorktreeDetailPanel 稼働ポート表示", () => {
  it("listening は開くリンク、headless worker (headless 宣言) は no port テキストで表示する", () => {
    const port: PortItem = {
      repo: "myrepo",
      wt_name: "myrepo--feat-1",
      port_base: 9000,
      port_range: "9000-9001",
      ports: [
        { port: 9000, listening: true, service: "api" },
        { port: 9001, listening: false, running: true, headless: true, service: "worker" },
      ],
      has_dev_config: true,
      running: true,
    };
    renderPanel(makeTree(), vi.fn(), port);

    const apiLink = screen.getByRole("link", { name: "api:9000" });
    expect(apiLink).toHaveAttribute("href", "http://localhost:9000");

    // worker は開けないのでリンクではなくテキスト。
    expect(screen.queryByRole("link", { name: /worker/ })).toBeNull();
    expect(screen.getByText("worker (no port)")).toBeInTheDocument();
  });

  it("LISTEN すべきなのに未 LISTEN のサービス (unhealthy) は警告表示にし、no port と混同しない", () => {
    const port: PortItem = {
      repo: "myrepo",
      wt_name: "myrepo--feat-1",
      port_base: 9000,
      port_range: "9000-9001",
      ports: [{ port: 9000, listening: false, running: true, unhealthy: true, service: "go" }],
      has_dev_config: true,
      running: true,
      degraded: true,
    };
    renderPanel(makeTree(), vi.fn(), port);

    // 開けないのでリンクではない。良性の「(no port)」でもない。
    expect(screen.queryByRole("link", { name: /go/ })).toBeNull();
    expect(screen.queryByText("go (no port)")).toBeNull();
    expect(screen.getByText("go ⚠ 未LISTEN")).toBeInTheDocument();
    // 縮退稼働の warning banner も出る。
    expect(
      screen.getByText(/一部のサービスが正常に稼働していません（縮退稼働）/),
    ).toBeInTheDocument();
  });
});

describe("WorktreeDetailPanel 最新化", () => {
  it("dirty な worktree では最新化ボタンが無効", () => {
    renderPanel(makeTree({ diff_count: 3 }));
    expect(screen.getByRole("button", { name: "最新化" })).toBeDisabled();
  });

  it("clean な worktree では最新化ボタンが有効", () => {
    renderPanel(makeTree({ diff_count: 0 }));
    expect(screen.getByRole("button", { name: "最新化" })).toBeEnabled();
  });
});
