import { useState, useEffect } from "react";
import { useQuery, useMutation } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { RefreshCw, Copy, Check, ChevronDown, ChevronRight } from "lucide-react";
import { treesApi, reposApi, type AddTreeRequest, type TreeItem, type MergedPRInfo, type IssueDetail } from "@/api";
import { filterTrees } from "./treesFilter";
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

export function TreesPage() {
  const [searchParams] = useSearchParams();

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

  const [formOpen, setFormOpen] = useState(false);
  const [issueMode, setIssueMode] = useState(true);
  const [form, setForm] = useState<AddTreeRequest>({ repo: "", branch: "", type: "feature" });
  const [addError, setAddError] = useState("");

  const [filterText, setFilterText] = useState("");
  const initialRepoFilter = searchParams.get("repo") ?? "";
  const [showMain, setShowMain] = useState(initialRepoFilter !== "");
  const [repoFilter, setRepoFilter] = useState(initialRepoFilter);

  const [deleteTarget, setDeleteTarget] = useState<TreeItem | null>(null);
  const [copiedPath, setCopiedPath] = useState<string | null>(null);
  const [copyTemplate, setCopyTemplate] = useState<string>(
    () => localStorage.getItem("wt-copy-template") ?? "$path"
  );

  // キャッシュから初期化 → マウント後に未取得分を順次フェッチ
  const [prData, setPrData] = useState<Record<string, MergedPRInfo[]>>(
    () => Object.fromEntries(prCache.entries())
  );
  const [loadingPRRepos, setLoadingPRRepos] = useState<Set<string>>(new Set());

  const [issueData, setIssueData] = useState<Record<string, IssueDetail[]>>(
    () => Object.fromEntries(issueCache.entries())
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
          setLoadingPRRepos((prev) => { const n = new Set(prev); n.delete(repo); return n; });
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
          setLoadingIssueRepos((prev) => { const n = new Set(prev); n.delete(repo); return n; });
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

  const addMutation = useMutation({
    mutationFn: treesApi.add,
    onSuccess: () => {
      refetch();
      setForm({ repo: "", branch: "", type: "feature" });
      setAddError("");
    },
    onError: (e: Error) => setAddError(e.message),
  });

  const deleteMutation = useMutation({
    mutationFn: treesApi.delete,
    onSuccess: () => {
      refetch();
      setDeleteTarget(null);
    },
    onError: (e: Error) => alert(e.message),
  });

  const handleAdd = () => {
    if (issueMode) {
      addMutation.mutate({ issue_url: form.issue_url });
    } else {
      addMutation.mutate({ repo: form.repo, branch: form.branch, type: form.type });
    }
  };

  const handleCopyTemplate = (value: string) => {
    setCopyTemplate(value);
    localStorage.setItem("wt-copy-template", value);
  };

  const handleCopyPath = async (path: string) => {
    const text = (copyTemplate || "$path").replace(/\$path/g, path);
    await navigator.clipboard.writeText(text);
    setCopiedPath(path);
    setTimeout(() => setCopiedPath(null), 2000);
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

  return (
    <div className="space-y-6">
      {/* 追加フォーム（折りたたみ） */}
      <Card>
        <CardHeader
          className="cursor-pointer select-none"
          onClick={() => setFormOpen(!formOpen)}
        >
          <CardTitle className="flex items-center gap-2">
            {formOpen ? (
              <ChevronDown className="h-4 w-4" />
            ) : (
              <ChevronRight className="h-4 w-4" />
            )}
            Worktree を追加
          </CardTitle>
        </CardHeader>
        {formOpen && (
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
                  {["feature", "fix", "chore", "docs", "refactor", "test", "ci"].map((t) => (
                    <option key={t} value={t}>
                      {t}
                    </option>
                  ))}
                </select>
              </div>
            )}

            {addError && (
              <Alert variant="destructive">
                <AlertDescription>{addError}</AlertDescription>
              </Alert>
            )}
            <Button onClick={handleAdd} disabled={addMutation.isPending}>
              {addMutation.isPending ? "作成中..." : "作成"}
            </Button>
          </CardContent>
        )}
      </Card>

      {/* Worktree 一覧 */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle>Worktree 一覧</CardTitle>
            <div className="flex items-center gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={handleRefresh}
                disabled={anyLoading}
                title="Issue / PR 状態をリフレッシュ"
              >
                <RefreshCw className={`h-3 w-3 mr-1 ${anyLoading ? "animate-spin" : ""}`} />
                更新
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => refetch()}
                disabled={isLoading}
                title="Worktree 一覧をリロード"
              >
                <RefreshCw className="h-4 w-4" />
              </Button>
            </div>
          </div>
          <div className="flex items-center gap-3 mt-2 flex-wrap">
            <Input
              className="max-w-xs"
              placeholder="名前で絞り込み..."
              value={filterText}
              onChange={(e) => setFilterText(e.target.value)}
            />
            <div className="flex items-center gap-1">
              <span className="text-xs text-muted-foreground whitespace-nowrap">コピー形式:</span>
              <Input
                className="w-56 font-mono text-xs h-7"
                placeholder="$path"
                value={copyTemplate}
                onChange={(e) => handleCopyTemplate(e.target.value)}
                title="$path がパスに置換されます（例: cd $path && tmc）"
              />
            </div>
            {repoFilter && (
              <div className="flex items-center gap-1 text-sm">
                <span className="text-muted-foreground">リポ:</span>
                <span className="font-medium">{repoFilter}</span>
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-5 px-1 text-xs"
                  onClick={() => setRepoFilter("")}
                >
                  ✕
                </Button>
              </div>
            )}
            <label className="flex items-center gap-1 text-sm text-muted-foreground cursor-pointer">
              <input
                type="checkbox"
                checked={showMain}
                onChange={(e) => setShowMain(e.target.checked)}
              />
              main/master を表示
            </label>
          </div>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <p className="text-sm text-muted-foreground">読み込み中...</p>
          ) : filteredTrees.length === 0 ? (
            <p className="text-sm text-muted-foreground">Worktree がありません</p>
          ) : (
            <div className="overflow-x-auto">
              <Table className="whitespace-nowrap">
                <TableHeader>
                  <TableRow>
                    <TableHead>Repo</TableHead>
                    <TableHead>フォルダ名</TableHead>
                    <TableHead>Branch</TableHead>
                    <TableHead>Issue</TableHead>
                    <TableHead>親 issue</TableHead>
                    <TableHead>PR</TableHead>
                    <TableHead title="同名の tmux セッションが存在するか">tmux</TableHead>
                    <TableHead title="git status の変更ファイル数（未追跡除く）">変更</TableHead>
                    <TableHead>作成日</TableHead>
                    <TableHead></TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filteredTrees.map((t) => {
                    const pr = getPR(t);
                    const issueURL = getIssueURL(t);
                    const issueDetail = getIssueDetail(t);
                    const isPRLoading = loadingPRRepos.has(t.repo);
                    return (
                      <TableRow key={t.path} className={t.is_main ? "opacity-60" : ""}>
                        <TableCell className="text-xs">
                          <button
                            className="text-blue-600 hover:underline"
                            onClick={() => { setRepoFilter(t.repo); setShowMain(true); }}
                          >
                            {t.repo}
                          </button>
                        </TableCell>
                        <TableCell className="font-mono text-xs">
                          <div className="flex items-center gap-1">
                            {t.wt_name}
                            <Button
                              variant="ghost"
                              size="sm"
                              className="h-5 w-5 p-0"
                              onClick={() => handleCopyPath(t.path)}
                              title={t.path}
                            >
                              {copiedPath === t.path ? (
                                <Check className="h-3 w-3 text-green-600" />
                              ) : (
                                <Copy className="h-3 w-3" />
                              )}
                            </Button>
                          </div>
                        </TableCell>
                        <TableCell className="font-mono text-xs text-muted-foreground">
                          {t.branch || "—"}
                        </TableCell>
                        <TableCell className="text-xs">
                          {issueURL ? (
                            <div className="flex items-center gap-1">
                              <a href={issueURL} target="_blank" rel="noopener noreferrer" className="text-blue-600 hover:underline">
                                {t.issue}
                              </a>
                              {issueDetail ? (
                                <span className={issueDetail.state === "OPEN" ? "text-green-600" : "text-muted-foreground"}>
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
                                  <span className="text-muted-foreground ml-1">({text})</span>
                                </a>
                              );
                            })()
                          ) : isPRLoading ? (
                            <span className="text-muted-foreground">…</span>
                          ) : (
                            <span className="text-muted-foreground">—</span>
                          )}
                        </TableCell>
                        <TableCell className="text-xs">
                          {t.has_tmux ? (
                            <span className="text-green-600">✓</span>
                          ) : (
                            <span className="text-muted-foreground">—</span>
                          )}
                        </TableCell>
                        <TableCell className="text-xs">
                          {t.diff_count > 0 ? (
                            <span className="text-amber-600 font-medium">{t.diff_count}</span>
                          ) : (
                            <span className="text-muted-foreground">0</span>
                          )}
                        </TableCell>
                        <TableCell className="text-xs text-muted-foreground">
                          {t.created_at || "—"}
                        </TableCell>
                        <TableCell>
                          {!t.is_main && (
                            <Button
                              variant="destructive"
                              size="sm"
                              onClick={() => setDeleteTarget(t)}
                            >
                              削除
                            </Button>
                          )}
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>

      {/* 削除確認モーダル */}
      {deleteTarget && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
          <Card className="w-96">
            <CardHeader>
              <CardTitle>削除確認</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <p className="text-sm">
                <span className="font-mono">
                  {deleteTarget.repo} / {deleteTarget.wt_name}
                </span>{" "}
                を削除しますか？
              </p>
              <div className="flex gap-2 justify-end">
                <Button variant="outline" onClick={() => setDeleteTarget(null)}>
                  キャンセル
                </Button>
                <Button
                  variant="destructive"
                  disabled={deleteMutation.isPending}
                  onClick={() =>
                    deleteMutation.mutate({
                      repo: deleteTarget.repo,
                      branch: deleteTarget.branch || deleteTarget.wt_name,
                    })
                  }
                >
                  {deleteMutation.isPending ? "削除中..." : "削除"}
                </Button>
              </div>
            </CardContent>
          </Card>
        </div>
      )}
    </div>
  );
}
