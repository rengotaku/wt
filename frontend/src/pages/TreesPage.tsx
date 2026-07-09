import { useState, useEffect, useRef, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { toast } from "sonner";
import {
  RefreshCw,
  Copy,
  Check,
  Pin,
  Plus,
  X,
} from "lucide-react";
import {
  treesApi,
  reposApi,
  portsApi,
  statsApi,
  formatBytes,
  type AddTreeRequest,
  type TreeItem,
  type MergedPRInfo,
  type IssueDetail,
  type PortItem,
  type ProcessStatsResponse,
  type WorktreeProcessStats,
} from "@/api";
import { filterTrees } from "./treesFilter";
import { DevConfigPanel, type DevConfigTarget } from "@/components/DevConfigPanel";
import { LogPanel, type LogTarget } from "@/components/LogPanel";
import { WorktreeDetailPanel } from "@/components/WorktreeDetailPanel";
import { WorktreeCard } from "@/components/WorktreeCard";
import { ProcessStatsOverlay } from "@/components/ProcessStatsOverlay";
import { useIsMobile } from "@/hooks";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Alert, AlertDescription } from "@/components/ui/alert";

// ページ遷移をまたいで保持するモジュールレベルキャッシュ
const prCache = new Map<string, MergedPRInfo[]>();
const issueCache = new Map<string, IssueDetail[]>();

// navigator.clipboard が使えない環境向けの execCommand フォールバック。成否を返す。
function fallbackCopy(text: string): boolean {
  try {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.select();
    const ok = document.execCommand("copy");
    document.body.removeChild(ta);
    return ok;
  } catch {
    return false;
  }
}

export function TreesPage() {
  const [searchParams] = useSearchParams();
  // md(768px) 未満ではテーブルの代わりにカード一覧を描画する（デスクトップは現状維持）。
  const isMobile = useIsMobile();

  const {
    data: trees = [],
    isLoading,
    refetch,
  } = useQuery({
    queryKey: ["trees"],
    queryFn: treesApi.list,
    staleTime: Infinity,
    refetchOnWindowFocus: false,
  });
  const { data: repos = [] } = useQuery({
    queryKey: ["repos"],
    queryFn: reposApi.list,
    staleTime: Infinity,
    refetchOnWindowFocus: false,
  });
  const { data: portItems = [] } = useQuery<PortItem[]>({
    queryKey: ["ports"],
    queryFn: portsApi.list,
    refetchOnWindowFocus: false,
    // 稼働中の worktree がある間だけ 3 秒間隔でポーリングする。serve 直後は
    // uvicorn(--reload) のように bind が遅いサービスがあり、起動完了前の 1 回の
    // 再取得では listening=false のまま固定されてしまう。running 中はポーリングして
    // 遅れて起動したポートも「稼働」表示に追従させる（idle 時は止めて無駄打ちを避ける）。
    refetchInterval: (query) =>
      (query.state.data as PortItem[] | undefined)?.some((p) => p.running)
        ? 3000
        : false,
  });
  const portMap = new Map(portItems.map((p) => [`${p.repo}/${p.wt_name}`, p]));

  const { data: statsData } = useQuery<ProcessStatsResponse>({
    queryKey: ["process-stats"],
    queryFn: statsApi.list,
    refetchOnWindowFocus: false,
    refetchInterval: () => (portItems.some((p) => p.running) ? 10000 : false),
  });

  const statsMap = useMemo(() => {
    const map = new Map<string, WorktreeProcessStats>();
    if (statsData?.items) {
      for (const item of statsData.items) {
        map.set(`${item.repo}/${item.wt_name}`, item);
      }
    }
    return map;
  }, [statsData]);

  const queryClient = useQueryClient();
  const serveMutation = useMutation({
    mutationFn: ({ repo, wt }: { repo: string; wt: string }) => portsApi.serve(repo, wt),
    onSuccess: (_r, v) => {
      toast.success(`${v.wt} を起動しました`);
      queryClient.invalidateQueries({ queryKey: ["ports"] });
    },
    onError: (e: Error) => toast.error("起動に失敗しました", { description: e.message }),
  });
  const downMutation = useMutation({
    mutationFn: ({ repo, wt }: { repo: string; wt: string }) => portsApi.down(repo, wt),
    onSuccess: (_r, v) => {
      toast.success(`${v.wt} を停止しました`);
      queryClient.invalidateQueries({ queryKey: ["ports"] });
    },
    onError: (e: Error) => toast.error("停止に失敗しました", { description: e.message }),
  });
  const portBusy = serveMutation.isPending || downMutation.isPending;
  const [devConfigTarget, setDevConfigTarget] = useState<DevConfigTarget | null>(null);
  const [logTarget, setLogTarget] = useState<LogTarget | null>(null);
  const [overlayStats, setOverlayStats] = useState<WorktreeProcessStats | null>(null);
  const [detailTree, setDetailTree] = useState<TreeItem | null>(null);
  const deleteMutation = useMutation({
    mutationFn: ({ repo, branch, force }: { repo: string; branch: string; force: boolean }) =>
      treesApi.delete({ repo, branch, force }),
    onSuccess: (_r, v) => {
      toast.success(`${v.branch} を削除しました`);
      setDetailTree(null);
      refetch();
    },
    onError: (e: Error) => toast.error("削除に失敗しました", { description: e.message }),
  });
  const updateMutation = useMutation({
    mutationFn: ({ repo, wt }: { repo: string; wt: string }) => treesApi.update(repo, wt),
    onSuccess: (r, v) => {
      const desc = r.restarted ? `${r.output}\n\ndev サービスを再起動しました` : r.output;
      toast.success(`${v.wt} を最新化しました`, { description: desc });
      queryClient.invalidateQueries({ queryKey: ["ports"] });
      refetch();
    },
    onError: (e: Error) => toast.error("最新化に失敗しました", { description: e.message }),
  });

  // Issue / 親issue / PR 列の表示。既定は非表示。localStorage に永続化。
  const [showCols, setShowCols] = useState<{
    issue: boolean;
    parentIssue: boolean;
    pr: boolean;
  }>(() => {
    try {
      const saved = localStorage.getItem("wt.trees.cols");
      if (saved) return JSON.parse(saved);
    } catch {
      /* ignore */
    }
    return { issue: false, parentIssue: false, pr: false };
  });
  const toggleCol = (key: "issue" | "parentIssue" | "pr") =>
    setShowCols((prev) => {
      const next = { ...prev, [key]: !prev[key] };
      try {
        localStorage.setItem("wt.trees.cols", JSON.stringify(next));
      } catch {
        /* ignore */
      }
      return next;
    });

  // ピン留めは .worktrees.json に永続化され、サーバから tree.pinned として返る。
  // wt web 起動時の auto-serve もこのフラグを参照する。一覧の先頭固定はこの
  // path 集合で行う（サーバ状態から導出）。
  const pinnedPaths = useMemo(
    () => new Set(trees.filter((t) => t.pinned).map((t) => t.path)),
    [trees]
  );
  // 指定 path 群のピンを付与/解除し、サーバへ永続化してから一覧を再取得する。
  const applyPin = async (paths: string[], pin: boolean) => {
    const byPath = new Map(trees.map((t) => [t.path, t]));
    try {
      await Promise.all(
        paths.map((p) => {
          const t = byPath.get(p);
          return t ? treesApi.pin(t.repo, t.wt_name, pin) : Promise.resolve();
        })
      );
    } catch {
      toast.error("ピンの更新に失敗しました");
    }
    refetch();
  };

  const [formOpen, setFormOpen] = useState(false);
  const [issueMode, setIssueMode] = useState(true);
  const [form, setForm] = useState<AddTreeRequest>({
    repo: "",
    branch: "",
    type: "feature",
  });
  const [addError, setAddError] = useState("");

  const [filterText, setFilterText] = useState("");
  const initialRepoFilter = searchParams.get("repo") ?? "";
  const [showMain, setShowMain] = useState(true);
  const [repoFilter, setRepoFilter] = useState(initialRepoFilter);

  const [selectedPaths, setSelectedPaths] = useState<Set<string>>(new Set());
  const [bulkAction, setBulkAction] = useState("delete");
  const [bulkConfirming, setBulkConfirming] = useState(false);
  const [bulkConfirmInput, setBulkConfirmInput] = useState("");
  const [bulkDeleting, setBulkDeleting] = useState(false);
  const [bulkUpdating, setBulkUpdating] = useState(false);
  const headerCheckboxRef = useRef<HTMLInputElement>(null);
  // モバイルの全選択 checkbox（アクションバー側）。indeterminate 表示に使う。
  const mobileSelectAllRef = useRef<HTMLInputElement>(null);

  const [newlyAddedPath, setNewlyAddedPath] = useState<string | null>(null);
  // テーブル行(tr)／モバイルのカード(div)どちらの DOM も登録できるよう HTMLElement で保持。
  const rowRefs = useRef<Map<string, HTMLElement>>(new Map());

  // コピー直後にボタンを ✓ 表示へ切り替えるための状態（行ごと、1.5秒で戻す）
  const [copiedPath, setCopiedPath] = useState<string | null>(null);
  const copyResetRef = useRef<number>(0);

  // キャッシュから初期化 → マウント後に未取得分を順次フェッチ
  const [prData, setPrData] = useState<Record<string, MergedPRInfo[]>>(() =>
    Object.fromEntries(prCache.entries())
  );
  const [loadingPRRepos, setLoadingPRRepos] = useState<Set<string>>(new Set());

  const [issueData, setIssueData] = useState<Record<string, IssueDetail[]>>(() =>
    Object.fromEntries(issueCache.entries())
  );
  const [loadingIssueRepos, setLoadingIssueRepos] = useState<Set<string>>(new Set());

  const [refreshKey, setRefreshKey] = useState(0);

  useEffect(() => {
    if (trees.length === 0) return;
    const uniqueRepos = [...new Set(trees.map((t) => t.repo))];
    uniqueRepos.forEach(async (repo) => {
      // PR fetch
      if (!prCache.has(repo)) {
        setLoadingPRRepos((prev) => new Set([...prev, repo]));
        try {
          const prs = await treesApi.mergedPRs(repo);
          prCache.set(repo, prs);
          setPrData((prev) => ({ ...prev, [repo]: prs }));
        } catch {
          prCache.set(repo, []);
          setPrData((prev) => ({ ...prev, [repo]: [] }));
        } finally {
          setLoadingPRRepos((prev) => {
            const n = new Set(prev);
            n.delete(repo);
            return n;
          });
        }
      }
      // Issue detail fetch
      if (!issueCache.has(repo)) {
        setLoadingIssueRepos((prev) => new Set([...prev, repo]));
        try {
          const details = await treesApi.issueDetails(repo);
          issueCache.set(repo, details);
          setIssueData((prev) => ({ ...prev, [repo]: details }));
        } catch {
          issueCache.set(repo, []);
          setIssueData((prev) => ({ ...prev, [repo]: [] }));
        } finally {
          setLoadingIssueRepos((prev) => {
            const n = new Set(prev);
            n.delete(repo);
            return n;
          });
        }
      }
    });
  }, [trees, refreshKey]);

  const handleRefresh = () => {
    prCache.clear();
    issueCache.clear();
    setPrData({});
    setIssueData({});
    setLoadingPRRepos(new Set());
    setLoadingIssueRepos(new Set());
    setRefreshKey((k) => k + 1);
  };

  useEffect(() => {
    if (!newlyAddedPath) return;
    const el = rowRefs.current.get(newlyAddedPath);
    if (!el) return;
    el.scrollIntoView({ behavior: "smooth", block: "nearest" });
    const timer = setTimeout(() => setNewlyAddedPath(null), 3000);
    return () => clearTimeout(timer);
  }, [trees, newlyAddedPath]);

  const addMutation = useMutation({
    mutationFn: treesApi.add,
    onSuccess: (data) => {
      setNewlyAddedPath(data.path);
      refetch();
      setForm({ repo: "", branch: "", type: "feature" });
      setAddError("");
      setFormOpen(false);
    },
    onError: (e: Error) => setAddError(e.message),
  });

  const handleAdd = () => {
    if (issueMode) {
      addMutation.mutate({ issue_url: form.issue_url });
    } else {
      addMutation.mutate({ repo: form.repo, branch: form.branch, type: form.type });
    }
  };

  const handleCopyPath = async (path: string) => {
    const text = path;
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      // navigator.clipboard が使えない環境（非セキュアコンテキスト等）でも握りつぶさず
      // フォールバックを試み、それでも失敗ならエラーを通知する。
      const ok = fallbackCopy(text);
      if (!ok) {
        toast.error("コピーに失敗しました");
        return;
      }
    }
    setCopiedPath(path);
    window.clearTimeout(copyResetRef.current);
    copyResetRef.current = window.setTimeout(() => setCopiedPath(null), 1500);
    toast.success("コピーしました", { description: text });
  };

  const repoURLMap = Object.fromEntries(repos.map((r) => [r.name, r.github_url ?? ""]));

  const getIssueURL = (t: TreeItem): string | null => {
    if (!t.issue) return null;
    const base = repoURLMap[t.repo];
    if (!base) return null;
    return `${base}/issues/${t.issue.replace("#", "")}`;
  };

  const getPR = (t: TreeItem): MergedPRInfo | undefined => {
    if (!t.branch) return undefined;
    return prData[t.repo]?.find((pr) => pr.head_ref_name === t.branch);
  };

  const prStateLabel = (state?: string) => {
    if (state === "OPEN") return { text: "open", cls: "text-green-600" };
    if (state === "MERGED") return { text: "merged", cls: "text-purple-600" };
    return { text: "closed", cls: "text-muted-foreground" };
  };

  const anyLoading = loadingPRRepos.size > 0 || loadingIssueRepos.size > 0;

  const getIssueDetail = (t: TreeItem): IssueDetail | undefined => {
    if (!t.issue) return undefined;
    const num = parseInt(t.issue.replace("#", ""), 10);
    return issueData[t.repo]?.find((d) => d.number === num);
  };

  const filteredTrees = filterTrees(trees, filterText, showMain, repoFilter, issueData);

  // ピン留めを最優先で先頭に固定し、その中／外それぞれを作成日の新しい順で並べる。
  // created_at は "YYYY-MM-DD" なので辞書順比較で日付順になる。
  // created_at が空（main worktree 等）は末尾に回す。
  const sortedTrees = [...filteredTrees].sort((a, b) => {
    const ap = pinnedPaths.has(a.path) ? 0 : 1;
    const bp = pinnedPaths.has(b.path) ? 0 : 1;
    if (ap !== bp) return ap - bp;
    if (!a.created_at && !b.created_at) return 0;
    if (!a.created_at) return 1;
    if (!b.created_at) return -1;
    return b.created_at.localeCompare(a.created_at);
  });

  // 全行を選択可能にする（main/master も最新化・ピン留め対象にできる）。
  // ただし削除だけは main を対象外にするため deletableSelected で別途絞る。
  const selectableTrees = filteredTrees;
  const selectedInView = selectableTrees.filter((t) => selectedPaths.has(t.path));
  const deletableSelected = selectedInView.filter((t) => !t.is_main);
  const allSelected =
    selectableTrees.length > 0 && selectedInView.length === selectableTrees.length;
  const someSelected = selectedInView.length > 0;

  useEffect(() => {
    const indeterminate = someSelected && !allSelected;
    if (headerCheckboxRef.current) {
      headerCheckboxRef.current.indeterminate = indeterminate;
    }
    if (mobileSelectAllRef.current) {
      mobileSelectAllRef.current.indeterminate = indeterminate;
    }
  }, [allSelected, someSelected]);

  const toggleAll = () => {
    if (allSelected) {
      setSelectedPaths(new Set());
    } else {
      setSelectedPaths(new Set(selectableTrees.map((t) => t.path)));
    }
  };

  const togglePath = (path: string) => {
    setSelectedPaths((prev) => {
      const next = new Set(prev);
      if (next.has(path)) {
        next.delete(path);
      } else {
        next.add(path);
      }
      return next;
    });
  };

  const handleBulkExecute = () => {
    if (!someSelected) return;
    // ピン留め/解除は破壊的でないので確認なしで即適用する。
    if (bulkAction === "pin" || bulkAction === "unpin") {
      const pin = bulkAction === "pin";
      applyPin(selectedInView.map((t) => t.path), pin);
      toast.success(
        `${selectedInView.length} 件を${pin ? "ピン留め" : "ピン解除"}しました`
      );
      setSelectedPaths(new Set());
      return;
    }
    // 最新化（git pull --ff-only）も破壊的でないので確認なしで実行。変更ありは
    // backend がはじくため、ここで事前に除外してスキップ件数を通知する。
    if (bulkAction === "update") {
      handleBulkUpdate();
      return;
    }
    // 削除: main/master は対象外。除外後に対象が無ければ確認に進まない。
    if (deletableSelected.length === 0) {
      toast.error("削除できる worktree がありません（main/master は対象外）");
      return;
    }
    setBulkConfirmInput("");
    setBulkConfirming(true);
  };

  const handleBulkUpdate = async () => {
    const targets = selectedInView.filter((t) => t.diff_count === 0);
    const skipped = selectedInView.length - targets.length;
    if (targets.length === 0) {
      toast.error("最新化できる worktree がありません（変更ありはスキップ）");
      return;
    }
    setBulkUpdating(true);
    let ok = 0;
    const fails: string[] = [];
    for (const t of targets) {
      try {
        await treesApi.update(t.repo, t.wt_name);
        ok++;
      } catch (e: unknown) {
        fails.push(`${t.wt_name}: ${(e as Error).message}`);
      }
    }
    setBulkUpdating(false);
    setSelectedPaths(new Set());
    queryClient.invalidateQueries({ queryKey: ["ports"] });
    refetch();
    if (fails.length) {
      toast.error(`${fails.length} 件の最新化に失敗`, {
        description: fails.join("\n"),
      });
    }
    const parts = [`${ok} 件を最新化しました`];
    if (skipped) parts.push(`${skipped} 件スキップ（変更あり）`);
    toast.success(parts.join(" / "));
  };

  const handleBulkDelete = async () => {
    setBulkDeleting(true);
    const targets = deletableSelected;
    for (const t of targets) {
      try {
        await treesApi.delete({ repo: t.repo, branch: t.branch || t.wt_name, force: t.diff_count > 0 });
      } catch (e: unknown) {
        alert((e as Error).message);
      }
    }
    setBulkDeleting(false);
    setBulkConfirming(false);
    setBulkConfirmInput("");
    setSelectedPaths(new Set());
    refetch();
  };

  return (
    <div className="space-y-6">
      {/* Worktree 一覧（追加はヘッダの「追加」ボタン→モーダル） */}
      <Card>
        <CardHeader>
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
            <CardTitle>Worktree 一覧</CardTitle>
            <div className="flex flex-wrap items-center gap-2">
              <Button
                size="sm"
                onClick={() => {
                  setAddError("");
                  setFormOpen(true);
                }}
                title="Worktree を追加"
              >
                <Plus className="h-3 w-3 mr-1" />
                追加
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => refetch()}
                disabled={isLoading}
                title="Worktree 一覧を再スキャン（ローカルの git worktree を読み直す）"
              >
                <RefreshCw
                  className={`h-3 w-3 mr-1 ${isLoading ? "animate-spin" : ""}`}
                />
                再読込
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={handleRefresh}
                disabled={anyLoading}
                title="Issue / PR 状態を GitHub から再取得（Issue/PR 列を表示中のみ反映）"
              >
                <RefreshCw
                  className={`h-3 w-3 mr-1 ${anyLoading ? "animate-spin" : ""}`}
                />
                Issue/PR更新
              </Button>
            </div>
          </div>
          <div className="flex items-center gap-x-4 gap-y-2 mt-2 flex-wrap text-sm">
            <Input
              className="h-8 w-full sm:w-48"
              placeholder="フリーワードで絞り込み..."
              value={filterText}
              onChange={(e) => setFilterText(e.target.value)}
            />
            <select
              className="h-8 rounded-md border bg-background px-2 text-sm"
              value={repoFilter}
              onChange={(e) => setRepoFilter(e.target.value)}
              title="リポジトリで絞り込み"
            >
              <option value="">全 repo</option>
              {repos.map((r) => (
                <option key={r.name} value={r.name}>
                  {r.name}
                </option>
              ))}
            </select>
            <label className="flex items-center gap-1 text-muted-foreground cursor-pointer">
              <input
                type="checkbox"
                checked={showMain}
                onChange={(e) => setShowMain(e.target.checked)}
              />
              main/master
            </label>
            <span className="text-xs text-muted-foreground">列:</span>
            <label className="flex items-center gap-1 text-sm text-muted-foreground cursor-pointer">
              <input
                type="checkbox"
                checked={showCols.issue}
                onChange={() => toggleCol("issue")}
              />
              Issue
            </label>
            <label className="flex items-center gap-1 text-sm text-muted-foreground cursor-pointer">
              <input
                type="checkbox"
                checked={showCols.parentIssue}
                onChange={() => toggleCol("parentIssue")}
              />
              親 issue
            </label>
            <label className="flex items-center gap-1 text-sm text-muted-foreground cursor-pointer">
              <input
                type="checkbox"
                checked={showCols.pr}
                onChange={() => toggleCol("pr")}
              />
              PR
            </label>
          </div>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <p className="text-sm text-muted-foreground">読み込み中...</p>
          ) : filteredTrees.length === 0 ? (
            <p className="text-sm text-muted-foreground">Worktree がありません</p>
          ) : (
            <>
              {/* 一括アクションバーは常時表示。選択の有無で出し入れすると一覧が
                  上下にブレるため、未選択時は実行ボタンを無効化するだけにする。 */}
              <div className="flex flex-wrap items-center gap-2 mb-3 pb-3 border-b">
                {/* モバイルは全選択ヘッダ checkbox（テーブル側）が無いので、アクションバーに置く。 */}
                {isMobile && (
                  <label className="flex items-center gap-1 text-sm text-muted-foreground">
                    <input
                      ref={mobileSelectAllRef}
                      type="checkbox"
                      className="size-5"
                      checked={allSelected}
                      onChange={toggleAll}
                      disabled={selectableTrees.length === 0}
                      aria-label="全選択"
                    />
                    全選択
                  </label>
                )}
                <span className="text-sm text-muted-foreground">
                  {selectedInView.length} 件選択中
                </span>
                <select
                  className="border rounded px-2 py-1 text-sm"
                  value={bulkAction}
                  onChange={(e) => setBulkAction(e.target.value)}
                  aria-label="アクション選択"
                >
                  <option value="delete">削除</option>
                  <option value="update">最新化</option>
                  <option value="pin">ピン留め</option>
                  <option value="unpin">ピン解除</option>
                </select>
                <Button
                  variant={bulkAction === "delete" ? "destructive" : "default"}
                  size="sm"
                  onClick={handleBulkExecute}
                  disabled={!someSelected || bulkUpdating}
                >
                  {bulkUpdating ? "実行中..." : "実行"}
                </Button>
              </div>
              {isMobile ? (
                <div className="space-y-2">
                  {sortedTrees.map((t) => (
                    <WorktreeCard
                      key={t.path}
                      tree={t}
                      port={portMap.get(`${t.repo}/${t.wt_name}`)}
                      stats={statsMap.get(`${t.repo}/${t.wt_name}`)}
                      pinned={pinnedPaths.has(t.path)}
                      selected={selectedPaths.has(t.path)}
                      copied={copiedPath === t.path}
                      isNew={t.path === newlyAddedPath}
                      onToggleSelect={() => togglePath(t.path)}
                      onTogglePin={() => applyPin([t.path], !pinnedPaths.has(t.path))}
                      onCopy={() => handleCopyPath(t.path)}
                      onRepoClick={() => {
                        setRepoFilter(t.repo);
                        setShowMain(true);
                      }}
                      onOpenDetail={() => setDetailTree(t)}
                      onOpenStats={() => {
                        const s = statsMap.get(`${t.repo}/${t.wt_name}`);
                        if (s) setOverlayStats(s);
                      }}
                      registerRef={(el) => {
                        if (el) rowRefs.current.set(t.path, el);
                        else rowRefs.current.delete(t.path);
                      }}
                    />
                  ))}
                </div>
              ) : (
              <div className="overflow-x-auto">
                <Table className="table-fixed w-full whitespace-nowrap">
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-8">
                      <input
                        ref={headerCheckboxRef}
                        type="checkbox"
                        checked={allSelected}
                        onChange={toggleAll}
                        disabled={selectableTrees.length === 0}
                        aria-label="全選択"
                      />
                    </TableHead>
                    <TableHead className="w-36">Repo</TableHead>
                    <TableHead className="w-12">コピー</TableHead>
                    <TableHead className="w-56">フォルダ名 / Branch</TableHead>
                    {showCols.issue && <TableHead className="w-24">Issue</TableHead>}
                    {showCols.parentIssue && <TableHead className="w-20">親 issue</TableHead>}
                    {showCols.pr && <TableHead className="w-24">PR</TableHead>}
                    <TableHead className="w-14" title="同名の tmux セッションが存在するか">tmux</TableHead>
                    <TableHead className="w-14" title="git status の変更ファイル数（未追跡除く）">
                      変更
                    </TableHead>
                    <TableHead className="w-24">作成日</TableHead>
                    <TableHead className="w-40" title="稼働状況と起動/停止">ポート</TableHead>
                    <TableHead className="w-24" title="dev サービスのメモリ使用量">状態</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {sortedTrees.map((t) => {
                    const pr = getPR(t);
                    const issueURL = getIssueURL(t);
                    const issueDetail = getIssueDetail(t);
                    const isPRLoading = loadingPRRepos.has(t.repo);
                    const port = portMap.get(`${t.repo}/${t.wt_name}`);
                    return (
                      <TableRow
                        key={t.path}
                        ref={(el) => {
                          if (el) rowRefs.current.set(t.path, el);
                          else rowRefs.current.delete(t.path);
                        }}
                        onClick={(e) => {
                          // 行内のインタラクティブ要素クリックでは詳細を開かない
                          if (
                            (e.target as HTMLElement).closest(
                              "a,button,input,select,label"
                            )
                          ) {
                            return;
                          }
                          setDetailTree(t);
                        }}
                        className={[
                          "cursor-pointer",
                          t.is_main ? "opacity-60" : "",
                          t.path === newlyAddedPath ? "row-highlight" : "",
                          statsMap.get(`${t.repo}/${t.wt_name}`)?.level === "danger" ? "bg-red-500/10 hover:bg-red-500/15" : "",
                        ]
                          .filter(Boolean)
                          .join(" ")}
                      >
                        <TableCell onClick={(e) => e.stopPropagation()}>
                          <input
                            type="checkbox"
                            checked={selectedPaths.has(t.path)}
                            onChange={() => togglePath(t.path)}
                            aria-label={`${t.repo}/${t.wt_name} を選択`}
                          />
                        </TableCell>
                        <TableCell className="text-xs" onClick={(e) => e.stopPropagation()}>
                          <button
                            className="inline-block max-w-full truncate text-left text-blue-600 hover:underline align-bottom"
                            title={t.repo}
                            onClick={() => {
                              setRepoFilter(t.repo);
                              setShowMain(true);
                            }}
                          >
                            {t.repo}
                          </button>
                        </TableCell>
                        <TableCell onClick={(e) => e.stopPropagation()}>
                          <div className="flex items-center gap-0.5">
                            <Button
                              variant="ghost"
                              size="sm"
                              className="h-6 w-6 p-0"
                              onClick={() => handleCopyPath(t.path)}
                              title={
                                copiedPath === t.path
                                  ? "コピーしました"
                                  : `${t.wt_name}\n${t.path}`
                              }
                              aria-label={`${t.wt_name} のパスをコピー`}
                            >
                              {copiedPath === t.path ? (
                                <Check className="h-3.5 w-3.5 text-green-600" />
                              ) : (
                                <Copy className="h-3.5 w-3.5" />
                              )}
                            </Button>
                          </div>
                        </TableCell>
                        <TableCell
                          className="text-xs"
                          title={`${t.wt_name}\n${t.branch || "（ブランチなし）"}`}
                        >
                          <div className="flex items-center gap-1">
                            {(() => {
                                const pinned = pinnedPaths.has(t.path);
                                return (
                                  <button
                                    type="button"
                                    onClick={() => applyPin([t.path], !pinned)}
                                    title={pinned ? "ピンを解除" : "ピン留め"}
                                    aria-label={
                                      pinned
                                        ? `${t.repo}/${t.wt_name} のピンを解除`
                                        : `${t.repo}/${t.wt_name} をピン留め`
                                    }
                                    className={`shrink-0 ${
                                      pinned
                                        ? "text-amber-500 hover:text-amber-600"
                                        : "text-muted-foreground/40 hover:text-amber-500"
                                    }`}
                                  >
                                    <Pin
                                      className={`h-3 w-3 ${pinned ? "fill-current" : ""}`}
                                    />
                                  </button>
                                );
                              })()}
                            <span className="truncate">{t.wt_name}</span>
                          </div>
                          <div className="truncate font-mono text-muted-foreground">
                            {t.branch || "—"}
                          </div>
                        </TableCell>
                        {showCols.issue && (
                        <TableCell className="text-xs">
                          {issueURL ? (
                            <div className="flex items-center gap-1">
                              <a
                                href={issueURL}
                                target="_blank"
                                rel="noopener noreferrer"
                                className="text-blue-600 hover:underline"
                              >
                                {t.issue}
                              </a>
                              {issueDetail ? (
                                <span
                                  className={
                                    issueDetail.state === "OPEN"
                                      ? "text-green-600"
                                      : "text-muted-foreground"
                                  }
                                >
                                  {issueDetail.state === "OPEN" ? "open" : "closed"}
                                </span>
                              ) : loadingIssueRepos.has(t.repo) ? (
                                <span className="text-muted-foreground">…</span>
                              ) : null}
                            </div>
                          ) : (
                            <span className="text-muted-foreground">—</span>
                          )}
                        </TableCell>
                        )}
                        {showCols.parentIssue && (
                        <TableCell className="text-xs">
                          {issueDetail?.parent_number ? (
                            <a
                              href={issueDetail.parent_url || "#"}
                              target="_blank"
                              rel="noopener noreferrer"
                              className="text-blue-600 hover:underline"
                            >
                              #{issueDetail.parent_number}
                            </a>
                          ) : loadingIssueRepos.has(t.repo) && t.issue ? (
                            <span className="text-muted-foreground">…</span>
                          ) : (
                            <span className="text-muted-foreground">—</span>
                          )}
                        </TableCell>
                        )}
                        {showCols.pr && (
                        <TableCell className="text-xs">
                          {pr ? (
                            (() => {
                              const { text, cls } = prStateLabel(pr.state);
                              const base = repoURLMap[t.repo];
                              return (
                                <a
                                  href={base ? `${base}/pull/${pr.number}` : "#"}
                                  target="_blank"
                                  rel="noopener noreferrer"
                                  className={`hover:underline ${cls}`}
                                >
                                  #{pr.number}
                                  <span className="text-muted-foreground ml-1">
                                    ({text})
                                  </span>
                                </a>
                              );
                            })()
                          ) : isPRLoading ? (
                            <span className="text-muted-foreground">…</span>
                          ) : (
                            <span className="text-muted-foreground">—</span>
                          )}
                        </TableCell>
                        )}
                        <TableCell className="text-xs">
                          {t.has_tmux ? (
                            <span className="text-green-600">✓</span>
                          ) : (
                            <span className="text-muted-foreground">—</span>
                          )}
                        </TableCell>
                        <TableCell className="text-xs">
                          {t.diff_count > 0 ? (
                            <span className="text-amber-600 font-medium">
                              {t.diff_count}
                            </span>
                          ) : (
                            <span className="text-muted-foreground">0</span>
                          )}
                        </TableCell>
                        <TableCell className="text-xs text-muted-foreground">
                          {t.created_at || "—"}
                        </TableCell>
                        <TableCell className="text-xs">
                          {!port?.has_dev_config ? (
                            <span className="text-muted-foreground">—</span>
                          ) : port.running && port.degraded ? (
                            <span
                              className="inline-flex items-center gap-1 text-amber-600"
                              title="一部のサービスが正常に稼働していません（停止または未LISTEN）"
                            >
                              <span className="h-2 w-2 rounded-full bg-amber-500" />
                              ⚠ 縮退
                            </span>
                          ) : (
                            <span
                              className={
                                port.running
                                  ? "inline-flex items-center gap-1 text-green-700"
                                  : "inline-flex items-center gap-1 text-muted-foreground"
                              }
                              title={port.port_range ?? "未割当"}
                            >
                              <span
                                className={`h-2 w-2 rounded-full ${port.running ? "bg-green-600" : "bg-muted-foreground/40"}`}
                              />
                              {port.running ? "稼働" : "停止"}
                            </span>
                          )}
                        </TableCell>
                        <TableCell className="text-xs">
                          {(() => {
                            const stats = statsMap.get(`${t.repo}/${t.wt_name}`);
                            if (!stats) return <span className="text-muted-foreground">—</span>;
                            
                            const colorClass = 
                              stats.level === "danger" ? "text-red-600 font-medium" :
                              stats.level === "warn" ? "text-amber-600" :
                              "text-muted-foreground";

                            return (
                              <button
                                className={`hover:underline ${colorClass}`}
                                onClick={(e) => {
                                  e.stopPropagation();
                                  setOverlayStats(stats);
                                }}
                              >
                                {formatBytes(stats.total_rss_bytes)}
                              </button>
                            );
                          })()}
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </div>
            )}
            </>
          )}
        </CardContent>
      </Card>

      {/* Worktree 追加モーダル */}
      {formOpen && (
        <div
          className="fixed inset-0 bg-black/40 flex items-center justify-center z-50"
          onClick={() => setFormOpen(false)}
        >
          <Card
            className="w-[36rem] max-w-[92vw]"
            onClick={(e) => e.stopPropagation()}
          >
            <CardHeader>
              <div className="flex items-center justify-between">
                <CardTitle>Worktree を追加</CardTitle>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => setFormOpen(false)}
                  aria-label="閉じる"
                >
                  <X className="h-4 w-4" />
                </Button>
              </div>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="flex gap-2">
                <Button
                  variant={issueMode ? "default" : "outline"}
                  size="sm"
                  onClick={() => setIssueMode(true)}
                >
                  Issue URL モード
                </Button>
                <Button
                  variant={!issueMode ? "default" : "outline"}
                  size="sm"
                  onClick={() => setIssueMode(false)}
                >
                  Branch モード
                </Button>
              </div>

              {issueMode ? (
                <Input
                  placeholder="https://github.com/owner/repo/issues/123"
                  value={form.issue_url ?? ""}
                  onChange={(e) => setForm({ ...form, issue_url: e.target.value })}
                  autoFocus
                />
              ) : (
                <div className="flex gap-2">
                  <select
                    className="border rounded px-2 py-1 text-sm"
                    value={form.repo ?? ""}
                    onChange={(e) => setForm({ ...form, repo: e.target.value })}
                  >
                    <option value="">リポジトリを選択</option>
                    {repos.map((r) => (
                      <option key={r.name} value={r.name}>
                        {r.name}
                      </option>
                    ))}
                  </select>
                  <Input
                    placeholder="branch 名 (e.g. issue155)"
                    value={form.branch ?? ""}
                    onChange={(e) => setForm({ ...form, branch: e.target.value })}
                  />
                  <select
                    className="border rounded px-2 py-1 text-sm"
                    value={form.type ?? "feature"}
                    onChange={(e) => setForm({ ...form, type: e.target.value })}
                  >
                    {["feature", "fix", "chore", "docs", "refactor", "test", "ci"].map(
                      (t) => (
                        <option key={t} value={t}>
                          {t}
                        </option>
                      )
                    )}
                  </select>
                </div>
              )}

              {addError && (
                <Alert variant="destructive">
                  <AlertDescription>{addError}</AlertDescription>
                </Alert>
              )}
              <div className="flex justify-end gap-2">
                <Button variant="outline" onClick={() => setFormOpen(false)}>
                  キャンセル
                </Button>
                <Button onClick={handleAdd} disabled={addMutation.isPending}>
                  {addMutation.isPending ? "作成中..." : "作成"}
                </Button>
              </div>
            </CardContent>
          </Card>
        </div>
      )}

      {/* バルク削除確認モーダル */}
      {bulkConfirming && (() => {
        const hasDirty = deletableSelected.some((t) => t.diff_count > 0);
        const excludedMain = selectedInView.length - deletableSelected.length;
        return (
          <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50 p-4">
            <Card className="w-[32rem] max-w-[92vw]">
              <CardHeader>
                <CardTitle>削除確認</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <p className="text-sm">以下の Worktree を削除しますか？</p>
                <ul className="text-sm font-mono space-y-1 max-h-48 overflow-y-auto border rounded p-2">
                  {deletableSelected.map((t) => (
                    <li key={t.path}>
                      {t.repo} / {t.wt_name}
                      {t.diff_count > 0 && (
                        <span className="text-amber-600 ml-1">
                          （変更 {t.diff_count} 件）
                        </span>
                      )}
                    </li>
                  ))}
                </ul>
                {excludedMain > 0 && (
                  <p className="text-xs text-muted-foreground">
                    main/master {excludedMain} 件は削除対象外のため除外しました。
                  </p>
                )}
                {hasDirty ? (
                  <>
                    <p className="text-sm text-amber-600">
                      未コミット変更のある worktree が含まれています。削除すると変更が失われます。
                    </p>
                    <p className="text-sm">
                      続行するには <span className="font-mono font-semibold">yes</span> と入力してください。
                    </p>
                    <input
                      type="text"
                      value={bulkConfirmInput}
                      onChange={(e) => setBulkConfirmInput(e.target.value)}
                      placeholder="yes"
                      aria-label="削除確認のため yes と入力"
                      autoFocus
                      className="w-full rounded border px-2 py-1 font-mono text-sm"
                    />
                  </>
                ) : (
                  <p className="text-sm">この操作は取り消せません。</p>
                )}
                <div className="flex gap-2 justify-end">
                  <Button
                    variant="outline"
                    onClick={() => {
                      setBulkConfirming(false);
                      setBulkConfirmInput("");
                    }}
                    disabled={bulkDeleting}
                  >
                    キャンセル
                  </Button>
                  <Button
                    variant="destructive"
                    onClick={handleBulkDelete}
                    disabled={
                      bulkDeleting ||
                      (hasDirty && bulkConfirmInput !== "yes")
                    }
                  >
                    {bulkDeleting
                      ? "削除中..."
                      : hasDirty
                        ? "強制削除"
                        : "削除"}
                  </Button>
                </div>
              </CardContent>
            </Card>
          </div>
        );
      })()}

      {overlayStats && statsData && (
        <ProcessStatsOverlay
          stats={overlayStats}
          warnBytes={statsData.warn_bytes}
          dangerBytes={statsData.danger_bytes}
          onClose={() => setOverlayStats(null)}
        />
      )}

      <DevConfigPanel
        target={devConfigTarget}
        onClose={() => setDevConfigTarget(null)}
      />
      <LogPanel target={logTarget} onClose={() => setLogTarget(null)} />
      <WorktreeDetailPanel
        key={detailTree?.path ?? "none"}
        detail={
          detailTree
            ? {
                tree: detailTree,
                port: portMap.get(`${detailTree.repo}/${detailTree.wt_name}`),
                issueURL: getIssueURL(detailTree),
                issueDetail: getIssueDetail(detailTree),
                pr: getPR(detailTree),
                repoURL: repoURLMap[detailTree.repo],
              }
            : null
        }
        portBusy={portBusy}
        onServe={() =>
          detailTree &&
          serveMutation.mutate({ repo: detailTree.repo, wt: detailTree.wt_name })
        }
        onDown={() =>
          detailTree &&
          downMutation.mutate({ repo: detailTree.repo, wt: detailTree.wt_name })
        }
        onEditConfig={() =>
          detailTree &&
          setDevConfigTarget({ repo: detailTree.repo, wt: detailTree.wt_name })
        }
        onShowLogs={() =>
          detailTree && setLogTarget({ repo: detailTree.repo, wt: detailTree.wt_name })
        }
        onUpdate={() =>
          detailTree &&
          updateMutation.mutate({ repo: detailTree.repo, wt: detailTree.wt_name })
        }
        updating={updateMutation.isPending}
        onDelete={(force) =>
          detailTree &&
          deleteMutation.mutate({
            repo: detailTree.repo,
            branch: detailTree.branch || detailTree.wt_name,
            force,
          })
        }
        deleting={deleteMutation.isPending}
        onClose={() => setDetailTree(null)}
      />
    </div>
  );
}
