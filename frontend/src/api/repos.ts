import { apiClient } from "./client";

export interface Repo {
  name: string;
  container: string;
  count: number;
  github_url?: string;
  description?: string;
  main_branch?: string;
  main_dirty: boolean;
  main_ahead: number;
  main_behind: number;
}

export interface RepoConfig {
  symlink_candidates: string[];
}

export const reposApi = {
  list: (): Promise<Repo[]> => apiClient.get("api/repos").json(),

  add: (url: string): Promise<{ output: string }> =>
    apiClient.post("api/repos", { json: { url } }).json(),

  delete: (name: string): Promise<{ output: string }> =>
    apiClient.delete("api/repos", { json: { name } }).json(),

  refresh: (name: string): Promise<{ output: string }> =>
    apiClient.post("api/repos/refresh", { json: { name } }).json(),

  sync: (name: string): Promise<{ output: string }> =>
    apiClient.post("api/repos/sync", { json: { name } }).json(),

  syncAll: (): Promise<{ message: string }> =>
    apiClient.post("api/repos/sync-all").json(),

  getConfig: (name: string): Promise<RepoConfig> =>
    apiClient.get(`api/repos/${encodeURIComponent(name)}/config`).json(),

  updateConfig: (name: string, cfg: RepoConfig): Promise<RepoConfig> =>
    apiClient.put(`api/repos/${encodeURIComponent(name)}/config`, { json: cfg }).json(),
};
