// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { v2, V2ProblemError } from "@/api/v2/request";
import AdminRequests from "./AdminRequests";

vi.mock("@/api/v2/request", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/api/v2/request")>()),
  v2: vi.fn(),
}));
vi.mock("@/api/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/api/client")>()),
  captureProfileRequestContext: () => ({
    accessToken: "test",
    profileId: "profile",
    profileToken: null,
    authContextVersion: 1,
    serverOrigin: "",
  }),
  isProfileRequestContextCurrent: () => true,
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
vi.mock("@/hooks/queries/admin/users", () => ({
  useAdminUsers: () => ({ data: [{ id: 1, username: "member" }], isLoading: false }),
}));
const conflict = () =>
  new V2ProblemError(
    "updateRequestUserLimit",
    {
      type: "https://silo.test/problems/precondition_failed",
      title: "Changed",
      status: 412,
      detail: "Changed",
      instance: "test",
    },
    null,
    '"newer"',
  );
function reply(options: unknown, body: unknown, etag = '"initial"') {
  (options as { onResponse?: (r: Response) => void })?.onResponse?.(
    new Response(null, { headers: { ETag: etag } }),
  );
  return Promise.resolve(body) as never;
}
function mount(tab: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: 3 } },
  });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[`/admin/requests?tab=${tab}`]}>
        <Routes>
          <Route path="/admin/requests" element={<AdminRequests />} />
          <Route path="/admin/settings/requests" element={<h1>Request settings page</h1>} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return client;
}
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("request administration", () => {
  it.each(["settings", "integrations"])(
    "sends the retired ?tab=%s to the Requests settings page",
    async (tab) => {
      vi.mocked(v2).mockImplementation((operation, options) => {
        if (operation === "GET /api/v2/admin/requests/capabilities")
          return reply(options, { available: true, guarded_configuration: true });
        throw new Error(operation);
      });
      mount(tab);
      expect(
        await screen.findByRole("heading", { name: "Request settings page" }),
      ).toBeInTheDocument();
    },
  );

  it("links to the Requests settings page and keeps only the queue and overrides tabs", async () => {
    vi.mocked(v2).mockImplementation((operation, options) => {
      if (operation === "GET /api/v2/admin/requests/capabilities")
        return reply(options, { available: true, guarded_configuration: true });
      if (operation === "GET /api/v2/admin/requests")
        return reply(options, { items: [], page: { has_more: false } });
      throw new Error(operation);
    });
    mount("queue");
    expect(await screen.findByRole("link", { name: "Request settings" })).toHaveAttribute(
      "href",
      "/admin/settings/requests",
    );
    expect(screen.getAllByRole("tab").map((tab) => tab.textContent)).toEqual([
      "Queue",
      "User Overrides",
    ]);
  });

  it("offers Decline only for requests nothing has been sent for", async () => {
    const request = (id: string, title: string, status: string, targets: unknown[] = []) => ({
      id,
      provider: "tmdb",
      media_type: "movie",
      tmdb_id: Number(id.replace(/\D/g, "")),
      title,
      status,
      outcome: "active",
      requested_by_user_id: "1",
      is_anime: false,
      targets,
      created_at: "2026-09-01T00:00:00Z",
      updated_at: "2026-09-01T00:00:00Z",
    });
    vi.mocked(v2).mockImplementation((operation, options) => {
      if (operation === "GET /api/v2/admin/requests/capabilities")
        return reply(options, { available: true, guarded_configuration: true });
      if (operation === "GET /api/v2/admin/requests")
        return reply(options, {
          items: [
            request("r1", "Waiting Title", "pending"),
            request("r2", "Approved Title", "approved"),
            request("r3", "Sent Title", "queued", [
              {
                id: "1",
                request_id: "r3",
                quality: "1080p",
                is_anime: false,
                status: "queued",
                created_at: "2026-09-01T00:00:00Z",
                updated_at: "2026-09-01T00:00:00Z",
              },
            ]),
          ],
          page: { has_more: false },
        });
      throw new Error(operation);
    });
    mount("queue");
    const declineFor = async (title: string) => {
      const row = (await screen.findByText(title)).closest("tr") as HTMLElement;
      return within(row).getByRole("button", { name: "Decline" }) as HTMLButtonElement;
    };
    expect((await declineFor("Waiting Title")).disabled).toBe(false);
    expect((await declineFor("Approved Title")).disabled).toBe(false);
    expect((await declineFor("Sent Title")).disabled).toBe(true);
  });

  it("keeps user override edits and validator until explicit reload after a stale response", async () => {
    let reads = 0;
    vi.mocked(v2).mockImplementation((operation, options) => {
      if (operation === "GET /api/v2/admin/requests/capabilities")
        return reply(options, { available: true, guarded_configuration: true });
      if (operation === "GET /api/v2/admin/request-users/{user_id}/limit")
        return reply(
          options,
          {
            user_id: "1",
            limit_mode: "custom",
            max_requests: ++reads === 1 ? 3 : 6,
            window_days: 7,
            approval_mode: "inherit",
          },
          reads === 1 ? '"initial"' : '"reloaded"',
        );
      if (operation === "PUT /api/v2/admin/request-users/{user_id}/limit")
        return Promise.reject(conflict());
      throw new Error(operation);
    });
    mount("overrides");
    await screen.findByText("Save Override");
    const input = screen.getAllByRole("spinbutton")[0]!;
    fireEvent.change(input, { target: { value: "11" } });
    fireEvent.click(screen.getByText("Save Override"));
    await screen.findByRole("alert");
    expect((input as HTMLInputElement).value).toBe("11");
    expect(
      vi
        .mocked(v2)
        .mock.calls.filter(([op]) => op === "PUT /api/v2/admin/request-users/{user_id}/limit"),
    ).toHaveLength(1);
    const writes = () =>
      vi
        .mocked(v2)
        .mock.calls.filter(([op]) => op === "PUT /api/v2/admin/request-users/{user_id}/limit");
    expect(writes()[0]![1]).toMatchObject({
      headers: { "If-Match": '"initial"' },
      body: { max_requests: 11 },
    });
    expect(
      (screen.getByText("Save Override").closest("button") as HTMLButtonElement).disabled,
    ).toBe(true);
    fireEvent.click(screen.getByText("Reload latest version"));
    await waitFor(() =>
      expect((screen.getAllByRole("spinbutton")[0] as HTMLInputElement).value).toBe("6"),
    );
    fireEvent.click(screen.getByText("Save Override"));
    await waitFor(() => expect(writes()).toHaveLength(2));
    expect(writes()[1]![1]).toMatchObject({
      headers: { "If-Match": '"reloaded"' },
      body: { max_requests: 6 },
    });
  });
});
