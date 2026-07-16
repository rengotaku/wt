import { apiClient } from "./client";

export interface PortState {
  port: number;
  listening: boolean;
  running?: boolean; // 記録済みサービスの PID が生存（ポート未bindの headless worker でも true）
  headless?: boolean; // ポートを張らない宣言のサービス（worker/scheduler）
  unhealthy?: boolean; // LISTEN すべきなのに PID 生存のまま未 LISTEN（起動失敗の疑い）
  pid?: number;
  proc?: string;
  service?: string; // dev service名 (api/web/admin 等)
}

export interface PortItem {
  repo: string;
  wt_name: string;
  branch?: string;
  port_base: number; // 0 when unallocated
  port_range?: string; // "9000-9004"
  ports: PortState[];
  has_dev_config: boolean;
  running: boolean;
  degraded?: boolean; // running しているが記録済みサービスの一部が停止している（縮退）
  unmanaged?: boolean; // LISTEN しているが wt の起動記録が無い（外部起動）。wt からは停止できない
  domain?: string; // <label>.wt.localhost when a domain service exists
  domain_port?: number; // localhost port of the domain(=user-facing)サービス
}

export interface ServeResult {
  output: string;
  running: boolean;
}

export interface StaleItem {
  repo: string;
  wt_name: string;
  port_base: number;
  port_range?: string; // "9000-9004"
}

export interface PruneResult {
  removed: StaleItem[];
  count: number;
}

export interface ListenerRow {
  port: number;
  pid?: number;
  proc?: string;
  managed: boolean;
  owner?: string; // "repo/wtname" when managed
}

export interface DevService {
  name: string;
  cmd: string;
  domain: boolean;
  headless?: boolean; // ポートを張らない worker/scheduler（既定は LISTEN する想定）
}

export interface DevConfig {
  has_config: boolean;
  source: string; // "worktree" | "repo" | "file" | ""
  services: DevService[];
}

export interface ServiceLog {
  name: string;
  content: string;
}

const devConfigURL = (repo: string, wtName: string) =>
  `api/ports/${encodeURIComponent(repo)}/${encodeURIComponent(wtName)}/devconfig`;

const logsURL = (repo: string, wtName: string) =>
  `api/ports/${encodeURIComponent(repo)}/${encodeURIComponent(wtName)}/logs`;

export const portsApi = {
  list: (): Promise<PortItem[]> => apiClient.get("api/ports").json(),

  listeners: (): Promise<ListenerRow[]> =>
    apiClient.get("api/ports/listeners").json(),

  stale: (): Promise<StaleItem[]> => apiClient.get("api/ports/stale").json(),

  prune: (): Promise<PruneResult> =>
    apiClient.post("api/ports/prune").json(),

  serve: (repo: string, wtName: string): Promise<ServeResult> =>
    apiClient
      .post(`api/ports/${encodeURIComponent(repo)}/${encodeURIComponent(wtName)}/serve`)
      .json(),

  down: (repo: string, wtName: string): Promise<ServeResult> =>
    apiClient
      .post(`api/ports/${encodeURIComponent(repo)}/${encodeURIComponent(wtName)}/down`)
      .json(),

  getDevConfig: (repo: string, wtName: string): Promise<DevConfig> =>
    apiClient.get(devConfigURL(repo, wtName)).json(),

  putDevConfig: (
    repo: string,
    wtName: string,
    services: DevService[],
  ): Promise<DevConfig> =>
    apiClient.put(devConfigURL(repo, wtName), { json: { services } }).json(),

  logs: (repo: string, wtName: string): Promise<{ logs: ServiceLog[] }> =>
    apiClient.get(logsURL(repo, wtName)).json(),
};
