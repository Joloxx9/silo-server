import { useId } from "react";
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

import { overrideFields, type RouteDestinationDraft } from "./requestRoutingModel";
import {
  serverConfigSchema,
  serverInstallation,
  type RequestRouterInstallation,
} from "./requestServerModel";
import { SETTINGS_CONTROL_WIDTH, SettingFieldRow } from "./SettingField";

/** Select values for the choices that are not a server. */
export const DEST_NONE = "__none__";
export const DEST_NEXT = "__next__";
export const DEST_SKIP = "__skip__";
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
 * A row of toggle chips for a short, fixed list (genres, a server's tags):
 * every choice visible, each one a pressed or unpressed button.
 */
export function ChipToggleList({
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
 * The chosen values of a long list (languages, countries, accounts) as
 * removable chips, and one select to add another.
 */
export function ValuePicker({
  addLabel,
  options,
  selected,
  onChange,
  labelOf,
  disabled,
  unavailableHint,
}: {
  addLabel: string;
  options: readonly Choice[];
  selected: readonly string[];
  onChange: (next: string[]) => void;
  labelOf?: (value: string) => string;
  disabled?: boolean;
  /** Why there is nothing to pick from; shown in place of the select. Chosen values stay. */
  unavailableHint?: string;
}) {
  const nameOf = (value: string) =>
    labelOf?.(value) ?? options.find((option) => option.value === value)?.label ?? value;
  const available = options.filter((option) => !selected.includes(option.value));
  return (
    <div className="flex flex-col gap-2">
      {selected.length > 0 ? (
        <ul className="flex list-none flex-wrap gap-1.5">
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
      {unavailableHint ? (
        <p className="text-muted-foreground text-xs">{unavailableHint}</p>
      ) : (
        <Select
          value=""
          onValueChange={(value) => {
            if (value) onChange([...selected, value]);
          }}
          disabled={disabled || available.length === 0}
        >
          <SelectTrigger aria-label={addLabel} className="w-full sm:w-64">
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
  server,
  options,
  optionsLoading,
  value,
  onChange,
  error,
}: {
  field: PluginAdminFormField;
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
        label={field.label || field.key}
        description={
          selected.length === 0 ? "None chosen, so the server's own setting applies." : undefined
        }
        status={<FieldError>{error}</FieldError>}
      >
        {optionsLoading && options.length === 0 ? (
          <span className="text-muted-foreground text-xs">Loading…</span>
        ) : options.length === 0 ? (
          <span className="text-muted-foreground text-xs">Nothing to choose on this server.</span>
        ) : (
          <ChipToggleList
            label={field.label || field.key}
            options={options}
            selected={selected}
            onChange={onChange}
            className="justify-end sm:max-w-[var(--settings-control-w)]"
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
    <SettingFieldRow
      label={field.label || field.key}
      htmlFor={controlId}
      status={<FieldError>{error}</FieldError>}
    >
      <Select
        value={current || SERVER_SETTING}
        onValueChange={(next) => onChange(next === SERVER_SETTING ? undefined : next)}
      >
        <SelectTrigger id={controlId} className={SETTINGS_CONTROL_WIDTH}>
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
 * The server settings a route can replace for one tier (root folder, quality
 * profile, tags, ...), offered from the server's own form and filled from the
 * server's options. Collapsed until the route overrides something.
 */
function RouteOverrideFields({
  sectionId,
  server,
  installations,
  tierLabel,
  overrides,
  onChange,
  errors,
  errorPrefix,
}: {
  sectionId: string;
  server: RequestIntegration;
  installations: RequestRouterInstallation[];
  tierLabel: string;
  overrides: Record<string, unknown>;
  onChange: (overrides: Record<string, unknown>) => void;
  errors: Record<string, string>;
  errorPrefix: string;
}) {
  const entry = serverInstallation(installations, server.installation_id, server.capability_id);
  const { descriptor, jsonSchema } = serverConfigSchema(entry);
  const fields = overrideFields(descriptor, server.plugin_config ?? {});
  const fieldTypes = parseFieldTypes(jsonSchema);
  const needsOptions = fields.some((field) => field.dynamic_options);
  const options = useRequestIntegrationOptions(needsOptions ? server.id : undefined);

  if (fields.length === 0) return null;

  const overrideErrors = Object.keys(errors).some((key) =>
    key.startsWith(`${errorPrefix}.overrides.`),
  );
  const overridden = Object.values(overrides).some((value) => value !== undefined);

  function set(field: PluginAdminFormField, raw: unknown) {
    const next = { ...overrides };
    if (raw === undefined || (Array.isArray(raw) && raw.length === 0)) {
      delete next[field.key];
    } else {
      next[field.key] = coerceFieldValue(field, raw, fieldTypes[field.key]);
    }
    onChange(next);
  }

  return (
    <AdvancedSection
      id={`requests.route-overrides.${sectionId}`}
      title={`Override ${tierLabel} server settings`}
      count={fields.length}
      forceOpen={overridden || overrideErrors}
    >
      {options.isError ? (
        <p className="settings-field-note py-3 text-xs text-amber-600 dark:text-amber-400">
          Couldn&apos;t read root folders and profiles from {server.name}:{" "}
          {options.error instanceof Error ? options.error.message : "unknown error"}
        </p>
      ) : null}
      {fields.map((field) => (
        <OverrideFieldRow
          key={field.key}
          field={field}
          server={server}
          options={
            field.dynamic_options ? (options.data?.[field.key] ?? []) : (field.options ?? [])
          }
          optionsLoading={Boolean(field.dynamic_options) && options.isLoading}
          value={overrides[field.key]}
          onChange={(raw) => set(field, raw)}
          error={errors[`${errorPrefix}.overrides.${field.key}`]}
        />
      ))}
    </AdvancedSection>
  );
}

/**
 * Where a route sends one quality tier: a server select, with the choices
 * that are not a server first, and the overrides for the chosen server.
 * `errors` holds the route editor's field errors keyed like the API's
 * (`hd.integration_id`, `uhd.overrides.root_folder`, ...).
 */
export function RouteDestinationFields({
  sectionId,
  label,
  tierLabel,
  servers,
  allServers,
  installations,
  value,
  choices,
  selected,
  onSelect,
  onOverridesChange,
  errors,
  errorPrefix,
  dirty,
}: {
  label: string;
  tierLabel: string;
  /** The servers this tier may go to (the media type's kind). */
  servers: readonly RequestIntegration[];
  /** Every server, to name one that no longer fits the media type. */
  allServers: readonly RequestIntegration[];
  installations: RequestRouterInstallation[];
  value: RouteDestinationDraft;
  choices: readonly Choice[];
  selected: string;
  onSelect: (value: string) => void;
  onOverridesChange: (overrides: Record<string, unknown>) => void;
  errors: Record<string, string>;
  errorPrefix: string;
  dirty?: boolean;
  /**
   * Names this tier's override section, e.g. `fallback-movie.hd`, so each
   * section keeps its own open state instead of opening every other one.
   */
  sectionId: string;
}) {
  const controlId = useId();
  const server = value.integration_id
    ? allServers.find((candidate) => candidate.id === value.integration_id)
    : undefined;
  const serverChoices: Choice[] = servers.map((candidate) => ({
    value: candidate.id,
    label: candidate.enabled ? candidate.name : `${candidate.name} (disabled)`,
  }));
  if (
    value.integration_id &&
    !serverChoices.some((choice) => choice.value === value.integration_id)
  ) {
    serverChoices.push({ value: value.integration_id, label: server?.name ?? "Missing server" });
  }
  const error = errors[errorPrefix] ?? errors[`${errorPrefix}.integration_id`];

  return (
    <>
      <SettingFieldRow
        label={label}
        htmlFor={controlId}
        dirty={dirty}
        status={<FieldError>{error}</FieldError>}
      >
        <Select value={selected} onValueChange={onSelect}>
          <SelectTrigger
            id={controlId}
            className={SETTINGS_CONTROL_WIDTH}
            aria-invalid={Boolean(error)}
          >
            <SelectValue placeholder="Choose a server" />
          </SelectTrigger>
          <SelectContent>
            {choices.map((choice) => (
              <SelectItem key={choice.value} value={choice.value}>
                {choice.label}
              </SelectItem>
            ))}
            {serverChoices.map((choice) => (
              <SelectItem key={choice.value} value={choice.value}>
                {choice.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </SettingFieldRow>
      {server ? (
        <RouteOverrideFields
          key={server.id}
          sectionId={sectionId}
          server={server}
          installations={installations}
          tierLabel={tierLabel}
          overrides={value.overrides}
          onChange={onOverridesChange}
          errors={errors}
          errorPrefix={errorPrefix}
        />
      ) : null}
    </>
  );
}
