import { act, renderHook } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";

import { createElement, type ReactNode } from "react";
import { MemoryRouter, useNavigate } from "react-router";

import { useAdminNavTopics, useCloseAdminNavTopicsOutsideAdmin } from "./useAdminNavTopics";

const KEY = "silo.admin-nav.open-topics";

afterEach(() => {
  sessionStorage.clear();
  localStorage.clear();
});

// The desktop sidebar and the phone drawer each run the hook at once.
it("keeps a choice made in one sidebar when the other toggles a different topic", () => {
  const desktop = renderHook(() => useAdminNavTopics(null));
  const drawer = renderHook(() => useAdminNavTopics(null));

  act(() => drawer.result.current.toggle("playback"));
  act(() => desktop.result.current.toggle("server"));

  expect(JSON.parse(sessionStorage.getItem(KEY) ?? "{}")).toEqual({ playback: true, server: true });
});

it("opens the topic holding the current page and remembers other toggles", () => {
  const { result } = renderHook(() => useAdminNavTopics("server"));
  expect(result.current.isOpen("server")).toBe(true);
  expect(result.current.isOpen("playback")).toBe(false);

  act(() => result.current.toggle("playback"));
  expect(result.current.isOpen("playback")).toBe(true);
  expect(JSON.parse(sessionStorage.getItem(KEY) ?? "{}")).toEqual({ playback: true });
});

function renderAt(path: string) {
  return renderHook(
    () => {
      useCloseAdminNavTopicsOutsideAdmin();
      return useNavigate();
    },
    {
      wrapper: ({ children }: { children: ReactNode }) =>
        createElement(MemoryRouter, { initialEntries: [path] }, children),
    },
  );
}

it("keeps the open topics while the admin moves between admin pages", () => {
  sessionStorage.setItem(KEY, JSON.stringify({ server: true }));
  const app = renderAt("/admin/playback");

  act(() => void app.result.current("/admin/libraries"));
  act(() => void app.result.current("/admin"));

  expect(JSON.parse(sessionStorage.getItem(KEY) ?? "{}")).toEqual({ server: true });
});

it("closes every topic once the app leaves the admin area", () => {
  sessionStorage.setItem(KEY, JSON.stringify({ server: true }));
  localStorage.setItem(KEY, JSON.stringify({ logs: true }));
  const app = renderAt("/admin/playback");

  act(() => void app.result.current("/"));

  expect(sessionStorage.getItem(KEY)).toBeNull();
  expect(localStorage.getItem(KEY)).toBeNull();
  const { result } = renderHook(() => useAdminNavTopics(null));
  expect(result.current.isOpen("server")).toBe(false);
});

it("does not treat a path that only starts with admin as the admin area", () => {
  sessionStorage.setItem(KEY, JSON.stringify({ server: true }));
  renderAt("/administrators");
  expect(sessionStorage.getItem(KEY)).toBeNull();
});
