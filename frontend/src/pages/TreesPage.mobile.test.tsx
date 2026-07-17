import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@/test/test-utils";

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  Toaster: () => null,
}));

const mockTrees = [
  {
    wt_name: "myrepo--feat-issue-1-abc",
    repo: "myrepo",
    label: "[feat] feat/issue-1-abc",
    path: "/home/user/Workspace/myrepo/myrepo--feat-issue-1-abc",
    created_at: "2024-01-01",
    diff_count: 0,
    is_main: false,
    branch: "feat/issue-1-abc",
  },
  {
    wt_name: "myrepo",
    repo: "myrepo",
    label: "[main]",
    path: "/home/user/Workspace/myrepo",
    created_at: "",
    diff_count: 0,
    is_main: true,
    branch: "main",
  },
];

vi.mock("@/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/api")>();
  return {
    ...actual,
    treesApi: {
      ...actual.treesApi,
      list: vi.fn(),
      add: vi.fn(),
      mergedPRs: vi.fn().mockResolvedValue([]),
      issueDetails: vi.fn().mockResolvedValue([]),
      delete: vi.fn().mockResolvedValue({ output: "" }),
      update: vi.fn().mockResolvedValue({ output: "" }),
    },
    reposApi: { ...actual.reposApi, list: vi.fn().mockResolvedValue([]) },
    portsApi: {
      ...actual.portsApi,
      list: vi.fn().mockResolvedValue([]),
      serve: vi.fn(),
      down: vi.fn(),
    },
  };
});

describe("TreesPage - モバイル（カード表示）", () => {
  beforeEach(async () => {
    // モバイル幅をシミュレート: useIsMobile の matchMedia が常に true を返すようにする。
    window.matchMedia = ((query: string) => ({
      matches: true,
      media: query,
      onchange: null,
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {},
      dispatchEvent: () => false,
    })) as unknown as typeof window.matchMedia;
    const { treesApi } = await import("@/api");
    vi.mocked(treesApi.list).mockResolvedValue(mockTrees as never);
  });

  it("md 未満ではテーブルではなくカード一覧を描画する", async () => {
    const { TreesPage } = await import("./TreesPage");
    render(<TreesPage />);

    await waitFor(() => {
      expect(
        screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択")
      ).toBeInTheDocument();
    });

    // デスクトップ用テーブルは描画されない。
    expect(screen.queryByRole("table")).not.toBeInTheDocument();

    // モバイルではアクションバーに「全選択」が出る（テーブルヘッダの代替）。
    expect(screen.getByLabelText("全選択")).toBeInTheDocument();

    // 各 worktree がカードとして操作可能（コピー / ピン / main 行も含む）。
    expect(
      screen.getByLabelText("myrepo--feat-issue-1-abc のパスをコピー")
    ).toBeInTheDocument();
    expect(
      screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc をピン留め")
    ).toBeInTheDocument();
    expect(screen.getByLabelText("myrepo/myrepo を選択")).toBeInTheDocument();
  });
});
