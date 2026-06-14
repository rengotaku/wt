import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@/test/test-utils";
import { SettingsPage } from "./SettingsPage";

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  Toaster: () => null,
}));

vi.mock("@/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/api")>();
  return {
    ...actual,
    settingsApi: { get: vi.fn(), update: vi.fn() },
  };
});

describe("SettingsPage", () => {
  beforeEach(async () => {
    const { settingsApi } = await import("@/api");
    vi.mocked(settingsApi.get).mockResolvedValue({
      dev_ports: { start: 9000, end: 9999, block_size: 5 },
    });
    vi.mocked(settingsApi.update).mockResolvedValue({
      dev_ports: { start: 9500, end: 9700, block_size: 5 },
    });
  });

  it("loads the current band into the inputs", async () => {
    render(<SettingsPage />);
    await waitFor(() => {
      expect(screen.getByDisplayValue("9000")).toBeInTheDocument();
    });
    expect(screen.getByDisplayValue("9999")).toBeInTheDocument();
  });

  it("sends start/end to the API on save", async () => {
    const { settingsApi } = await import("@/api");
    render(<SettingsPage />);
    await waitFor(() => {
      expect(screen.getByDisplayValue("9000")).toBeInTheDocument();
    });

    fireEvent.change(screen.getByDisplayValue("9000"), { target: { value: "9500" } });
    fireEvent.change(screen.getByDisplayValue("9999"), { target: { value: "9700" } });
    fireEvent.click(screen.getByRole("button", { name: "保存" }));

    await waitFor(() => {
      expect(vi.mocked(settingsApi.update)).toHaveBeenCalled();
    });
    expect(vi.mocked(settingsApi.update).mock.calls[0][0]).toEqual({
      dev_ports: { start: 9500, end: 9700 },
    });
  });
});
