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
      update: vi.fn().mockResolvedValue({ output: "1 commits pulled" }),
      pin: vi.fn(),
      setAutoStart: vi.fn(),
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
    statsApi: {
      ...actual.statsApi,
      list: vi.fn().mockResolvedValue({ items: [], warn_bytes: 2048 * 1024 * 1024, danger_bytes: 4096 * 1024 * 1024 }),
    },
  };
});

describe("TreesPage - checkbox and bulk action", () => {
  beforeEach(async () => {
    const { treesApi } = await import("@/api");
    vi.mocked(treesApi.list).mockResolvedValue(mockTrees as never);
  });

  it("全行（main 含む）にチェックボックスとヘッダチェックボックスを表示する", async () => {
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
    // main 行にもチェックボックスが出る（最新化・ピン留め対象にできる）。
    expect(screen.getByLabelText("myrepo/myrepo を選択")).toBeInTheDocument();
  });

  it("一括アクションバーは常時表示で、選択すると実行が有効になる", async () => {
    render(<TreesPage />);
    await waitFor(() => {
      expect(
        screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択")
      ).toBeInTheDocument();
    });
    // バーは常時表示（出し入れによる画面ブレを避ける）。未選択時は実行が無効。
    expect(screen.getByLabelText("アクション選択")).toBeInTheDocument();
    expect(screen.getByText("0 件選択中")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "実行" })).toBeDisabled();

    fireEvent.click(screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択"));

    expect(screen.getByText("1 件選択中")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "実行" })).toBeEnabled();
  });

  it("ヘッダチェックボックスで main 含む全行を選択する", async () => {
    render(<TreesPage />);
    await waitFor(() => {
      expect(screen.getByLabelText("全選択")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByLabelText("全選択"));

    expect(screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択")).toBeChecked();
    expect(screen.getByLabelText("myrepo/myrepo--feat-issue-2-xyz を選択")).toBeChecked();
    expect(screen.getByLabelText("myrepo/myrepo を選択")).toBeChecked();
    expect(screen.getByText("3 件選択中")).toBeInTheDocument();
  });

  it("deselects all when header checkbox clicked while all selected", async () => {
    render(<TreesPage />);
    await waitFor(() => {
      expect(screen.getByLabelText("全選択")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByLabelText("全選択"));
    expect(screen.getByText("3 件選択中")).toBeInTheDocument();

    fireEvent.click(screen.getByLabelText("全選択"));
    expect(screen.getByText("0 件選択中")).toBeInTheDocument();
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

describe("TreesPage - dirty worktree も選択可能", () => {
  const dirtyTrees = [
    { ...mockTrees[0], diff_count: 0 },
    { ...mockTrees[1], diff_count: 4 },
  ];

  beforeEach(async () => {
    const { treesApi } = await import("@/api");
    vi.mocked(treesApi.list).mockResolvedValue(dirtyTrees as never);
  });

  it("dirty 行のチェックボックスも有効で、全選択で両方選ばれる", async () => {
    render(<TreesPage />);
    await waitFor(() => {
      expect(screen.getByLabelText("全選択")).toBeInTheDocument();
    });

    expect(
      screen.getByLabelText("myrepo/myrepo--feat-issue-2-xyz を選択")
    ).toBeEnabled();

    fireEvent.click(screen.getByLabelText("全選択"));

    // dirty 行も含めて 2 件選択される
    expect(screen.getByText("2 件選択中")).toBeInTheDocument();
    expect(
      screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択")
    ).toBeChecked();
    expect(
      screen.getByLabelText("myrepo/myrepo--feat-issue-2-xyz を選択")
    ).toBeChecked();
  });
});

describe("TreesPage - バルク削除確認モーダル", () => {
  const trees = [
    { ...mockTrees[0], diff_count: 0 },
    { ...mockTrees[1], wt_name: "myrepo--dirty", diff_count: 4, branch: "feat/dirty" },
  ];

  beforeEach(async () => {
    const { treesApi } = await import("@/api");
    vi.mocked(treesApi.list).mockResolvedValue(trees as never);
    vi.mocked(treesApi.delete).mockClear();
  });

  it("すべて clean な選択ではシンプルな確認モーダルで force=false で削除する", async () => {
    const { treesApi } = await import("@/api");
    render(<TreesPage />);
    await waitFor(() => {
      expect(
        screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択")
      ).toBeInTheDocument();
    });

    // clean な行だけ選択
    fireEvent.click(screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択"));
    fireEvent.click(screen.getByRole("button", { name: "実行" }));

    await waitFor(() => {
      expect(screen.getByText("削除確認")).toBeInTheDocument();
    });

    // dirty が含まれないので yes 入力は不要、"削除" ボタンが表示される
    expect(screen.queryByLabelText("削除確認のため yes と入力")).not.toBeInTheDocument();
    const delBtn = screen.getByRole("button", { name: "削除" });
    expect(delBtn).toBeEnabled();

    fireEvent.click(delBtn);
    await waitFor(() => {
      expect(treesApi.delete).toHaveBeenCalledWith({
        repo: "myrepo",
        branch: "feat/issue-1-abc",
        force: false,
      });
    });
  });

  it("dirty 含む選択では yes 入力が一致するまで強制削除できない", async () => {
    const { treesApi } = await import("@/api");
    render(<TreesPage />);
    await waitFor(() => {
      expect(
        screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択")
      ).toBeInTheDocument();
    });

    // 両方選択（dirty 含む）
    fireEvent.click(screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択"));
    fireEvent.click(screen.getByLabelText("myrepo/myrepo--dirty を選択"));
    fireEvent.click(screen.getByRole("button", { name: "実行" }));

    await waitFor(() => {
      expect(screen.getByText("削除確認")).toBeInTheDocument();
    });

    // yes 入力が必要
    const input = screen.getByLabelText("削除確認のため yes と入力");
    expect(input).toBeInTheDocument();

    const forceBtn = screen.getByRole("button", { name: "強制削除" });
    expect(forceBtn).toBeDisabled();

    // 間違った文字列では有効化されない
    fireEvent.change(input, { target: { value: "no" } });
    expect(forceBtn).toBeDisabled();

    // 正しい文字列で有効化
    fireEvent.change(input, { target: { value: "yes" } });
    expect(forceBtn).toBeEnabled();

    fireEvent.click(forceBtn);
    await waitFor(() => {
      // clean なものは force=false、dirty なものは force=true
      expect(treesApi.delete).toHaveBeenCalledWith({
        repo: "myrepo",
        branch: "feat/issue-1-abc",
        force: false,
      });
      expect(treesApi.delete).toHaveBeenCalledWith({
        repo: "myrepo",
        branch: "feat/dirty",
        force: true,
      });
    });
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

describe("TreesPage - 一括最新化", () => {
  beforeEach(async () => {
    const { treesApi } = await import("@/api");
    vi.mocked(treesApi.list).mockResolvedValue(mockTrees as never);
    vi.mocked(treesApi.update).mockClear();
  });

  it("チェックして『最新化』を実行すると選択行を pull する", async () => {
    const { treesApi } = await import("@/api");
    render(<TreesPage />);
    await waitFor(() => {
      expect(
        screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択")
      ).toBeInTheDocument();
    });

    fireEvent.click(screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択"));
    fireEvent.change(screen.getByLabelText("アクション選択"), {
      target: { value: "update" },
    });
    fireEvent.click(screen.getByRole("button", { name: "実行" }));

    await waitFor(() => {
      expect(treesApi.update).toHaveBeenCalledWith(
        "myrepo",
        "myrepo--feat-issue-1-abc"
      );
    });
  });
});

describe("TreesPage - main は削除対象外", () => {
  beforeEach(async () => {
    const { treesApi } = await import("@/api");
    vi.mocked(treesApi.list).mockResolvedValue(mockTrees as never);
    vi.mocked(treesApi.delete).mockClear();
  });

  it("main だけ選択して削除を実行すると対象外で確認モーダルが出ない", async () => {
    render(<TreesPage />);
    const mainCb = await waitFor(() => screen.getByLabelText("myrepo/myrepo を選択"));
    fireEvent.click(mainCb);
    // 削除アクションのまま実行
    fireEvent.click(screen.getByRole("button", { name: "実行" }));
    // main は削除対象外なので確認モーダルは出ない
    await waitFor(() => {
      expect(screen.queryByText("削除確認")).not.toBeInTheDocument();
    });
  });

  it("main と非 main を選択して削除すると非 main のみ確認に出る", async () => {
    render(<TreesPage />);
    await waitFor(() => screen.getByLabelText("myrepo/myrepo を選択"));
    fireEvent.click(screen.getByLabelText("myrepo/myrepo を選択"));
    fireEvent.click(screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択"));
    fireEvent.click(screen.getByRole("button", { name: "実行" }));

    await waitFor(() => {
      expect(screen.getByText("削除確認")).toBeInTheDocument();
    });
    // 非 main は一覧に出る
    expect(screen.getByText(/myrepo \/ myrepo--feat-issue-1-abc/)).toBeInTheDocument();
    // main 除外の注記が出る
    expect(
      screen.getByText(/main\/master 1 件は削除対象外/)
    ).toBeInTheDocument();
  });
});

describe("TreesPage - ピン留め", () => {
  const path1 = "/home/user/Workspace/myrepo/myrepo--feat-issue-1-abc";

  // pin はサーバ(.worktrees.json)に永続化される。テストでは pin 状態を保持し、
  // list は pinned を反映し、pin はその状態を書き換える（= 実サーバの代役）。
  let pinnedByPath: Record<string, boolean>;

  beforeEach(async () => {
    pinnedByPath = {};
    const { treesApi } = await import("@/api");
    vi.mocked(treesApi.list).mockImplementation(
      async () =>
        mockTrees.map((t) => ({ ...t, pinned: !!pinnedByPath[t.path] })) as never
    );
    vi.mocked(treesApi.pin).mockImplementation(async (repo, wt, pinned) => {
      const t = mockTrees.find((x) => x.repo === repo && x.wt_name === wt);
      if (t) pinnedByPath[t.path] = pinned;
      return { pinned };
    });
  });

  it("チェックして『ピン留め』を実行するとピンが付き先頭に並ぶ", async () => {
    const { treesApi } = await import("@/api");
    render(<TreesPage />);
    await waitFor(() => {
      expect(
        screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択")
      ).toBeInTheDocument();
    });

    // 既定の並びは作成日降順なので issue-2(01-02) が issue-1(01-01) より上。
    const before = screen.getAllByRole("row");
    expect(before[1]).toHaveTextContent("myrepo--feat-issue-2-xyz");

    // issue-1 を選択し、アクションを「ピン留め」にして実行。
    fireEvent.click(screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc を選択"));
    fireEvent.change(screen.getByLabelText("アクション選択"), {
      target: { value: "pin" },
    });
    fireEvent.click(screen.getByRole("button", { name: "実行" }));

    // サーバへ pin 永続化が呼ばれ、再取得後にピン解除ボタン（=付与の証跡）が出る。
    await waitFor(() => {
      expect(
        screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc のピンを解除")
      ).toBeInTheDocument();
    });
    expect(treesApi.pin).toHaveBeenCalledWith("myrepo", "myrepo--feat-issue-1-abc", true);

    // ピン留めにより issue-1 が先頭へ。
    const after = screen.getAllByRole("row");
    expect(after[1]).toHaveTextContent("myrepo--feat-issue-1-abc");
  });

  it("各行の常時表示ピンボタンのクリックで直接ピン留めできる", async () => {
    const { treesApi } = await import("@/api");
    render(<TreesPage />);
    // 未ピン行にも「ピン留め」ボタンが常時出ている（チェック不要）。
    const pinBtn = await waitFor(() =>
      screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc をピン留め")
    );
    fireEvent.click(pinBtn);

    await waitFor(() => {
      expect(
        screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc のピンを解除")
      ).toBeInTheDocument();
    });
    expect(treesApi.pin).toHaveBeenCalledWith("myrepo", "myrepo--feat-issue-1-abc", true);
  });

  it("main 行もピン留めできる", async () => {
    const { treesApi } = await import("@/api");
    render(<TreesPage />);
    // main(wt_name=myrepo) にも常時ピンボタンが出る。
    const pinMain = await waitFor(() =>
      screen.getByLabelText("myrepo/myrepo をピン留め")
    );
    fireEvent.click(pinMain);

    await waitFor(() => {
      expect(
        screen.getByLabelText("myrepo/myrepo のピンを解除")
      ).toBeInTheDocument();
    });
    expect(treesApi.pin).toHaveBeenCalledWith("myrepo", "myrepo", true);
  });

  it("サーバ側で既にピンされた worktree を先頭固定し、アイコンのクリックで解除できる", async () => {
    const { treesApi } = await import("@/api");
    pinnedByPath[path1] = true; // サーバが pinned 状態で返す
    render(<TreesPage />);

    const unpin = await waitFor(() =>
      screen.getByLabelText("myrepo/myrepo--feat-issue-1-abc のピンを解除")
    );
    fireEvent.click(unpin);

    await waitFor(() => {
      expect(
        screen.queryByLabelText("myrepo/myrepo--feat-issue-1-abc のピンを解除")
      ).not.toBeInTheDocument();
    });
    expect(treesApi.pin).toHaveBeenCalledWith(
      "myrepo",
      "myrepo--feat-issue-1-abc",
      false
    );
  });
});

describe("TreesPage - 詳細パネルの自動起動トグル即時反映", () => {
  const path1 = "/home/user/Workspace/myrepo/myrepo--feat-issue-1-abc";

  // setAutoStart はサーバ(.worktrees.json)に永続化される。テストでは pin と同じ
  // 「サーバの代役」パターンで auto_start 状態を保持し、list がそれを反映する。
  let autoStartByPath: Record<string, boolean>;

  beforeEach(async () => {
    autoStartByPath = {};
    const { treesApi } = await import("@/api");
    vi.mocked(treesApi.list).mockImplementation(
      async () =>
        mockTrees.map((t) => ({ ...t, auto_start: !!autoStartByPath[t.path] })) as never
    );
    vi.mocked(treesApi.setAutoStart).mockImplementation(async (repo, wt, autoStart) => {
      const t = mockTrees.find((x) => x.repo === repo && x.wt_name === wt);
      if (t) autoStartByPath[t.path] = autoStart;
      return { auto_start: autoStart };
    });
  });

  it("パネルを開いたままトグルを押すと閉じ開き不要で ON/OFF 表示が即時反映される", async () => {
    const { treesApi } = await import("@/api");
    render(<TreesPage />);

    // 行のインタラクティブ要素以外をクリックして詳細パネルを開く。
    const nameCell = await waitFor(() =>
      screen.getByText("myrepo--feat-issue-1-abc")
    );
    fireEvent.click(nameCell);

    const toggle = await waitFor(() =>
      screen.getByRole("switch", { name: "自動起動の ON/OFF" })
    );
    expect(toggle).toHaveAttribute("aria-checked", "false");

    // パネルを閉じずにトグルを押す。
    fireEvent.click(toggle);

    await waitFor(() => {
      expect(treesApi.setAutoStart).toHaveBeenCalledWith(
        "myrepo",
        "myrepo--feat-issue-1-abc",
        true
      );
    });

    // 閉じ開きせず、同じパネルの表示が ON に切り替わること。
    await waitFor(() => {
      expect(
        screen.getByRole("switch", { name: "自動起動の ON/OFF" })
      ).toHaveAttribute("aria-checked", "true");
    });
    // パネル自体が閉じていない（詳細ヘッダがまだ見える）ことも確認する。
    expect(screen.getByRole("heading", { name: "myrepo--feat-issue-1-abc" })).toBeInTheDocument();
  });

  it("サーバ側で既に auto_start な worktree はパネルを開くと ON 表示になる", async () => {
    autoStartByPath[path1] = true;
    render(<TreesPage />);

    const nameCell = await waitFor(() =>
      screen.getByText("myrepo--feat-issue-1-abc")
    );
    fireEvent.click(nameCell);

    const toggle = await waitFor(() =>
      screen.getByRole("switch", { name: "自動起動の ON/OFF" })
    );
    expect(toggle).toHaveAttribute("aria-checked", "true");
  });
});

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
    fireEvent.click(screen.getByRole("button", { name: "追加" }));
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

    fireEvent.click(screen.getByRole("button", { name: "追加" }));
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

    fireEvent.click(screen.getByRole("button", { name: "追加" }));
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

  it("copies the raw worktree path to the clipboard", async () => {
    render(<TreesPage />);
    await waitFor(() => {
      expect(
        screen.getByLabelText("myrepo--feat-issue-1-abc のパスをコピー")
      ).toBeInTheDocument();
    });

    fireEvent.click(
      screen.getByLabelText("myrepo--feat-issue-1-abc のパスをコピー")
    );

    await waitFor(() => {
      expect(navigator.clipboard.writeText).toHaveBeenCalledWith(
        "/home/user/Workspace/myrepo/myrepo--feat-issue-1-abc"
      );
      expect(vi.mocked(toast.success)).toHaveBeenCalledWith("コピーしました", {
        description: "/home/user/Workspace/myrepo/myrepo--feat-issue-1-abc",
      });
    });
  });
});

describe("TreesPage - process stats", () => {
  beforeEach(async () => {
    const { treesApi, statsApi } = await import("@/api");
    vi.mocked(treesApi.list).mockResolvedValue(mockTrees as never);
    vi.mocked(statsApi.list).mockResolvedValue({
      warn_bytes: 2 * 1024 * 1024 * 1024,
      danger_bytes: 4 * 1024 * 1024 * 1024,
      items: [
        {
          repo: "myrepo",
          wt_name: "myrepo--feat-issue-1-abc",
          level: "ok",
          total_rss_bytes: 1024 * 1024 * 500, // 500MB -> 500M
          services: [
            { name: "web", pid: 1234, port: 9000, alive: true, procs: 1, rss_bytes: 1024 * 1024 * 500, uptime_sec: 100 },
          ],
        },
        {
          repo: "myrepo",
          wt_name: "myrepo--feat-issue-2-xyz",
          level: "danger",
          total_rss_bytes: 1024 * 1024 * 5000, // 5000MB -> 4.9G
          services: [
            { name: "db", pid: 5678, port: 9001, alive: true, procs: 2, rss_bytes: 1024 * 1024 * 5000, uptime_sec: 200 },
          ],
        },
      ],
    } as never);
  });

  it("状態列に合計メモリが表示され、danger 行には背景色が付く", async () => {
    render(<TreesPage />);
    
    // ①状態列に合計メモリが表示される
    const okBtn = await waitFor(() => screen.getByText("500M"));
    expect(okBtn).toBeInTheDocument();
    
    // formatBytes(5000 * 1024 * 1024) is 5000 / 1024 = 4.88 -> 4.9G
    const dangerBtn = screen.getByText("4.9G");
    expect(dangerBtn).toBeInTheDocument();

    // ②danger の行に bg-red-500/10 が付く
    const dangerRow = dangerBtn.closest("tr");
    expect(dangerRow).toHaveClass("bg-red-500/10");
  });

  it("状態リンククリックでオーバーレイが開きサービス名と RSS が見える", async () => {
    render(<TreesPage />);
    
    const okBtn = await waitFor(() => screen.getByText("500M"));
    fireEvent.click(okBtn);

    // ③状態リンククリックでオーバーレイが開きサービス名と RSS が見える
    await waitFor(() => {
      expect(screen.getByText("myrepo/myrepo--feat-issue-1-abc")).toBeInTheDocument();
    });
    
    expect(screen.getByText("web")).toBeInTheDocument();
    expect(screen.getAllByText("500M").length).toBeGreaterThan(0);
  });
});
