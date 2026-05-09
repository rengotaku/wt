import "@testing-library/jest-dom/vitest";
import { beforeAll, afterEach, afterAll } from "vitest";
import { server } from "./mocks/server";

// jsdom 29 + vitest 4 で localStorage が正しく初期化されない場合の workaround
const localStorageStore: Record<string, string> = {};
Object.defineProperty(window, "localStorage", {
  value: {
    getItem: (key: string): string | null => localStorageStore[key] ?? null,
    setItem: (key: string, value: string): void => { localStorageStore[key] = value; },
    removeItem: (key: string): void => { delete localStorageStore[key]; },
    clear: (): void => { Object.keys(localStorageStore).forEach((k) => delete localStorageStore[k]); },
    key: (index: number): string | null => Object.keys(localStorageStore)[index] ?? null,
    get length() { return Object.keys(localStorageStore).length; },
  },
  writable: true,
});

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());
