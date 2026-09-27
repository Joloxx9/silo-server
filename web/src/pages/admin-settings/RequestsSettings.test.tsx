// @vitest-environment jsdom
import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { toast } from "sonner";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { adminKeys } from "@/hooks/queries/keys";

import {
  calls,
  choose,
  conflict,
  fallback,
  group,
  mount,
  problem,
  radarr,
  reply,
  route,
  serve,
  server,
  serverOptions,
  settings,
  sonarr,
  stubBrowser,
} from "./requestsSettings.fixtures";

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
vi.mock("@/hooks/queries/admin/users", async () => {
  const { adminUsers } = await import("./requestsSettings.mockData");
  return { useAdminUsers: () => adminUsers };
});
vi.mock("@/hooks/queries/admin/plugins", async () => {
  const { pluginInstallations } = await import("./requestsSettings.mockData");
  return { useAdminPluginInstallations: () => pluginInstallations };
});

beforeEach(stubBrowser);
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  vi.unstubAllGlobals();
});

describe("Requests settings: general", () => {
  it("saves through the save bar with the validator it read, and reloads after a 412", async () => {
    let reads = 0;
    serve({
      handlers: {
        "GET /api/v2/admin/request-settings": (options) => {
          reads += 1;
          return reply(
            options,
            { ...settings, global_max_requests: reads === 1 ? 5 : 9 },
            reads === 1 ? '"initial"' : '"reloaded"',
          );
        },
        "PUT /api/v2/admin/request-settings": (options) =>
          options.headers?.["If-Match"] === '"initial"'
            ? Promise.reject(conflict())
            : reply(options, { ...settings, global_max_requests: 10 }, '"saved"'),
      },
    });
    const client = mount();
    const limit = (await screen.findByLabelText("Request limit")) as HTMLInputElement;
    fireEvent.change(limit, { target: { value: "13" } });

    // A background refresh must not replace the edit or its validator.
    act(() =>
      client.setQueryData(adminKeys.requestSettings(), {
        ...settings,
        global_max_requests: 22,
        etag: '"background"',
        updated_at: "",
      }),
    );
    expect(limit.value).toBe("13");

    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await within(group("General")).findByRole("alert");
    expect(limit.value).toBe("13");
    expect(calls("PUT /api/v2/admin/request-settings")).toHaveLength(1);
    expect(calls("PUT /api/v2/admin/request-settings")[0]).toMatchObject({
      headers: { "If-Match": '"initial"' },
      body: { global_max_requests: 13, global_window_days: 7 },
    });
    // Nothing left that could save until the admin reloads.
    expect((screen.getByRole("button", { name: "Save" }) as HTMLButtonElement).disabled).toBe(true);

    fireEvent.click(screen.getByRole("button", { name: "Reload latest version" }));
    await waitFor(() =>
      expect((screen.getByLabelText("Request limit") as HTMLInputElement).value).toBe("9"),
    );
    expect(screen.queryByRole("button", { name: "Save" })).toBeNull();

    fireEvent.change(screen.getByLabelText("Request limit"), { target: { value: "10" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(calls("PUT /api/v2/admin/request-settings")).toHaveLength(2));
    expect(calls("PUT /api/v2/admin/request-settings")[1]).toMatchObject({
      headers: { "If-Match": '"reloaded"' },
      body: { global_max_requests: 10 },
    });
  });

  it("does not offer a request limit below 1", async () => {
    serve();
    mount();
    const limit = await screen.findByLabelText("Request limit");
    fireEvent.change(limit, { target: { value: "0" } });
    expect(screen.getByText(/Allow at least 1/)).toBeInTheDocument();
    expect((screen.getByRole("button", { name: "Save" }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("explains what the 4K switch does", async () => {
    serve();
    mount();
    expect(
      await screen.findByText(
        "Normally only requesters who can play 4K get a 4K copy, and only when a server takes 4K. With this on, every request also asks for 4K.",
      ),
    ).toBeInTheDocument();
  });

  it("calls approval and the limit server-wide defaults that groups and accounts override", async () => {
    serve();
    mount();
    const limit = await screen.findByLabelText("Request limit");
    const help = document.getElementById(limit.getAttribute("aria-describedby")!);
    expect(help).toHaveTextContent(
      "Declined and failed requests don't count. The server-wide default. Access groups and accounts can override this.",
    );
    expect(within(help!).getByRole("link", { name: "Access groups" })).toHaveAttribute(
      "href",
      "/admin/access-groups",
    );
    const approval = screen.getByRole("combobox", { name: "Approval" });
    expect(document.getElementById(approval.getAttribute("aria-describedby")!)).toHaveTextContent(
      "The server-wide default. Access groups and accounts can override this.",
    );
    // Per-account limits no longer have a page of their own.
    expect(screen.queryByRole("link", { name: "User overrides" })).toBeNull();
    expect(screen.getByRole("link", { name: "Users" })).toHaveAttribute("href", "/admin/users");
  });
});

async function openServer(name: string) {
  mount();
  const tile = await screen.findByRole("group", { name });
  fireEvent.click(within(tile).getByRole("button", { name: "Edit" }));
  return screen.findByRole("dialog");
}

describe("Requests settings: servers", () => {
  it("names each server by type and hides the switches routing owns", async () => {
    serve({
      handlers: {
        "PUT /api/v2/admin/request-integrations/{id}": (options) =>
          reply(options, radarr, '"saved"'),
      },
    });
    mount();
    const tile = await screen.findByRole("group", { name: "Radarr Anime" });
    expect(within(tile).getByText("Radarr")).toBeInTheDocument();
    expect(within(group("Sonarr")).getByText("Everything else (series)")).toBeInTheDocument();
    fireEvent.click(within(group("Radarr")).getByRole("button", { name: "Edit" }));
    const dialog = await screen.findByRole("dialog");

    for (const hidden of ["Default (HD/1080p)", "4K instance", "Enable anime overrides"]) {
      expect(within(dialog).queryByText(hidden)).toBeNull();
    }
    // The plugin's collapsed Library section is shown open.
    expect(within(dialog).getByText("Service")).toBeInTheDocument();
    expect(within(dialog).getByText("Quality profile")).toBeInTheDocument();
    expect(within(dialog).queryByRole("button", { name: "Show" })).toBeNull();

    const key = within(dialog).getByLabelText("API key") as HTMLInputElement;
    expect(key.type).toBe("password");
    expect(key.value).toBe("");
    expect(key.placeholder).toBe("••••••••••••");

    fireEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(calls("PUT /api/v2/admin/request-integrations/{id}")).toHaveLength(1),
    );
    const [write] = calls("PUT /api/v2/admin/request-integrations/{id}");
    expect(write).toMatchObject({ headers: { "If-Match": '"initial"' } });
    const body = write!.body as { api_key_ref?: string; plugin_config: Record<string, unknown> };
    // A blank key keeps the saved one, and hidden settings pass through.
    expect(body.api_key_ref).toBeUndefined();
    expect(body.plugin_config).toMatchObject({ is_default: true, quality_profile_id: 1 });
  });

  it("tests the connection and reports what it found or why it failed", async () => {
    let fail = false;
    serve({
      handlers: {
        "POST /api/v2/admin/request-integrations/{id}/options": (options) =>
          fail
            ? Promise.reject(problem(502, "dependency_unavailable", "401 Unauthorized from Radarr"))
            : reply(options, { options: serverOptions }),
      },
    });
    const dialog = await openServer("Radarr");
    const test = within(dialog).getByRole("button", { name: "Test" });

    fireEvent.click(test);
    expect(
      await within(dialog).findByText("Connected — 2 quality profiles, 2 root folders"),
    ).toBeInTheDocument();

    fail = true;
    fireEvent.click(within(dialog).getByRole("button", { name: "Test" }));
    expect(await within(dialog).findByText("401 Unauthorized from Radarr")).toBeInTheDocument();
    const probes = calls("POST /api/v2/admin/request-integrations/{id}/options");
    expect(probes.at(-1)).toMatchObject({ path: { id: "radarr-1" } });
  });

  it("keeps the delete confirmation open after a stale delete", async () => {
    serve({
      handlers: {
        "DELETE /api/v2/admin/request-integrations/{id}": () => Promise.reject(conflict()),
      },
    });
    const dialog = await openServer("Radarr");
    fireEvent.click(within(dialog).getByRole("button", { name: "Delete" }));
    const confirm = await screen.findByRole("alertdialog");
    fireEvent.click(within(confirm).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(within(confirm).getByRole("alert")).toBeTruthy());
    expect(screen.getByRole("alertdialog")).toBeTruthy();
    expect(calls("DELETE /api/v2/admin/request-integrations/{id}")).toHaveLength(1);
    expect(calls("DELETE /api/v2/admin/request-integrations/{id}")[0]).toMatchObject({
      headers: { "If-Match": '"initial"' },
    });
  });

  it("shows save errors beside their fields and clears them on edit", async () => {
    const errors = [
      { location: "body.name", code: "invalid", detail: "Server name is rejected" },
      { location: "body.api_key_ref", code: "invalid", detail: "Re-enter the key" },
      { location: "body.base_url", code: "invalid", detail: "Server URL is rejected" },
      { location: "body.installation_id", code: "invalid", detail: "Choose another plugin" },
      { location: "body.is_default", code: "invalid", detail: "Radarr already has a default" },
    ];
    serve({
      handlers: {
        "PUT /api/v2/admin/request-integrations/{id}": () =>
          Promise.reject(problem(422, "validation_failed", "Review invalid fields", errors)),
      },
    });
    const dialog = await openServer("Radarr");
    fireEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    for (const error of errors) {
      expect(await within(dialog).findByText(error.detail)).toBeInTheDocument();
    }
    const name = within(dialog).getByLabelText("Name");
    expect(name.getAttribute("aria-invalid")).toBe("true");
    fireEvent.change(name, { target: { value: "Corrected" } });
    expect(within(dialog).queryByText("Server name is rejected")).toBeNull();
    expect(name.getAttribute("aria-invalid")).toBe("false");
  });
});

describe("Requests settings: after a save", () => {
  it("keeps a saved general edit while the refetch is slow, and saves the next against the new validator", async () => {
    let stored = { ...settings };
    let version = 0;
    let slow = false;
    serve({
      handlers: {
        "GET /api/v2/admin/request-settings": (options) =>
          slow ? new Promise(() => {}) : reply(options, stored, `"settings-${version}"`),
        "PUT /api/v2/admin/request-settings": (options) => {
          stored = { ...stored, ...(options.body as object) };
          version += 1;
          slow = true;
          return reply(options, stored, `"settings-${version}"`);
        },
      },
    });
    mount();
    const allow = await screen.findByRole("switch", { name: "Allow requests" });
    await waitFor(() => expect(calls("GET /api/v2/admin/request-routes")).toHaveLength(1));
    fireEvent.click(allow);
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(calls("PUT /api/v2/admin/request-settings")).toHaveLength(1));
    await waitFor(() => expect(screen.queryByRole("button", { name: "Save" })).toBeNull());

    // The edit stays saved on screen, not the replaced record.
    expect(screen.getByRole("switch", { name: "Allow requests" })).toHaveAttribute(
      "aria-checked",
      "false",
    );
    // Settings writes leave the routing list alone.
    expect(calls("GET /api/v2/admin/request-routes")).toHaveLength(1);

    fireEvent.change(screen.getByLabelText("Request limit"), { target: { value: "8" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(calls("PUT /api/v2/admin/request-settings")).toHaveLength(2));
    expect(calls("PUT /api/v2/admin/request-settings")[1]).toMatchObject({
      headers: { "If-Match": '"settings-1"' },
      body: { requests_enabled: false, global_max_requests: 8 },
    });
  });
});

describe("Requests settings: recovering from errors", () => {
  it("clears a conflict on discard and starts over from the latest version", async () => {
    let reads = 0;
    serve({
      handlers: {
        "GET /api/v2/admin/request-settings": (options) => {
          reads += 1;
          return reply(
            options,
            { ...settings, global_max_requests: reads === 1 ? 5 : 9 },
            reads === 1 ? '"initial"' : '"reloaded"',
          );
        },
        "PUT /api/v2/admin/request-settings": () => Promise.reject(conflict()),
      },
    });
    mount();
    fireEvent.change(await screen.findByLabelText("Request limit"), { target: { value: "13" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await within(group("General")).findByRole("alert");

    fireEvent.click(screen.getByRole("button", { name: "Discard" }));
    expect(within(group("General")).queryByRole("alert")).toBeNull();
    await waitFor(() =>
      expect((screen.getByLabelText("Request limit") as HTMLInputElement).value).toBe("9"),
    );
  });
});

describe("Requests settings: server delete and kind", () => {
  it("does not offer Delete for a server routing still sends to, and says which routes", async () => {
    serve({
      routes: [
        fallback("movie"),
        route({
          id: "anime",
          name: "Anime",
          media_type: "series",
          conditions: { anime: true },
          uhd: { integration_id: "sonarr-1" },
        }),
        fallback("series", "sonarr-1"),
      ],
    });
    const dialog = await openServer("Sonarr");
    const del = within(dialog).getByRole("button", { name: "Delete" }) as HTMLButtonElement;
    expect(del.disabled).toBe(true);
    expect(
      within(dialog).getByText(
        "Routing still sends requests here (Anime 4K (series) · Everything else (series)); send them elsewhere first.",
      ),
    ).toBeInTheDocument();
  });

  it("does not let a routed server change type, and says why", async () => {
    serve();
    const dialog = await openServer("Sonarr");
    const service = within(dialog).getByRole("combobox", { name: "Service" });
    expect(service).toBeDisabled();
    const reason = within(dialog).getByText(
      "Routing sends requests here (Everything else (series)). Change that first to switch the type.",
    );
    expect(service.getAttribute("aria-describedby")).toBe(reason.id);

    // An unrouted server can still change type.
    cleanup();
    serve();
    const other = await openServer("Radarr Anime");
    expect(within(other).getByRole("combobox", { name: "Service" })).toBeEnabled();
  });

  it("drops the retired default switches when a server changes kind", async () => {
    serve({
      handlers: {
        "PUT /api/v2/admin/request-integrations/{id}": (options) =>
          reply(options, radarr, '"saved"'),
      },
    });
    const dialog = await openServer("Radarr Anime");
    await choose(dialog, "Service", "Sonarr (series)");
    fireEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(calls("PUT /api/v2/admin/request-integrations/{id}")).toHaveLength(1),
    );
    expect(calls("PUT /api/v2/admin/request-integrations/{id}")[0]!.body).toMatchObject({
      supported_media_types: ["series"],
      plugin_config: { service_kind: "sonarr", is_default: false },
    });
  });

  it("keeps the default switches when the kind is unchanged", async () => {
    serve({
      handlers: {
        "PUT /api/v2/admin/request-integrations/{id}": (options) =>
          reply(options, radarr, '"saved"'),
      },
    });
    const dialog = await openServer("Radarr Anime");
    fireEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(calls("PUT /api/v2/admin/request-integrations/{id}")).toHaveLength(1),
    );
    expect(calls("PUT /api/v2/admin/request-integrations/{id}")[0]!.body).toMatchObject({
      plugin_config: { service_kind: "radarr", is_default: true },
    });
  });
});

describe("Requests settings: servers and routing", () => {
  it("leaves routing out when the server does not offer it", async () => {
    serve({
      handlers: {
        "GET /api/v2/admin/requests/capabilities": (options) =>
          reply(options, { available: true, guarded_configuration: true, routing: false }),
      },
    });
    mount();
    expect(await screen.findByRole("group", { name: "Servers" })).toBeInTheDocument();
    expect(screen.queryByRole("group", { name: "Where requests go" })).toBeNull();
    expect(calls("GET /api/v2/admin/request-routes")).toHaveLength(0);
    expect(calls("GET /api/v2/admin/request-routing")).toHaveLength(0);
  });

  it("lets the last server of its kind go with the Everything else only it serves", async () => {
    serve({
      servers: [radarr, sonarr],
      routes: [fallback("movie", "radarr-1"), fallback("series", "sonarr-1")],
      handlers: {
        "DELETE /api/v2/admin/request-integrations/{id}": (options) => reply(options, undefined),
      },
    });
    const dialog = await openServer("Radarr");
    const del = within(dialog).getByRole("button", { name: "Delete" }) as HTMLButtonElement;
    expect(del.disabled).toBe(false);
    fireEvent.click(del);
    const confirm = await screen.findByRole("alertdialog");
    expect(confirm).toHaveTextContent(
      "Everything else for movies goes with it, so those requests have nowhere to go until you add another server.",
    );
    fireEvent.click(within(confirm).getByRole("button", { name: "Delete" }));
    await waitFor(() =>
      expect(calls("DELETE /api/v2/admin/request-integrations/{id}")).toHaveLength(1),
    );
    // The delete can change Everything else, so the routes are read again.
    await waitFor(() => expect(calls("GET /api/v2/admin/request-routes")).toHaveLength(2));
  });

  async function addRadarr(defaulted: boolean) {
    const added = server("radarr-9", "Radarr Main", "radarr");
    let created = false;
    const routes = () => [
      fallback("movie", created && defaulted ? "radarr-9" : undefined),
      fallback("series", "sonarr-1"),
    ];
    serve({
      servers: [sonarr],
      handlers: {
        "POST /api/v2/admin/request-integrations": (options) => {
          created = true;
          return reply(options, added, '"new"');
        },
        // The server makes Everything else send to the new server only
        // when its own rules allow; the toast follows what it did.
        "GET /api/v2/admin/request-routes": (options) => reply(options, { items: routes() }),
        "GET /api/v2/admin/request-routes/{id}": (options) =>
          reply(
            options,
            routes().find((r) => r.id === options.path?.id),
          ),
      },
    });
    mount();
    fireEvent.click(await screen.findByRole("button", { name: "Add server" }));
    const dialog = await screen.findByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText("Name"), { target: { value: "Radarr Main" } });
    fireEvent.change(within(dialog).getByLabelText("URL"), {
      target: { value: "http://radarr:7878" },
    });
    fireEvent.change(within(dialog).getByLabelText("API key"), { target: { value: "key" } });
    await choose(dialog, "Service", "Radarr (movies)");
    fireEvent.click(within(dialog).getByRole("button", { name: "Add server" }));
    await waitFor(() => expect(toast.success).toHaveBeenCalled());
    expect(calls("GET /api/v2/admin/request-routes").length).toBeGreaterThanOrEqual(2);
  }

  it("says where every movie request goes when the new server became Everything else", async () => {
    await addRadarr(true);
    expect(toast.success).toHaveBeenCalledWith(
      "Radarr Main added. Every movie request now goes to it.",
    );
  });

  it("just says the server was added when the server kept Everything else as it was", async () => {
    await addRadarr(false);
    expect(toast.success).toHaveBeenCalledWith("Server added");
    expect(toast.success).toHaveBeenCalledTimes(1);
  });
});
