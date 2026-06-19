import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@/test/test-utils";
import { WorktreeDetailPanel, type WorktreeDetail } from "./WorktreeDetailPanel";
import type { TreeItem } from "@/api";

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
    ...overrides,
  };
}

function renderPanel(tree: TreeItem, onDelete = vi.fn()) {
  const detail: WorktreeDetail = { tree, issueURL: null };
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
