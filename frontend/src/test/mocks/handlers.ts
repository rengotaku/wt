import { http, HttpResponse } from "msw";

export const handlers = [
  http.get("/api/repos", () => HttpResponse.json([])),
  http.get("/api/trees", () => HttpResponse.json([])),
];
