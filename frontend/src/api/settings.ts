import { apiClient } from "./client";

export interface DevPorts {
  start: number;
  end: number;
  block_size: number;
}

export interface Settings {
  dev_ports: DevPorts;
}

export interface UpdateSettingsRequest {
  dev_ports: { start: number; end: number };
}

export const settingsApi = {
  get: (): Promise<Settings> => apiClient.get("api/settings").json(),

  update: (req: UpdateSettingsRequest): Promise<Settings> =>
    apiClient.put("api/settings", { json: req }).json(),
};
