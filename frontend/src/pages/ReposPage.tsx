import { useState } from "react";
import { useQuery, useMutation } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import {
  AlertTriangle,
  ChevronDown,
  ChevronRight,
  ExternalLink,
  RefreshCw,
  Settings,
  Eye,
  EyeOff,
} from "lucide-react";
import { reposApi, type Repo } from "@/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { RepoConfigPanel } from "@/components/RepoConfigPanel";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

export function ReposPage() {
  const {
    data: repos = [],
    isLoading,
    refetch,
  } = useQuery({
    queryKey: ["repos"],
    queryFn: reposApi.list,
    staleTime: Infinity,
    refetchOnWindowFocus: false,
  });

  const [addFormOpen, setAddFormOpen] = useState(false);
  const [addURL, setAddURL] = useState("");
  const [addError, setAddError] = useState("");
  const [addOutput, setAddOutput] = useState("");

  const [deleteTarget, setDeleteTarget] = useState<Repo | null>(null);

  const [syncMessage, setSyncMessage] = useState("");
  const [syncing, setSyncing] = useState(false);

  const [refreshingRepo, setRefreshingRepo] = useState<string | null>(null);
  const [syncingRepo, setSyncingRepo] = useState<string | null>(null);
  const [syncRepoMsg, setSyncRepoMsg] = useState<
    Record<string, { ok: boolean; msg: string }>
  >({});

  const [configRepo, setConfigRepo] = useState<string | null>(null);

  const addMutation = useMutation({
    mutationFn: (url: string) => reposApi.add(url),
    onSuccess: (res) => {
      refetch();
      setAddURL("");
      setAddError("");
      setAddOutput(res.output);
    },
    onError: (e: Error) => {
      setAddError(e.message);
      setAddOutput("");
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (name: string) => reposApi.delete(name),
    onSuccess: () => {
      refetch();
      setDeleteTarget(null);
    },
    onError: (e: Error) => alert(e.message),
  });

  const hiddenMutation = useMutation({
    mutationFn: ({ name, hidden }: { name: string; hidden: boolean }) =>
      reposApi.setHidden(name, hidden),
    onSuccess: () => {
      refetch();
    },
    onError: (e: Error) => alert(e.message),
  });

  const handleRefresh = async (name: string) => {
    setRefreshingRepo(name);
    try {
      await reposApi.refresh(name);
      refetch();
    } catch (e) {
      alert((e as Error).message);
    } finally {
      setRefreshingRepo(null);
    }
  };

  const handleSyncRepo = async (name: string) => {
    setSyncingRepo(name);
    try {
      const res = await reposApi.sync(name);
      const finalMsg = res.restarted ? `${res.output}（dev 再起動）` : res.output;
      setSyncRepoMsg((prev) => ({ ...prev, [name]: { ok: true, msg: finalMsg } }));
      refetch();
    } catch (e) {
      setSyncRepoMsg((prev) => ({
        ...prev,
        [name]: { ok: false, msg: (e as Error).message },
      }));
    } finally {
      setSyncingRepo(null);
    }
  };

  const handleSyncAll = async () => {
    setSyncing(true);
    setSyncMessage("");
    try {
      await reposApi.syncAll();
      setSyncMessage(
        "同期を開始しました。しばらくしてからリロードして状態を確認してください。"
      );
    } catch (e) {
      setSyncMessage(`エラー: ${(e as Error).message}`);
    } finally {
      setSyncing(false);
    }
  };

  return (
    <div className="space-y-6">
      {/* リポジトリ追加 */}
      <Card>
        <CardHeader
          className="cursor-pointer select-none"
          onClick={() => setAddFormOpen(!addFormOpen)}
        >
          <CardTitle className="flex items-center gap-2">
            {addFormOpen ? (
              <ChevronDown className="h-4 w-4" />
            ) : (
              <ChevronRight className="h-4 w-4" />
            )}
            リポジトリを追加
          </CardTitle>
        </CardHeader>
        {addFormOpen && (
          <CardContent className="space-y-3">
            <div className="flex gap-2">
              <Input
                placeholder="https://github.com/owner/repo"
                value={addURL}
                onChange={(e) => setAddURL(e.target.value)}
                className="flex-1"
              />
              <Button
                onClick={() => addMutation.mutate(addURL)}
                disabled={addMutation.isPending || !addURL}
              >
                {addMutation.isPending ? "追加中..." : "追加"}
              </Button>
            </div>
            {addError && (
              <Alert variant="destructive">
                <AlertDescription>{addError}</AlertDescription>
              </Alert>
            )}
            {addOutput && (
              <pre className="text-xs font-mono bg-muted p-2 rounded whitespace-pre-wrap">
                {addOutput}
              </pre>
            )}
          </CardContent>
        )}
      </Card>

      {/* リポジトリ一覧 */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle>リポジトリ一覧</CardTitle>
            <div className="flex gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={handleSyncAll}
                disabled={syncing}
              >
                {syncing ? "同期中..." : "全リポ main を最新化"}
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => refetch()}
                disabled={isLoading}
              >
                <RefreshCw className="h-4 w-4" />
              </Button>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <p className="text-sm text-muted-foreground">読み込み中...</p>
          ) : repos.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              登録リポジトリがありません (wt repo add で追加してください)
            </p>
          ) : (
            <Table wrapperClassName="max-h-[calc(100vh-250px)]">
              <TableHeader className="sticky top-0 z-10 bg-background shadow-[0_1px_2px_rgba(0,0,0,0.1)]">
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Base</TableHead>
                  <TableHead>Worktrees</TableHead>
                  <TableHead>状態</TableHead>
                  <TableHead>GitHub</TableHead>
                  <TableHead></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {repos.map((r) => (
                  <TableRow key={r.name} className={r.hidden ? "opacity-50" : ""}>
                    <TableCell className="font-medium">
                      <span>{r.name}</span>
                      {r.description && (
                        <span className="ml-2 text-xs text-muted-foreground font-normal">
                          {r.description}
                        </span>
                      )}
                    </TableCell>
                    <TableCell className="text-xs font-mono text-muted-foreground">
                      {r.main_branch ?? "—"}
                    </TableCell>
                    <TableCell>
                      <Link
                        to={`/?repo=${r.name}`}
                        className="text-blue-600 hover:underline"
                      >
                        {r.count}
                      </Link>
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-1">
                        {r.main_dirty && (
                          <span
                            className="text-amber-500"
                            title="main に未コミットの差分があります"
                          >
                            <AlertTriangle className="h-4 w-4" />
                          </span>
                        )}
                        {r.main_behind > 0 && (
                          <span
                            className="text-red-500 text-xs font-medium"
                            title={`リモートより ${r.main_behind} コミット遅れています（リフレッシュで最新化）`}
                          >
                            ↓{r.main_behind}
                          </span>
                        )}
                        {r.main_ahead > 0 && (
                          <span
                            className="text-blue-500 text-xs font-medium"
                            title={`リモートより ${r.main_ahead} コミット先行しています`}
                          >
                            ↑{r.main_ahead}
                          </span>
                        )}
                        {!r.main_dirty && r.main_behind === 0 && r.main_ahead === 0 && (
                          <span className="text-muted-foreground text-xs">—</span>
                        )}
                      </div>
                    </TableCell>
                    <TableCell>
                      {r.github_url ? (
                        <a
                          href={r.github_url}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="inline-flex items-center gap-1 text-blue-600 hover:underline text-xs"
                        >
                          <ExternalLink className="h-3 w-3" />
                          GitHub
                        </a>
                      ) : (
                        <span className="text-muted-foreground text-xs">—</span>
                      )}
                    </TableCell>
                    <TableCell>
                      <div className="flex flex-col gap-1">
                        <div className="flex gap-1">
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => hiddenMutation.mutate({ name: r.name, hidden: !r.hidden })}
                            title={r.hidden ? "一覧に表示する" : "一覧から隠す"}
                            disabled={hiddenMutation.isPending}
                          >
                            {r.hidden ? (
                              <EyeOff className="h-3 w-3 text-muted-foreground" />
                            ) : (
                              <Eye className="h-3 w-3" />
                            )}
                          </Button>
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={refreshingRepo === r.name}
                            onClick={() => handleRefresh(r.name)}
                            title="git fetch を実行して最新状態を確認"
                          >
                            <RefreshCw
                              className={`h-3 w-3 ${refreshingRepo === r.name ? "animate-spin" : ""}`}
                            />
                          </Button>
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={syncingRepo === r.name}
                            onClick={() => handleSyncRepo(r.name)}
                            title="git fetch + pull --ff-only で main を最新化"
                          >
                            {syncingRepo === r.name ? "最新化中..." : "最新化"}
                          </Button>
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => setConfigRepo(r.name)}
                            title=".worktrees.json の設定を編集"
                          >
                            <Settings className="h-3 w-3" />
                          </Button>
                          <Button
                            variant="destructive"
                            size="sm"
                            onClick={() => setDeleteTarget(r)}
                          >
                            削除
                          </Button>
                        </div>
                        {syncRepoMsg[r.name] && (
                          <p
                            className={`text-xs ${syncRepoMsg[r.name].ok ? "text-green-600" : "text-red-600"}`}
                          >
                            {syncRepoMsg[r.name].msg ||
                              (syncRepoMsg[r.name].ok ? "Already up to date" : "error")}
                          </p>
                        )}
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      {syncMessage && <p className="text-sm text-muted-foreground">{syncMessage}</p>}

      {/* 設定サイドパネル */}
      <RepoConfigPanel repoName={configRepo} onClose={() => setConfigRepo(null)} />

      {/* 削除確認モーダル */}
      {deleteTarget && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50 p-4">
          <Card className="w-96 max-w-[92vw]">
            <CardHeader>
              <CardTitle>削除確認</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <p className="text-sm">
                <span className="font-mono font-medium">{deleteTarget.name}</span>{" "}
                を削除しますか？
              </p>
              <p className="text-xs text-muted-foreground">
                worktree が main のみの場合に限り削除できます。追加 worktree
                が残存している場合はエラーになります。
              </p>
              <div className="flex gap-2 justify-end">
                <Button variant="outline" onClick={() => setDeleteTarget(null)}>
                  キャンセル
                </Button>
                <Button
                  variant="destructive"
                  disabled={deleteMutation.isPending}
                  onClick={() => deleteMutation.mutate(deleteTarget.name)}
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
