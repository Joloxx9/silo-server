// @vitest-environment jsdom
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { toast } from "sonner";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { PluginAdminForm } from "@/api/types";
import { v2, V2ProblemError } from "@/api/v2/request";
import { adminKeys } from "@/hooks/queries/keys";

import RequestsSettings from "./RequestsSettings";

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
  useAdminUsers: () => ({
    data: [
      { id: 1, username: "admin" },
      { id: 2, username: "kid" },
    ],
    isLoading: false,
  }),
}));

// The Sonarr/Radarr plugin's form, trimmed to the fields these tests touch.
const field = (extra: Record<string, unknown>) => ({
  required: false,
  secret: false,
  multiline: false,
  ...extra,
});
const descriptor = {
  fields: [
    field({
      key: "service_kind",
      label: "Service",
      control: "SELECT",
      required: true,
      options: [
        { value: "radarr", label: "Radarr (movies)" },
        { value: "sonarr", label: "Sonarr (series)" },
      ],
    }),
    field({ key: "root_folder", label: "Root folder", control: "SELECT", dynamic_options: true }),
    field({
      key: "quality_profile_id",
      label: "Quality profile",
      control: "SELECT",
      required: true,
      dynamic_options: true,
    }),
    field({ key: "tags", label: "Tags", control: "MULTI_SELECT", dynamic_options: true }),
    field({ key: "is_default", label: "Default (HD/1080p)", control: "SWITCH" }),
    field({ key: "is_4k", label: "4K instance", control: "SWITCH" }),
    field({
      key: "is_default_4k",
      label: "Default 4K (2160p)",
      control: "SWITCH",
      show_when: [{ field: "is_4k", equals: ["true"] }],
    }),
    field({ key: "search_on_add", label: "Search on add", control: "SWITCH", default_value: true }),
    field({ key: "anime_enabled", label: "Enable anime overrides", control: "SWITCH" }),
    field({
      key: "anime_root_folder",
      label: "Anime root folder",
      control: "SELECT",
      dynamic_options: true,
      show_when: [{ field: "anime_enabled", equals: ["true"] }],
    }),
  ],
  sections: [
    {
      key: "library",
      title: "Library",
      collapsible: true,
      collapsed_default: true,
      field_keys: [
        "service_kind",
        "root_folder",
        "quality_profile_id",
        "tags",
        "is_default",
        "is_4k",
        "is_default_4k",
        "search_on_add",
      ],
    },
    {
      key: "anime",
      title: "Anime overrides",
      collapsible: false,
      collapsed_default: false,
      field_keys: ["anime_enabled", "anime_root_folder"],
    },
  ],
} as PluginAdminForm;
const jsonSchema = JSON.stringify({
  type: "object",
  properties: {
    service_kind: { type: "string" },
    root_folder: { type: "string" },
    quality_profile_id: { type: "integer" },
    tags: { type: "array", items: { type: "integer" } },
    is_default: { type: "boolean" },
    is_4k: { type: "boolean" },
    is_default_4k: { type: "boolean" },
    search_on_add: { type: "boolean" },
    anime_enabled: { type: "boolean" },
    anime_root_folder: { type: "string" },
  },
});
vi.mock("@/hooks/queries/admin/plugins", () => ({
  useAdminPluginInstallations: () => ({
    data: [
      {
        id: 1,
        plugin_id: "silo.requests.arr",
        enabled: true,
        capabilities: [
          {
            type: "request_router.v1",
            id: "arr",
            display_name: "Sonarr / Radarr",
            config_schema: [
              {
                key: "connection",
                title: "Connection",
                json_schema: jsonSchema,
                required: false,
                admin_form: descriptor,
              },
            ],
          },
        ],
      },
    ],
    isLoading: false,
  }),
}));

// Radix Select opens through pointer capture and measures with ResizeObserver,
// neither of which jsdom has.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
beforeEach(() => {
  vi.stubGlobal("ResizeObserver", ResizeObserverStub);
  window.HTMLElement.prototype.hasPointerCapture = () => false;
  window.HTMLElement.prototype.scrollIntoView = () => {};
  localStorage.clear();
});
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  vi.unstubAllGlobals();
});

const settings = {
  requests_enabled: true,
  global_max_requests: 5,
  global_window_days: 7,
  global_auto_approval_enabled: false,
  force_dual_quality: false,
};

function server(id: string, name: string, kind: "radarr" | "sonarr", extra = {}) {
  return {
    id,
    name,
    enabled: true,
    base_url: `http://${id}:7878`,
    has_api_key: true,
    installation_id: "1",
    capability_id: "arr",
    plugin_config: {
      service_kind: kind,
      quality_profile_id: 1,
      root_folder: kind === "radarr" ? "/movies" : "/tv",
      is_default: true,
    },
    supported_media_types: [kind === "radarr" ? "movie" : "series"],
    last_check_at: null,
    last_check_status: "",
    last_check_error: "",
    updated_at: "2026-09-05T00:00:00Z",
    ...extra,
  };
}
const radarr = server("radarr-1", "Radarr", "radarr");
const radarrAnime = server("radarr-2", "Radarr Anime", "radarr");
const sonarr = server("sonarr-1", "Sonarr", "sonarr");

type Route = {
  id: string;
  media_type: "movie" | "series";
  position: number;
  name: string;
  enabled: boolean;
  is_fallback: boolean;
  conditions: Record<string, unknown>;
  hd: { integration_id?: string; overrides?: Record<string, unknown> };
  uhd: { integration_id?: string; overrides?: Record<string, unknown> };
  skip_uhd: boolean;
};
function route(extra: Partial<Route> & Pick<Route, "id">): Route {
  return {
    media_type: "movie",
    position: 0,
    name: extra.id,
    enabled: true,
    is_fallback: false,
    conditions: {},
    hd: {},
    uhd: {},
    skip_uhd: false,
    ...extra,
  };
}
const fallback = (mediaType: "movie" | "series", hd?: string) =>
  route({
    id: `fallback-${mediaType}`,
    media_type: mediaType,
    position: 1000,
    name: "Everything else",
    is_fallback: true,
    hd: hd ? { integration_id: hd } : {},
  });

const serverOptions = {
  root_folder: [
    { value: "/movies", label: "/movies (1.2 TiB free)" },
    { value: "/anime", label: "/anime (300 GiB free)" },
  ],
  quality_profile_id: [{ value: "1", label: "HD-1080p" }],
  tags: [{ value: "2", label: "anime" }],
};

function reply(options: unknown, body: unknown, etag = '"initial"') {
  (options as { onResponse?: (r: Response) => void })?.onResponse?.(
    new Response(null, { headers: { ETag: etag } }),
  );
  return Promise.resolve(body) as never;
}
const problem = (status: number, type: string, detail: string, errors?: unknown[]) =>
  new V2ProblemError("test", {
    type: `https://silo.test/problems/${type}`,
    title: detail,
    status,
    detail,
    instance: "test",
    ...(errors ? { errors } : {}),
  } as never);
const conflict = () => problem(412, "precondition_failed", "Changed");

type Options = { path?: { id?: string }; body?: unknown; headers?: Record<string, string> };
type Handler = (options: Options) => unknown;

/**
 * Serves the page's reads from fixtures and lets a test replace any
 * operation. Every route is read with an ETag naming its id, so a test can
 * tell which validator a write sent.
 */
function serve({
  servers = [radarr, radarrAnime, sonarr],
  routes = [fallback("movie"), fallback("series", "sonarr-1")],
  handlers = {},
}: {
  servers?: ReturnType<typeof server>[];
  routes?: Route[];
  handlers?: Record<string, Handler>;
} = {}) {
  vi.mocked(v2).mockImplementation(((operation: string, options: Options) => {
    const custom = handlers[operation];
    if (custom) return custom(options);
    switch (operation) {
      case "GET /api/v2/admin/requests/capabilities":
        return reply(options, { available: true, guarded_configuration: true, routing: true });
      case "GET /api/v2/admin/request-settings":
        return reply(options, settings);
      case "GET /api/v2/admin/request-integrations":
        return reply(options, { items: servers, page: { has_more: false } });
      case "GET /api/v2/admin/request-integrations/{id}":
        return reply(
          options,
          servers.find((s) => s.id === options.path?.id),
        );
      case "POST /api/v2/admin/request-integrations/{id}/options":
        return reply(options, { options: serverOptions });
      case "GET /api/v2/admin/request-routes":
        return reply(options, { items: routes });
      case "GET /api/v2/admin/request-routes/{id}":
        return reply(
          options,
          routes.find((r) => r.id === options.path?.id),
          `"${options.path?.id}-v1"`,
        );
      case "GET /api/v2/requests/discover/networks":
      case "GET /api/v2/requests/discover/studios":
        return reply(options, { items: [] });
      default:
        return Promise.reject(new Error(`unexpected ${operation}`));
    }
  }) as never);
}

function calls(operation: string) {
  return vi
    .mocked(v2)
    .mock.calls.filter(([op]) => op === operation)
    .map(([, options]) => options as Options);
}

function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={["/admin/settings/requests"]}>
        <RequestsSettings />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return client;
}

const group = (name: string) => screen.getByRole("group", { name });
const user = () => userEvent.setup({ pointerEventsCheck: 0 });

async function choose(scope: HTMLElement, label: string, option: string) {
  await user().click(await within(scope).findByRole("combobox", { name: label }));
  await user().click(await screen.findByRole("option", { name: option }));
}

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

  it("saves a request limit of 0 and refuses a negative or empty one", async () => {
    serve({
      handlers: {
        "PUT /api/v2/admin/request-settings": (options) =>
          reply(options, { ...settings, global_max_requests: 0 }, '"saved"'),
      },
    });
    mount();
    const limit = await screen.findByLabelText("Request limit");
    const save = () => screen.getByRole("button", { name: "Save" }) as HTMLButtonElement;
    for (const value of ["-1", ""]) {
      fireEvent.change(limit, { target: { value } });
      expect(screen.getByText(/0 or more/)).toBeInTheDocument();
      expect(save().disabled).toBe(true);
    }

    // 0 with per-account limits lets only those accounts request.
    fireEvent.change(limit, { target: { value: "0" } });
    expect(screen.queryByText(/0 or more/)).toBeNull();
    fireEvent.click(save());
    await waitFor(() => expect(calls("PUT /api/v2/admin/request-settings")).toHaveLength(1));
    expect(calls("PUT /api/v2/admin/request-settings")[0]).toMatchObject({
      body: { global_max_requests: 0 },
    });
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
    expect(within(group("Sonarr")).getByText("HD default for series")).toBeInTheDocument();
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
      await within(dialog).findByText("Connected — 1 quality profile, 2 root folders"),
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

describe("Requests settings: routing", () => {
  it("leaves routing out when the server does not offer it", async () => {
    serve({
      handlers: {
        "GET /api/v2/admin/requests/capabilities": (options) =>
          reply(options, { available: true, guarded_configuration: true, routing: false }),
      },
    });
    mount();
    expect(await screen.findByRole("group", { name: "Servers" })).toBeInTheDocument();
    expect(screen.queryByRole("group", { name: "Movie routing" })).toBeNull();
    expect(screen.queryByRole("group", { name: "Series routing" })).toBeNull();
    expect(calls("GET /api/v2/admin/request-routes")).toHaveLength(0);
  });

  it("saves a never-saved default destination with the revision-zero validator it read", async () => {
    serve({
      handlers: {
        "PUT /api/v2/admin/request-routes/{id}": (options) =>
          reply(options, { ...fallback("movie", "radarr-1") }, '"fallback-movie-v2"'),
      },
    });
    mount();
    const movies = await screen.findByRole("group", { name: "Movie routing" });
    await within(movies).findByRole("combobox", { name: "HD server" });
    await choose(movies, "HD server", "Radarr");
    fireEvent.click(await screen.findByRole("button", { name: "Save" }));

    await waitFor(() => expect(calls("PUT /api/v2/admin/request-routes/{id}")).toHaveLength(1));
    expect(calls("PUT /api/v2/admin/request-routes/{id}")[0]).toEqual(
      expect.objectContaining({
        path: { id: "fallback-movie" },
        headers: { "If-Match": '"fallback-movie-v1"' },
        body: {
          name: "Everything else",
          enabled: true,
          conditions: {},
          hd: { integration_id: "radarr-1" },
          uhd: {},
          skip_uhd: false,
        },
      }),
    );
  });

  it("offers Add rule only once the default destination has an HD server", async () => {
    serve();
    mount();
    const movies = await screen.findByRole("group", { name: "Movie routing" });
    const addMovie = await within(movies).findByRole("button", { name: "Add rule" });
    expect((addMovie as HTMLButtonElement).disabled).toBe(true);
    expect(
      within(movies).getByText(
        "Choose the default server for movies before adding rules; titles no rule matches go there.",
      ),
    ).toBeInTheDocument();

    const series = group("Series routing");
    expect(
      (within(series).getByRole("button", { name: "Add rule" }) as HTMLButtonElement).disabled,
    ).toBe(false);
  });

  const anime = route({
    id: "anime-movie",
    name: "Anime",
    position: 0,
    conditions: {
      anime: true,
      original_languages: ["ja"],
      year_from: 1980,
      year_to: 1989,
      genre_ids: [16],
    },
    hd: { integration_id: "radarr-2", overrides: { root_folder: "/anime" } },
  });

  it("summarises a rule and round-trips its edits, including skipping 4K", async () => {
    serve({
      routes: [anime, fallback("movie", "radarr-1"), fallback("series", "sonarr-1")],
      handlers: {
        "PUT /api/v2/admin/request-routes/{id}": (options) =>
          reply(options, anime, '"anime-movie-v2"'),
      },
    });
    mount();
    const movies = await screen.findByRole("group", { name: "Movie routing" });
    expect(
      await within(movies).findByText("Anime · Japanese · 1980–1989 · Animation"),
    ).toBeInTheDocument();
    expect(
      within(movies).getByText("HD → Radarr Anime · /anime · 4K → default"),
    ).toBeInTheDocument();

    fireEvent.click(within(movies).getByRole("button", { name: "Edit Anime" }));
    const dialog = await screen.findByRole("dialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "Family" }));
    await choose(dialog, "Add a country", "Japan");
    await choose(dialog, "4K server", "No 4K copy");
    fireEvent.click(within(dialog).getByRole("button", { name: "Save rule" }));

    await waitFor(() => expect(calls("PUT /api/v2/admin/request-routes/{id}")).toHaveLength(1));
    const [write] = calls("PUT /api/v2/admin/request-routes/{id}");
    expect(write).toMatchObject({
      path: { id: "anime-movie" },
      headers: { "If-Match": '"anime-movie-v1"' },
    });
    expect(write!.body).toEqual({
      name: "Anime",
      enabled: true,
      conditions: {
        anime: true,
        genre_ids: [16, 10751],
        original_languages: ["ja"],
        origin_countries: ["JP"],
        year_from: 1980,
        year_to: 1989,
      },
      hd: { integration_id: "radarr-2", overrides: { root_folder: "/anime" } },
      uhd: {},
      skip_uhd: true,
    });
  });

  it("creates a rule from a decade and a server", async () => {
    serve({
      routes: [fallback("movie", "radarr-1"), fallback("series", "sonarr-1")],
      handlers: {
        "POST /api/v2/admin/request-routes": (options) =>
          reply(options, route({ id: "new" }), '"new-v1"'),
      },
    });
    mount();
    const movies = await screen.findByRole("group", { name: "Movie routing" });
    fireEvent.click(await within(movies).findByRole("button", { name: "Add rule" }));
    const dialog = await screen.findByRole("dialog");
    fireEvent.change(within(dialog).getByLabelText("Name"), { target: { value: "Eighties" } });
    fireEvent.click(within(dialog).getByRole("button", { name: "1980s" }));
    await choose(dialog, "HD server", "Radarr Anime");
    fireEvent.click(within(dialog).getByRole("button", { name: "Add rule" }));

    await waitFor(() => expect(calls("POST /api/v2/admin/request-routes")).toHaveLength(1));
    expect(calls("POST /api/v2/admin/request-routes")[0]!.body).toEqual({
      media_type: "movie",
      name: "Eighties",
      enabled: true,
      conditions: { year_from: 1980, year_to: 1989 },
      hd: { integration_id: "radarr-2" },
      uhd: {},
      skip_uhd: false,
    });
  });

  it("shows the server's field errors inline in the rule editor", async () => {
    serve({
      routes: [anime, fallback("movie", "radarr-1"), fallback("series", "sonarr-1")],
      handlers: {
        "PUT /api/v2/admin/request-routes/{id}": () =>
          Promise.reject(
            problem(422, "validation_failed", "The request did not pass validation; see errors.", [
              {
                location: "body.conditions.year_to",
                code: "invalid",
                detail: "The end year comes before the start year.",
              },
              {
                location: "body.hd.integration_id",
                code: "invalid",
                detail: "That server no longer exists.",
              },
            ]),
          ),
      },
    });
    mount();
    const movies = await screen.findByRole("group", { name: "Movie routing" });
    fireEvent.click(await within(movies).findByRole("button", { name: "Edit Anime" }));
    const dialog = await screen.findByRole("dialog");
    fireEvent.click(within(dialog).getByRole("button", { name: "Save rule" }));

    expect(
      await within(dialog).findByText("The end year comes before the start year."),
    ).toBeInTheDocument();
    expect(within(dialog).getByText("That server no longer exists.")).toBeInTheDocument();
    expect(
      within(dialog).getByText("The request did not pass validation; see errors."),
    ).toBeInTheDocument();
    expect(within(dialog).getByLabelText("To year").getAttribute("aria-invalid")).toBe("true");
  });

  it("reorders by sending every rule of the media type in the new order", async () => {
    serve({
      routes: [
        route({ id: "a", name: "First", position: 0, conditions: { anime: true } }),
        route({ id: "b", name: "Second", position: 1, conditions: { genre_ids: [27] } }),
        fallback("movie", "radarr-1"),
        fallback("series", "sonarr-1"),
      ],
      handlers: {
        "POST /api/v2/admin/request-routes/order": (options) => reply(options, { items: [] }),
      },
    });
    mount();
    const movies = await screen.findByRole("group", { name: "Movie routing" });
    fireEvent.click(await within(movies).findByRole("button", { name: "Move Second up" }));
    await waitFor(() => expect(calls("POST /api/v2/admin/request-routes/order")).toHaveLength(1));
    expect(calls("POST /api/v2/admin/request-routes/order")[0]!.body).toEqual({
      media_type: "movie",
      ids: ["b", "a"],
    });
  });

  it("previews where a title would go, tier by tier", async () => {
    serve({
      routes: [anime, fallback("movie", "radarr-1"), fallback("series", "sonarr-1")],
      handlers: {
        "GET /api/v2/requests/search": (options) =>
          reply(options, {
            page: 1,
            total_pages: 1,
            total_results: 1,
            results: [
              {
                media_type: "movie",
                tmdb_id: 129,
                title: "Spirited Away",
                year: 2001,
                availability: "none",
                request: {},
              },
            ],
          }),
        "POST /api/v2/admin/request-routes/preview": (options) =>
          reply(options, {
            facts: {
              anime: true,
              genre_ids: [16, 14],
              keyword_ids: [],
              original_language: "ja",
              origin_countries: ["JP"],
              year: 2001,
              network_ids: [],
              company_ids: [],
            },
            tiers: [
              {
                quality: "1080p",
                route_id: "anime-movie",
                route_name: "Anime",
                integration_id: "radarr-2",
                integration_name: "Radarr Anime",
                overrides: { root_folder: "/anime", quality_profile_id: 1, tags: [2] },
              },
              { quality: "2160p", note: "No rule sends 4K for this title." },
            ],
          }),
      },
    });
    mount();
    const movies = await screen.findByRole("group", { name: "Movie routing" });
    fireEvent.change(await within(movies).findByLabelText("Test a title"), {
      target: { value: "Spirited" },
    });
    fireEvent.click(await within(movies).findByRole("button", { name: /Spirited Away/ }));

    expect(await within(movies).findByText("Radarr Anime")).toBeInTheDocument();
    expect(calls("POST /api/v2/admin/request-routes/preview")[0]!.body).toEqual({
      media_type: "movie",
      tmdb_id: 129,
    });
    expect(
      within(movies).getByText("Anime · Animation, Fantasy · Japanese · Japan · 2001"),
    ).toBeInTheDocument();
    expect(within(movies).getByText("· by Anime")).toBeInTheDocument();
    // Quality profiles and tags are named from the server's options; the root
    // folder keeps its bare path.
    expect(
      await within(movies).findByText("Root folder /anime · Quality profile HD-1080p · Tags anime"),
    ).toBeInTheDocument();
    expect(within(movies).getByText("No rule sends 4K for this title.")).toBeInTheDocument();
  });

  it("previews by TMDB ID while requests are off, without the search", async () => {
    serve({
      handlers: {
        "GET /api/v2/admin/request-settings": (options) =>
          reply(options, { ...settings, requests_enabled: false }),
        "POST /api/v2/admin/request-routes/preview": (options) =>
          reply(options, {
            facts: {
              anime: false,
              genre_ids: [],
              keyword_ids: [],
              origin_countries: [],
              network_ids: [],
              company_ids: [],
            },
            tiers: [{ quality: "1080p", note: "No rule sends HD for this title." }],
          }),
      },
    });
    mount();
    const movies = await screen.findByRole("group", { name: "Movie routing" });
    expect(within(movies).queryByRole("searchbox")).toBeNull();
    fireEvent.change(await within(movies).findByLabelText("Movie TMDB ID"), {
      target: { value: "129" },
    });
    fireEvent.click(within(movies).getByRole("button", { name: "Check" }));

    expect(await within(movies).findByText("No rule sends HD for this title.")).toBeInTheDocument();
    expect(calls("POST /api/v2/admin/request-routes/preview")[0]!.body).toEqual({
      media_type: "movie",
      tmdb_id: 129,
    });
    expect(calls("GET /api/v2/requests/search")).toHaveLength(0);
  });

  it("offers the TMDB ID when the search fails", async () => {
    serve({
      handlers: {
        "GET /api/v2/requests/search": () =>
          Promise.reject(problem(503, "capability_disabled", "Requests are turned off")),
      },
    });
    mount();
    const movies = await screen.findByRole("group", { name: "Movie routing" });
    fireEvent.change(await within(movies).findByLabelText("Test a title"), {
      target: { value: "Spirited" },
    });
    expect(await within(movies).findByText("Requests are turned off")).toBeInTheDocument();
    expect(within(movies).getByLabelText("Movie TMDB ID")).toBeInTheDocument();
  });
});

/** A promise the test settles when it chooses. */
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((r) => {
    resolve = r;
  });
  return { promise, resolve };
}

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

  it("keeps a saved default destination and saves the next edit against its new validator", async () => {
    let routes = [fallback("movie"), fallback("series", "sonarr-1")];
    const versions: Record<string, number> = {};
    serve({
      handlers: {
        "GET /api/v2/admin/request-routes": (options) => reply(options, { items: routes }),
        "GET /api/v2/admin/request-routes/{id}": (options) => {
          const id = options.path!.id!;
          return reply(
            options,
            routes.find((r) => r.id === id),
            `"${id}-v${versions[id] ?? 0}"`,
          );
        },
        "PUT /api/v2/admin/request-routes/{id}": (options) => {
          const id = options.path!.id!;
          const body = options.body as Pick<Route, "hd" | "uhd">;
          routes = routes.map((r) => (r.id === id ? { ...r, hd: body.hd, uhd: body.uhd } : r));
          versions[id] = (versions[id] ?? 0) + 1;
          return reply(
            options,
            routes.find((r) => r.id === id),
            `"${id}-v${versions[id]}"`,
          );
        },
      },
    });
    mount();
    const movies = await screen.findByRole("group", { name: "Movie routing" });
    await choose(movies, "HD server", "Radarr");
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(screen.queryByRole("button", { name: "Save" })).toBeNull());
    expect(within(movies).getByRole("combobox", { name: "HD server" })).toHaveTextContent("Radarr");
    expect(
      (within(movies).getByRole("button", { name: "Add rule" }) as HTMLButtonElement).disabled,
    ).toBe(false);

    await choose(movies, "4K server", "Radarr Anime");
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(calls("PUT /api/v2/admin/request-routes/{id}")).toHaveLength(2));
    expect(calls("PUT /api/v2/admin/request-routes/{id}")[1]).toMatchObject({
      headers: { "If-Match": '"fallback-movie-v1"' },
      body: { hd: { integration_id: "radarr-1" }, uhd: { integration_id: "radarr-2" } },
    });
  });

  it("holds the rule list until the reorder has been read back", async () => {
    let routes = [
      route({ id: "a", name: "First", position: 0, conditions: { anime: true } }),
      route({ id: "b", name: "Second", position: 1, conditions: { genre_ids: [27] } }),
      fallback("movie", "radarr-1"),
      fallback("series", "sonarr-1"),
    ];
    let hold: ReturnType<typeof deferred<void>> | null = null;
    serve({
      handlers: {
        "GET /api/v2/admin/request-routes": async (options) => {
          if (hold) await hold.promise;
          return reply(options, { items: routes });
        },
        "GET /api/v2/admin/request-routes/{id}": (options) =>
          reply(
            options,
            routes.find((r) => r.id === options.path?.id),
            '"v"',
          ),
        "POST /api/v2/admin/request-routes/order": (options) => {
          const ids = (options.body as { ids: string[] }).ids;
          routes = routes.map((r) =>
            ids.includes(r.id) ? { ...r, position: ids.indexOf(r.id) } : r,
          );
          hold = deferred<void>();
          return reply(options, { items: [] });
        },
      },
    });
    mount();
    const movies = await screen.findByRole("group", { name: "Movie routing" });
    fireEvent.click(await within(movies).findByRole("button", { name: "Move Second up" }));
    await waitFor(() => expect(calls("POST /api/v2/admin/request-routes/order")).toHaveLength(1));

    // The old order is still on screen, so no second move may be made from it.
    const firstDown = within(movies).getByRole("button", { name: "Move First down" });
    await waitFor(() => expect((firstDown as HTMLButtonElement).disabled).toBe(true));
    expect(
      (within(movies).getByRole("switch", { name: "First enabled" }) as HTMLButtonElement).disabled,
    ).toBe(true);

    await act(async () => hold!.resolve());
    await waitFor(() =>
      expect(
        (within(movies).getByRole("button", { name: "Move First up" }) as HTMLButtonElement)
          .disabled,
      ).toBe(false),
    );
    const names = within(within(movies).getByRole("list", { name: "Movie routing rules" }))
      .getAllByRole("button", { name: /^Edit / })
      .map((button) => button.getAttribute("aria-label"));
    expect(names).toEqual(["Edit Second", "Edit First"]);
  });

  it("says why a rule toggle was refused", async () => {
    serve({
      routes: [
        route({ id: "a", name: "First", conditions: { anime: true } }),
        fallback("movie", "radarr-1"),
        fallback("series", "sonarr-1"),
      ],
      handlers: {
        "PUT /api/v2/admin/request-routes/{id}": () =>
          Promise.reject(
            problem(422, "validation_failed", "The request did not pass validation; see errors.", [
              {
                location: "body.hd.integration_id",
                code: "invalid",
                detail: "That server no longer exists.",
              },
            ]),
          ),
      },
    });
    mount();
    const movies = await screen.findByRole("group", { name: "Movie routing" });
    fireEvent.click(await within(movies).findByRole("switch", { name: "First enabled" }));
    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(
        "The request did not pass validation; see errors. That server no longer exists.",
      ),
    );
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

  it("clears a default destination's save error when that tier is edited", async () => {
    serve({
      handlers: {
        "PUT /api/v2/admin/request-routes/{id}": () =>
          Promise.reject(
            problem(422, "validation_failed", "The request did not pass validation; see errors.", [
              {
                location: "body.hd.integration_id",
                code: "invalid",
                detail: "Radarr is a sonarr server; movies go to radarr.",
              },
            ]),
          ),
      },
    });
    mount();
    const movies = await screen.findByRole("group", { name: "Movie routing" });
    await choose(movies, "HD server", "Radarr");
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(
      await within(movies).findByText("Radarr is a sonarr server; movies go to radarr."),
    ).toBeInTheDocument();

    await choose(movies, "HD server", "Radarr Anime");
    expect(
      within(movies).queryByText("Radarr is a sonarr server; movies go to radarr."),
    ).toBeNull();
  });

  it("says so when reloading the default destination fails", async () => {
    let listReads = 0;
    serve({
      handlers: {
        "GET /api/v2/admin/request-routes": (options) => {
          listReads += 1;
          return listReads === 1
            ? reply(options, { items: [fallback("movie"), fallback("series", "sonarr-1")] })
            : Promise.reject(new Error("offline"));
        },
        "PUT /api/v2/admin/request-routes/{id}": () => Promise.reject(conflict()),
      },
    });
    mount();
    const movies = await screen.findByRole("group", { name: "Movie routing" });
    await choose(movies, "HD server", "Radarr");
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    fireEvent.click(await within(movies).findByRole("button", { name: "Reload latest version" }));
    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith("Couldn't reload the default destination: offline"),
    );
  });
});

describe("Requests settings: with requests off, and override sections", () => {
  const studioRule = route({
    id: "ghibli",
    name: "Ghibli",
    conditions: { company_ids: [10342] },
    hd: { integration_id: "radarr-2" },
  });

  it("does not read the studio list while requests are off, and keeps IDs on a rule", async () => {
    serve({
      routes: [studioRule, fallback("movie", "radarr-1"), fallback("series", "sonarr-1")],
      handlers: {
        "GET /api/v2/admin/request-settings": (options) =>
          reply(options, { ...settings, requests_enabled: false }),
      },
    });
    mount();
    const movies = await screen.findByRole("group", { name: "Movie routing" });
    expect(await within(movies).findByText("Studio 10342")).toBeInTheDocument();
    fireEvent.click(within(movies).getByRole("button", { name: "Edit Ghibli" }));
    const dialog = await screen.findByRole("dialog");

    expect(
      within(dialog).getByText("Turn on requests to pick networks and studios."),
    ).toBeInTheDocument();
    expect(within(dialog).queryByRole("combobox", { name: "Add a studio" })).toBeNull();
    expect(within(dialog).getByText("TMDB 10342")).toBeInTheDocument();
    expect(within(dialog).getByLabelText("Studio TMDB ID")).toBeInTheDocument();
    expect(calls("GET /api/v2/requests/discover/studios")).toHaveLength(0);
    expect(calls("GET /api/v2/requests/discover/networks")).toHaveLength(0);
  });

  it("gives each override section its own open state", async () => {
    serve({
      routes: [
        { ...fallback("movie", "radarr-1"), uhd: { integration_id: "radarr-2" } },
        fallback("series", "sonarr-1"),
      ],
    });
    mount();
    const movies = await screen.findByRole("group", { name: "Movie routing" });
    const hd = await within(movies).findByRole("button", { name: /^Override HD server settings/ });
    const uhd = within(movies).getByRole("button", { name: /^Override 4K server settings/ });
    fireEvent.click(hd);
    expect(hd).toHaveAttribute("aria-expanded", "true");
    expect(uhd).toHaveAttribute("aria-expanded", "false");

    // A section that mounts later (a rule editor) starts closed too.
    fireEvent.click(within(movies).getByRole("button", { name: "Add rule" }));
    const dialog = await screen.findByRole("dialog");
    await choose(dialog, "HD server", "Radarr Anime");
    expect(
      within(dialog).getByRole("button", { name: /^Override HD server settings/ }),
    ).toHaveAttribute("aria-expanded", "false");
  });
});

describe("Requests settings: server delete and kind", () => {
  it("does not offer Delete for a server routing still sends to, and says which routes", async () => {
    serve();
    const dialog = await openServer("Sonarr");
    const del = within(dialog).getByRole("button", { name: "Delete" }) as HTMLButtonElement;
    expect(del.disabled).toBe(true);
    expect(
      within(dialog).getByText(
        "Routing sends requests here (HD default for series). Change that first to delete it.",
      ),
    ).toBeInTheDocument();
  });

  it("does not let a routed server change type, and says why", async () => {
    serve();
    const dialog = await openServer("Sonarr");
    const service = within(dialog).getByRole("combobox", { name: "Service" });
    expect(service).toBeDisabled();
    const reason = within(dialog).getByText(
      "Routing sends requests here (HD default for series). Change that first to switch the type.",
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
