import ky from "ky";
import { logger } from "../lib/logger";

// prod (embed) と dev (Vite proxy) は同オリジン /api を使う
const API_BASE_URL = import.meta.env.VITE_API_BASE_URL ?? "";

export const apiClient = ky.create({
  prefixUrl: API_BASE_URL,
  headers: { "Content-Type": "application/json" },
  hooks: {
    beforeError: [
      async (error) => {
        const { response } = error;
        if (response) {
          try {
            const body = (await response.json()) as { error?: string };
            error.message = body.error ?? error.message;
            logger.error(`API ${response.url} failed: ${response.status}`, error.message);
          } catch {
            logger.warn(`Failed to parse error response: ${response.url}`);
          }
        } else {
          logger.error("Network request failed", error.message);
        }
        return error;
      },
    ],
  },
});
