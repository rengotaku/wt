import { apiClient } from "./client";

export interface BuildInfoResponse {
  is_dev: boolean;
  build_commit: string;
  build_commit_time: number; // unix seconds, 0 if unknown
  start_time: number; // unix seconds
  source_repo: string;
  head_commit: string;
  head_commit_time: number; // unix seconds, 0 if unknown
  head_branch: string;
  is_stale: boolean;
  error?: string;
}

export const buildInfoApi = {
  get: (): Promise<BuildInfoResponse> => apiClient.get("api/build-info").json(),
};
