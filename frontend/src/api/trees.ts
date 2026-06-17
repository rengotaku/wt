import { apiClient } from "./client";

export interface TreeItem {
  wt_name: string;
  repo: string;
  label: string;
  path: string;
  created_at: string;
  diff_count: number;
  has_tmux: boolean;
  is_main: boolean;
  branch: string;
  issue?: string;
}

export interface AddTreeRequest {
  repo?: string;
  branch?: string;
  type?: string;
  dir?: string;
  issue_url?: string;
}

export interface AddTreeResponse {
  path: string;
  output: string;
}

export interface DeleteTreeRequest {
  repo: string;
  branch: string;
  /** 未コミット変更がある worktree を強制削除する（AWS 風の入力確認を経て指定） */
  force?: boolean;
}

export interface GcRequest {
  merged?: boolean;
  closed?: boolean;
  include_dirty?: boolean;
  older_than?: string;
  no_tmux?: boolean;
  dry_run?: boolean;
  yes?: boolean;
}

export interface GcResponse {
  output: string;
}

export interface MergedPRInfo {
  number: number;
  head_ref_name: string;
  merged_at: string;
  state?: string;
}

export interface IssueDetail {
  number: number;
  state: string;
  parent_number?: number;
  parent_url?: string;
}

export const treesApi = {
  list: (): Promise<TreeItem[]> => apiClient.get("api/trees").json(),

  add: (body: AddTreeRequest): Promise<AddTreeResponse> =>
    apiClient.post("api/trees", { json: body }).json(),

  delete: (body: DeleteTreeRequest): Promise<{ output: string }> =>
    apiClient.delete("api/trees", { json: body }).json(),

  gc: (body: GcRequest): Promise<GcResponse> =>
    apiClient.post("api/trees/gc", { json: body }).json(),

  mergedPRs: (repo: string): Promise<MergedPRInfo[]> =>
    apiClient.get("api/trees/merged-prs", { searchParams: { repo } }).json(),

  issueDetails: (repo: string): Promise<IssueDetail[]> =>
    apiClient.get("api/trees/issue-details", { searchParams: { repo } }).json(),
};
