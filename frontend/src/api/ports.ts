import { apiClient } from "./client";

export interface PortState {
  port: number;
  listening: boolean;
  pid?: number;
  proc?: string;
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
  services: DevService[];
}

const devConfigURL = (repo: string, wtName: string) =>
  `api/ports/${encodeURIComponent(repo)}/${encodeURIComponent(wtName)}/devconfig`;

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
};
