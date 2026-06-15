import { apiClient } from "./client";

export interface ProxyStatus {
  running: boolean;
  port: number;
  suffix: string; // ".wt.localhost"
}

export const proxyApi = {
  status: (): Promise<ProxyStatus> => apiClient.get("api/proxy").json(),
  start: (): Promise<ProxyStatus> => apiClient.post("api/proxy/start").json(),
  stop: (): Promise<ProxyStatus> => apiClient.post("api/proxy/stop").json(),
};
