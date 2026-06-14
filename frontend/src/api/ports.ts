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
}

export interface ServeResult {
  output: string;
  running: boolean;
}

export const portsApi = {
  list: (): Promise<PortItem[]> => apiClient.get("api/ports").json(),

  serve: (repo: string, wtName: string): Promise<ServeResult> =>
    apiClient
      .post(`api/ports/${encodeURIComponent(repo)}/${encodeURIComponent(wtName)}/serve`)
      .json(),

  down: (repo: string, wtName: string): Promise<ServeResult> =>
    apiClient
      .post(`api/ports/${encodeURIComponent(repo)}/${encodeURIComponent(wtName)}/down`)
      .json(),
};
