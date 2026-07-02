import { http, HttpResponse } from "msw";

export const handlers = [
  http.get("/api/repos", () => HttpResponse.json([])),
  http.get("/api/trees", () => HttpResponse.json([])),
  http.get("/api/ports", () => HttpResponse.json([])),
  http.get("/api/ports/stale", () => HttpResponse.json([])),
  http.get("/api/proxy", () =>
    HttpResponse.json({ running: false, port: 8088, suffix: ".wt.localhost" }),
  ),
];
