import { apiClient } from "./client";

export interface ProxyStatus {
  running: boolean;
  port: number;
  bind: string; // "0.0.0.0" (既定, LAN からも到達可) / "127.0.0.1" (loopback のみ)
  suffix: string; // ".wt.localhost"
}

export const proxyApi = {
  status: (): Promise<ProxyStatus> => apiClient.get("api/proxy").json(),
  start: (): Promise<ProxyStatus> => apiClient.post("api/proxy/start").json(),
  stop: (): Promise<ProxyStatus> => apiClient.post("api/proxy/stop").json(),
};
