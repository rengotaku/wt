import { apiClient } from "./client";

export interface PortState {
  port: number;
  listening: boolean;
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
  domain?: string; // <label>.wt.localhost when a domain service exists
}

export interface ServeResult {
  output: string;
  running: boolean;
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
