import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@/test/test-utils";
import { TreesPage } from "./TreesPage";

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
      mergedPRs: vi.fn().mockResolvedValue([]),
      issueDetails: vi.fn().mockResolvedValue([]),
      delete: vi.fn().mockResolvedValue({ output: "" }),
    },
    reposApi: {
      ...actual.reposApi,
      list: vi.fn().mockResolvedValue([]),
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
    expect(screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択")).toBeInTheDocument();
    expect(screen.getByLabelText("myrepo/myrepo--feat-issue-2-xyz を選択")).toBeInTheDocument();
    // main row is hidden by default (showMain = false), so no checkbox for it
  });

  it("shows bulk action bar when a row is checked", async () => {
    render(<TreesPage />);
    await waitFor(() => {
      expect(screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択")).toBeInTheDocument();
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
      expect(screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択"));
    fireEvent.click(screen.getByRole("button", { name: "実行" }));

    await waitFor(() => {
      expect(screen.getByText("削除確認")).toBeInTheDocument();
    });
    expect(screen.getByText(/myrepo \/ myrepo--feat-issue-1-abc/)).toBeInTheDocument();
    expect(screen.queryByText(/myrepo \/ myrepo--feat-issue-2-xyz/)).not.toBeInTheDocument();
  });

  it("closes confirmation modal on cancel", async () => {
    render(<TreesPage />);
    await waitFor(() => {
      expect(screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択")).toBeInTheDocument();
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
