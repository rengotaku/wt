import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor, act } from "@/test/test-utils";
import { toast } from "sonner";
import { TreesPage } from "./TreesPage";

vi.mock("sonner", () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
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
    has_tmux: false,
    is_main: false,
    branch: "feat/issue-1-abc",
  },
  {
    wt_name: "myrepo--feat-issue-2-xyz",
    repo: "myrepo",
    label: "[feat] feat/issue-2-xyz",
    path: "/home/user/Workspace/myrepo/myrepo--feat-issue-2-xyz",
    created_at: "2024-01-02",
    diff_count: 0,
    has_tmux: false,
    is_main: false,
    branch: "feat/issue-2-xyz",
  },
  {
    wt_name: "myrepo",
    repo: "myrepo",
    label: "[main]",
    path: "/home/user/Workspace/myrepo",
    created_at: "",
    diff_count: 0,
    has_tmux: false,
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
    },
    reposApi: {
      ...actual.reposApi,
      list: vi.fn().mockResolvedValue([]),
    },
    portsApi: {
      ...actual.portsApi,
      list: vi.fn().mockResolvedValue([]),
      serve: vi.fn(),
      down: vi.fn(),
    },
  };
});

describe("TreesPage - checkbox and bulk action", () => {
  beforeEach(async () => {
    const { treesApi } = await import("@/api");
    vi.mocked(treesApi.list).mockResolvedValue(mockTrees as never);
  });

  it("renders checkboxes for non-main rows and a header checkbox", async () => {
    render(<TreesPage />);
    await waitFor(() => {
      expect(screen.getByLabelText("全選択")).toBeInTheDocument();
    });
    expect(
      screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択")
    ).toBeInTheDocument();
    expect(
      screen.getByLabelText("myrepo/myrepo--feat-issue-2-xyz を選択")
    ).toBeInTheDocument();
    // main row is hidden by default (showMain = false), so no checkbox for it
  });

  it("shows bulk action bar when a row is checked", async () => {
    render(<TreesPage />);
    await waitFor(() => {
      expect(
        screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択")
      ).toBeInTheDocument();
    });
    expect(screen.queryByLabelText("アクション選択")).not.toBeInTheDocument();

    fireEvent.click(screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択"));

    expect(screen.getByLabelText("アクション選択")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "実行" })).toBeInTheDocument();
    expect(screen.getByText("1 件選択中")).toBeInTheDocument();
  });

  it("selects all non-main rows via header checkbox", async () => {
    render(<TreesPage />);
    await waitFor(() => {
      expect(screen.getByLabelText("全選択")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByLabelText("全選択"));

    expect(screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択")).toBeChecked();
    expect(screen.getByLabelText("myrepo/myrepo--feat-issue-2-xyz を選択")).toBeChecked();
    expect(screen.getByText("2 件選択中")).toBeInTheDocument();
  });

  it("deselects all when header checkbox clicked while all selected", async () => {
    render(<TreesPage />);
    await waitFor(() => {
      expect(screen.getByLabelText("全選択")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByLabelText("全選択"));
    expect(screen.getByText("2 件選択中")).toBeInTheDocument();

    fireEvent.click(screen.getByLabelText("全選択"));
    expect(screen.queryByText(/件選択中/)).not.toBeInTheDocument();
  });

  it("shows confirmation modal with selected items when execute is clicked", async () => {
    render(<TreesPage />);
    await waitFor(() => {
      expect(
        screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択")
      ).toBeInTheDocument();
    });

    fireEvent.click(screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択"));
    fireEvent.click(screen.getByRole("button", { name: "実行" }));

    await waitFor(() => {
      expect(screen.getByText("削除確認")).toBeInTheDocument();
    });
    expect(screen.getByText(/myrepo \/ myrepo--feat-issue-1-abc/)).toBeInTheDocument();
    expect(
      screen.queryByText(/myrepo \/ myrepo--feat-issue-2-xyz/)
    ).not.toBeInTheDocument();
  });

  it("closes confirmation modal on cancel", async () => {
    render(<TreesPage />);
    await waitFor(() => {
      expect(
        screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択")
      ).toBeInTheDocument();
    });

    fireEvent.click(screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択"));
    fireEvent.click(screen.getByRole("button", { name: "実行" }));
    await waitFor(() => {
      expect(screen.getByText("削除確認")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: "キャンセル" }));
    expect(screen.queryByText("削除確認")).not.toBeInTheDocument();
  });
});

const newTree = {
  wt_name: "myrepo--feat-issue-3-new",
  repo: "myrepo",
  label: "[feat] feat/issue-3-new",
  path: "/home/user/Workspace/myrepo/myrepo--feat-issue-3-new",
  created_at: "2024-01-03",
  diff_count: 0,
  has_tmux: false,
  is_main: false,
  branch: "feat/issue-3-new",
};

describe("TreesPage - new row highlight and auto-scroll", () => {
  beforeEach(async () => {
    const { treesApi } = await import("@/api");
    let callCount = 0;
    vi.mocked(treesApi.list).mockImplementation(async () => {
      callCount++;
      if (callCount >= 2) return [...mockTrees, newTree] as never;
      return mockTrees as never;
    });
    vi.mocked(treesApi.add).mockResolvedValue({
      path: newTree.path,
      output: "created",
    } as never);
  });

  it("applies row-highlight class to newly added row after add", async () => {
    window.HTMLElement.prototype.scrollIntoView = vi.fn();
    render(<TreesPage />);

    await waitFor(() => {
      expect(
        screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択")
      ).toBeInTheDocument();
    });

    // フォームを開く
    fireEvent.click(screen.getByText("Worktree を追加"));
    await waitFor(() => {
      expect(
        screen.getByPlaceholderText("https://github.com/owner/repo/issues/123")
      ).toBeInTheDocument();
    });

    fireEvent.change(
      screen.getByPlaceholderText("https://github.com/owner/repo/issues/123"),
      {
        target: { value: "https://github.com/myrepo/repo/issues/3" },
      }
    );
    fireEvent.click(screen.getByRole("button", { name: "作成" }));

    await waitFor(() => {
      expect(screen.getByLabelText("myrepo--feat-issue-3-new のパスをコピー")).toBeInTheDocument();
    });

    const newRow = screen.getByLabelText("myrepo--feat-issue-3-new のパスをコピー").closest("tr");
    expect(newRow).toHaveClass("row-highlight");
  });

  it("calls scrollIntoView on the newly added row", async () => {
    const scrollIntoViewMock = vi.fn();
    window.HTMLElement.prototype.scrollIntoView = scrollIntoViewMock;

    render(<TreesPage />);

    await waitFor(() => {
      expect(
        screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択")
      ).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText("Worktree を追加"));
    await waitFor(() => {
      expect(
        screen.getByPlaceholderText("https://github.com/owner/repo/issues/123")
      ).toBeInTheDocument();
    });

    fireEvent.change(
      screen.getByPlaceholderText("https://github.com/owner/repo/issues/123"),
      {
        target: { value: "https://github.com/myrepo/repo/issues/3" },
      }
    );
    fireEvent.click(screen.getByRole("button", { name: "作成" }));

    await waitFor(() => {
      expect(screen.getByLabelText("myrepo--feat-issue-3-new のパスをコピー")).toBeInTheDocument();
    });

    expect(scrollIntoViewMock).toHaveBeenCalledWith({
      behavior: "smooth",
      block: "nearest",
    });
  });

  it("removes row-highlight class after 3 seconds", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    window.HTMLElement.prototype.scrollIntoView = vi.fn();

    render(<TreesPage />);

    await waitFor(() => {
      expect(
        screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択")
      ).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText("Worktree を追加"));
    await waitFor(() => {
      expect(
        screen.getByPlaceholderText("https://github.com/owner/repo/issues/123")
      ).toBeInTheDocument();
    });

    fireEvent.change(
      screen.getByPlaceholderText("https://github.com/owner/repo/issues/123"),
      {
        target: { value: "https://github.com/myrepo/repo/issues/3" },
      }
    );
    fireEvent.click(screen.getByRole("button", { name: "作成" }));

    await waitFor(() => {
      expect(screen.getByLabelText("myrepo--feat-issue-3-new のパスをコピー")).toBeInTheDocument();
    });

    const newRow = screen.getByLabelText("myrepo--feat-issue-3-new のパスをコピー").closest("tr");
    expect(newRow).toHaveClass("row-highlight");

    await act(async () => {
      vi.advanceTimersByTime(3000);
    });

    expect(newRow).not.toHaveClass("row-highlight");
  });

  afterEach(() => {
    vi.useRealTimers();
  });
});

describe("TreesPage - copy path toast", () => {
  beforeEach(async () => {
    const { treesApi } = await import("@/api");
    vi.mocked(treesApi.list).mockResolvedValue(mockTrees as never);

    Object.defineProperty(navigator, "clipboard", {
      value: { writeText: vi.fn().mockResolvedValue(undefined) },
      configurable: true,
      writable: true,
    });
    vi.mocked(toast.success).mockReset();
    vi.mocked(toast.error).mockReset();
  });

  it("calls toast.success with expanded path when copy button is clicked", async () => {
    render(<TreesPage />);
    await waitFor(() => {
      expect(
        screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択")
      ).toBeInTheDocument();
    });

    fireEvent.click(
      screen.getByLabelText("myrepo--feat-issue-1-abc のパスをコピー")
    );

    await waitFor(() => {
      expect(vi.mocked(toast.success)).toHaveBeenCalledWith("コピーしました", {
        description: "/home/user/Workspace/myrepo/myrepo--feat-issue-1-abc",
      });
    });
  });

  it("shows a Check icon after a successful copy", async () => {
    render(<TreesPage />);
    await waitFor(() => {
      expect(
        screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択")
      ).toBeInTheDocument();
    });

    fireEvent.click(
      screen.getByLabelText("myrepo--feat-issue-1-abc のパスをコピー")
    );

    await waitFor(() => {
      expect(vi.mocked(toast.success)).toHaveBeenCalled();
    });

    // コピー成功時はボタンが ✓ (text-green-600) に切り替わる
    await waitFor(() => {
      expect(document.querySelector(".text-green-600")).toBeInTheDocument();
    });
  });

  it("calls toast.error when clipboard write fails", async () => {
    Object.defineProperty(navigator, "clipboard", {
      value: { writeText: vi.fn().mockRejectedValue(new Error("denied")) },
      configurable: true,
      writable: true,
    });

    render(<TreesPage />);
    await waitFor(() => {
      expect(
        screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択")
      ).toBeInTheDocument();
    });

    fireEvent.click(
      screen.getByLabelText("myrepo--feat-issue-1-abc のパスをコピー")
    );

    await waitFor(() => {
      expect(vi.mocked(toast.error)).toHaveBeenCalledWith("コピーに失敗しました");
    });
  });

  it("expands copy template before writing to clipboard", async () => {
    render(<TreesPage />);
    await waitFor(() => {
      expect(screen.getByPlaceholderText("$path")).toBeInTheDocument();
    });

    fireEvent.change(screen.getByPlaceholderText("$path"), {
      target: { value: "cd $path && tmc" },
    });

    fireEvent.click(
      screen.getByLabelText("myrepo--feat-issue-1-abc のパスをコピー")
    );

    await waitFor(() => {
      expect(navigator.clipboard.writeText).toHaveBeenCalledWith(
        "cd /home/user/Workspace/myrepo/myrepo--feat-issue-1-abc && tmc"
      );
      expect(vi.mocked(toast.success)).toHaveBeenCalledWith("コピーしました", {
        description: "cd /home/user/Workspace/myrepo/myrepo--feat-issue-1-abc && tmc",
      });
    });
  });
});
