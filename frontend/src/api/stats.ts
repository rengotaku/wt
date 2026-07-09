import { apiClient } from "./client";

export interface ProcessStatsResponse {
  warn_bytes: number;
  danger_bytes: number;
  items: WorktreeProcessStats[];
}

export interface WorktreeProcessStats {
  repo: string;
  wt_name: string;
  total_rss_bytes: number;
  level: "ok" | "warn" | "danger";
  services: ServiceProcessStat[];
}

export interface ServiceProcessStat {
  name: string;
  pid: number;
  port: number;
  alive: boolean;
  procs: number;
  rss_bytes: number;
  uptime_sec: number;
}

export const statsApi = {
  list: (): Promise<ProcessStatsResponse> =>
    apiClient.get("api/process-stats").json(),
};

// 稼働時間の人間可読表示（例: 45s / 12m / 3h05m / 2d04h）。
export function formatDuration(sec: number): string {
  if (sec < 60) return `${sec}s`;
  if (sec < 3600) return `${Math.floor(sec / 60)}m`;
  if (sec < 86400) {
    const h = Math.floor(sec / 3600);
    const m = Math.floor((sec % 3600) / 60);
    return `${h}h${String(m).padStart(2, "0")}m`;
  }
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  return `${d}d${String(h).padStart(2, "0")}h`;
}

export function formatBytes(bytes: number): string {
  if (bytes === 0) return "0B";
  const k = 1024;
  const sizes = ["B", "K", "M", "G", "T", "P"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + sizes[i];
}
