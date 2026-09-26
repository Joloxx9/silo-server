import { useId, type ReactNode } from "react";
import { X } from "lucide-react";

import type { PluginAdminFormField, RequestIntegration } from "@/api/types";
import { coerceFieldValue, parseFieldTypes } from "@/components/admin/plugins/schemaFormUtils";
import { AdvancedSection } from "@/components/settings/AdvancedSection";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useRequestIntegrationOptions } from "@/hooks/queries/useRequests";
import { cn } from "@/lib/utils";

import {
  overrideLabel,
  serverOverrideFields,
  splitOverrideFields,
  type DestinationChoice,
  type Tier,
} from "./requestRoutingModel";
import {
  serverConfigSchema,
  serverInstallation,
  type RequestRouterInstallation,
} from "./requestServerModel";
import { SETTINGS_CONTROL_WIDTH, SettingFieldRow } from "./SettingField";

/** Select values for the choices that are not a server. */
const DEST_PASS = "__pass__";
const DEST_SKIP = "__skip__";
const SERVER_SETTING = "__server__";

export interface Choice {
  value: string;
  label: string;
}

/** An inline error under a field, announced when it appears. */
export function FieldError({ children }: { children?: string }) {
  if (!children) return null;
  return (
    <p role="alert" className="text-destructive text-xs">
      {children}
    </p>
  );
}

/**
 * A row of toggle chips for a short, fixed list (a server's tags): every
 * choice visible, each one a pressed or unpressed button.
 */
function ChipToggleList({
  label,
  options,
  selected,
  onChange,
  className,
}: {
  label: string;
  options: readonly Choice[];
  selected: readonly string[];
  onChange: (next: string[]) => void;
  className?: string;
}) {
  return (
    <div role="group" aria-label={label} className={cn("flex flex-wrap gap-1.5", className)}>
      {options.map((option) => {
        const on = selected.includes(option.value);
        return (
          <Button
            key={option.value}
            type="button"
            size="xs"
            variant={on ? "default" : "outline"}
            aria-pressed={on}
            onClick={() =>
              onChange(
                on
                  ? selected.filter((value) => value !== option.value)
                  : [...selected, option.value],
              )
            }
          >
            {option.label}
          </Button>
        );
      })}
    </div>
  );
}

/**
 * The chosen values of a list (genres, languages, accounts) as removable
 * chips, and one select to add another.
 */
export function ValuePicker({
  addLabel,
  options,
  selected,
  onChange,
  labelOf,
  disabled,
  unavailableHint,
  hideAdd = false,
}: {
  addLabel: string;
  options: readonly Choice[];
  selected: readonly string[];
  onChange: (next: string[]) => void;
  labelOf?: (value: string) => string;
  disabled?: boolean;
  /** Why there is nothing to pick from; shown in place of the select. Chosen values stay. */
  unavailableHint?: string;
  /** Shows only the chips; the caller offers its own way to add. */
  hideAdd?: boolean;
}) {
  const nameOf = (value: string) =>
    labelOf?.(value) ?? options.find((option) => option.value === value)?.label ?? value;
  const available = options.filter((option) => !selected.includes(option.value));
  return (
    <div className="flex min-w-0 flex-wrap items-center gap-1.5">
      {selected.length > 0 ? (
        <ul className="contents list-none">
          {selected.map((value) => (
            <li
              key={value}
              className="border-border bg-accent/60 inline-flex items-center gap-1 rounded-md border py-0.5 pr-0.5 pl-2 text-xs"
            >
              {nameOf(value)}
              <Button
                type="button"
                size="icon-xs"
                variant="ghost"
                aria-label={`Remove ${nameOf(value)}`}
                onClick={() => onChange(selected.filter((entry) => entry !== value))}
              >
                <X />
              </Button>
            </li>
          ))}
        </ul>
      ) : null}
      {hideAdd ? null : unavailableHint ? (
        <p className="text-muted-foreground text-xs">{unavailableHint}</p>
      ) : (
        <Select
          value=""
          onValueChange={(value) => {
            if (value) onChange([...selected, value]);
          }}
          disabled={disabled || available.length === 0}
        >
          <SelectTrigger aria-label={addLabel} size="sm" className="w-auto min-w-32 text-xs">
            <SelectValue placeholder={addLabel} />
          </SelectTrigger>
          <SelectContent>
            {available.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}
    </div>
  );
}

function overrideValue(value: unknown): string {
  return value === undefined || value === null ? "" : String(value);
}

/**
 * One server setting a route may replace. Unset means the server's own
 * setting applies, and the first choice says what that is.
 */
function OverrideFieldRow({
  field,
  label,
  server,
  options,
  optionsLoading,
  value,
  onChange,
  error,
}: {
  field: PluginAdminFormField;
  label: string;
  server: RequestIntegration;
  options: readonly Choice[];
  optionsLoading: boolean;
  value: unknown;
  onChange: (raw: unknown) => void;
  error?: string;
}) {
  const controlId = useId();
  const serverValue = server.plugin_config?.[field.key] ?? field.default_value;

  if (field.control === "MULTI_SELECT") {
    const selected = Array.isArray(value) ? value.map((entry) => String(entry)) : [];
    return (
      <SettingFieldRow
        label={label}
        description={
          selected.length === 0
            ? `None chosen — the server's ${label.toLowerCase()} apply.`
            : undefined
        }
        status={<FieldError>{error}</FieldError>}
      >
        {optionsLoading && options.length === 0 ? (
          <span className="text-muted-foreground text-xs">Loading…</span>
        ) : options.length === 0 ? (
          <span className="text-muted-foreground text-xs">Nothing to choose on this server.</span>
        ) : (
          <ChipToggleList
            label={label}
            options={options}
            selected={selected}
            onChange={onChange}
            className="sm:max-w-[var(--settings-control-w)] sm:justify-end"
          />
        )}
      </SettingFieldRow>
    );
  }

  const choices: Choice[] =
    field.control === "SWITCH"
      ? [
          { value: "true", label: "On" },
          { value: "false", label: "Off" },
        ]
      : [...options];
  const current = overrideValue(value);
  if (current && !choices.some((choice) => choice.value === current)) {
    choices.push({ value: current, label: current });
  }
  const serverLabel =
    field.control === "SWITCH"
      ? serverValue === true || serverValue === "true"
        ? "On"
        : "Off"
      : (choices.find((choice) => choice.value === overrideValue(serverValue))?.label ??
        overrideValue(serverValue));

  return (
    <SettingFieldRow label={label} htmlFor={controlId} status={<FieldError>{error}</FieldError>}>
      <Select
        value={current || SERVER_SETTING}
        onValueChange={(next) => onChange(next === SERVER_SETTING ? undefined : next)}
      >
        <SelectTrigger
          id={controlId}
          className={cn(SETTINGS_CONTROL_WIDTH, "min-w-0")}
          aria-invalid={Boolean(error)}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={SERVER_SETTING}>
            {serverLabel ? `Server's setting (${serverLabel})` : "Server's setting"}
          </SelectItem>
          {choices.map((choice) => (
            <SelectItem key={choice.value} value={choice.value}>
              {choice.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </SettingFieldRow>
  );
}

/**
 * The settings a destination replaces on its server: Folder, Quality and
 * Tags on their own rows, and every other field the plugin offers under
 * More settings, which opens when one of them is set.
 */
function DestinationOverrides({
  sectionId,
  server,
  installations,
  overrides,
  onChange,
  errors,
  errorPrefix,
}: {
  sectionId: string;
  server: RequestIntegration;
  installations: RequestRouterInstallation[];
  overrides: Record<string, unknown>;
  onChange: (overrides: Record<string, unknown>) => void;
  errors: Record<string, string>;
  errorPrefix: string;
}) {
  const entry = serverInstallation(installations, server.installation_id, server.capability_id);
  const fieldTypes = parseFieldTypes(serverConfigSchema(entry).jsonSchema);
  const fields = serverOverrideFields(server, installations);
  const { inline, more } = splitOverrideFields(fields);
  const needsOptions = fields.some((field) => field.dynamic_options);
  const options = useRequestIntegrationOptions(needsOptions ? server.id : undefined);

  if (fields.length === 0) return null;

  function set(field: PluginAdminFormField, raw: unknown) {
    const next = { ...overrides };
    if (raw === undefined || (Array.isArray(raw) && raw.length === 0)) {
      delete next[field.key];
    } else {
      next[field.key] = coerceFieldValue(field, raw, fieldTypes[field.key]);
    }
    onChange(next);
  }

  const row = (field: PluginAdminFormField, label: string) => (
    <OverrideFieldRow
      key={field.key}
      field={field}
      label={label}
      server={server}
      options={field.dynamic_options ? (options.data?.[field.key] ?? []) : (field.options ?? [])}
      optionsLoading={Boolean(field.dynamic_options) && options.isLoading}
      value={overrides[field.key]}
      onChange={(raw) => set(field, raw)}
      error={errors[`${errorPrefix}.overrides.${field.key}`]}
    />
  );
  const moreSet = more.some((field) => overrides[field.key] !== undefined);
  const moreErrors = more.some((field) => errors[`${errorPrefix}.overrides.${field.key}`]);

  return (
    <div className="border-border/60 ml-1 border-l pl-4">
      {options.isError ? (
        <p className="settings-field-note py-3 text-xs text-amber-600 dark:text-amber-400">
          Couldn&apos;t read folders and profiles from {server.name}:{" "}
          {options.error instanceof Error ? options.error.message : "unknown error"}
        </p>
      ) : null}
      {inline.map((field) => row(field, overrideLabel(field.key)))}
      {more.length > 0 ? (
        <AdvancedSection
          id={`requests.route-more.${sectionId}`}
          title="More settings"
          count={more.length}
          forceOpen={moreSet || moreErrors}
        >
          {more.map((field) => row(field, field.label || overrideLabel(field.key)))}
        </AdvancedSection>
      ) : null}
    </div>
  );
}

/**
 * Where one copy (HD or 4K) goes: a server, "Don't make a 4K copy", or the
 * pass-through choice, and under a chosen server the settings the route
 * replaces on it. `errors` holds the editor's field errors keyed like the
 * API's (`hd.integration_id`, `uhd.overrides.root_folder`, ...).
 */
export function RouteDestinationEditor({
  tier,
  sectionId,
  servers,
  allServers,
  installations,
  value,
  onChange,
  passLabel,
  allowSkip = false,
  caption,
  note,
  errors,
}: {
  tier: Tier;
  /**
   * Names this destination's More settings, e.g. `fallback-movie.hd`, so each
   * keeps its own open state.
   */
  sectionId: string;
  /** The servers this copy may go to (the media type's kind). */
  servers: readonly RequestIntegration[];
  /** Every server, to name one that no longer fits the media type. */
  allServers: readonly RequestIntegration[];
  installations: RequestRouterInstallation[];
  value: DestinationChoice;
  onChange: (next: DestinationChoice) => void;
  /** Offers passing the copy on, labelled so; without it a server is required. */
  passLabel?: string;
  /** Offers "Don't make a 4K copy". */
  allowSkip?: boolean;
  caption?: ReactNode;
  /** A line under the chosen server, e.g. what a preset set. */
  note?: ReactNode;
  errors: Record<string, string>;
}) {
  const controlId = useId();
  const { dest, skip } = value;
  const server = dest.integration_id
    ? allServers.find((candidate) => candidate.id === dest.integration_id)
    : undefined;
  const serverChoices: Choice[] = servers.map((candidate) => ({
    value: candidate.id,
    label: candidate.enabled ? candidate.name : `${candidate.name} (turned off)`,
  }));
  if (
    dest.integration_id &&
    !serverChoices.some((choice) => choice.value === dest.integration_id)
  ) {
    serverChoices.push({ value: dest.integration_id, label: server?.name ?? "Missing server" });
  }
  const selected = skip ? DEST_SKIP : dest.integration_id || (passLabel ? DEST_PASS : "");
  const error = errors[tier] ?? errors[`${tier}.integration_id`];
  const label = tier === "hd" ? "HD copies" : "4K copies";

  function select(next: string) {
    if (next === DEST_SKIP) onChange({ dest: { integration_id: "", overrides: {} }, skip: true });
    else if (next === DEST_PASS)
      onChange({ dest: { integration_id: "", overrides: {} }, skip: false });
    else if (next === dest.integration_id) onChange({ dest, skip: false });
    else onChange({ dest: { integration_id: next, overrides: {} }, skip: false });
  }

  return (
    <>
      <SettingFieldRow
        label={label}
        htmlFor={controlId}
        description={caption}
        status={<FieldError>{error}</FieldError>}
      >
        {server ? (
          <span className="text-muted-foreground shrink-0 text-xs" aria-hidden="true">
            Send to
          </span>
        ) : null}
        <Select value={selected} onValueChange={select}>
          <SelectTrigger
            id={controlId}
            className={cn(SETTINGS_CONTROL_WIDTH, "min-w-0")}
            aria-invalid={Boolean(error)}
          >
            <SelectValue placeholder="Choose a server" />
          </SelectTrigger>
          <SelectContent>
            {serverChoices.map((choice) => (
              <SelectItem key={choice.value} value={choice.value}>
                {choice.label}
              </SelectItem>
            ))}
            {allowSkip ? (
              <SelectItem value={DEST_SKIP}>Don&apos;t make a 4K copy</SelectItem>
            ) : null}
            {passLabel ? <SelectItem value={DEST_PASS}>{passLabel}</SelectItem> : null}
          </SelectContent>
        </Select>
      </SettingFieldRow>
      {server ? (
        <DestinationOverrides
          key={server.id}
          sectionId={sectionId}
          server={server}
          installations={installations}
          overrides={dest.overrides}
          onChange={(overrides) => onChange({ dest: { ...dest, overrides }, skip: false })}
          errors={errors}
          errorPrefix={tier}
        />
      ) : null}
      {server && note ? (
        <p className="text-muted-foreground border-border/60 ml-1 border-l py-2 pl-4 text-xs">
          {note}
        </p>
      ) : null}
    </>
  );
}
