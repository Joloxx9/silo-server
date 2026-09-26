import { useId, useState, type ReactNode } from "react";
import { Trash2 } from "lucide-react";

import type { DiscoverBrandCard, RequestIntegration } from "@/api/types";
import {
  getAdminRequestRouteV2,
  isRequestEditorConflict,
  requestValidationErrors,
  type RequestRoute,
  type RequestRouteConditions,
  type RequestRouteMediaType,
} from "@/api/v2/adminRequests";
import { EditorConflict } from "@/components/admin/EditorConflict";
import { AdvancedSection } from "@/components/settings/AdvancedSection";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import {
  useCreateRequestRoute,
  useDeleteRequestRoute,
  useUpdateRequestRoute,
} from "@/hooks/queries/useRequests";
import {
  ROUTING_COUNTRIES,
  ROUTING_DECADES,
  ROUTING_LANGUAGES,
  routingCountryName,
  routingLanguageName,
} from "@/lib/requestRoutingOptions";
import { tmdbGenresFor } from "@/lib/tmdbGenres";

import {
  ChipToggleList,
  DEST_NEXT,
  DEST_SKIP,
  FieldError,
  RouteDestinationFields,
  ValuePicker,
} from "./RequestRouteFields";
import { ruleBody, ruleDraft, type RouteRuleDraft } from "./requestRoutingModel";
import type { RequestRouterInstallation } from "./requestServerModel";
import { SETTINGS_CONTROL_WIDTH, SettingFieldRow } from "./SettingField";

type AnimeChoice = "any" | "only" | "not";

function animeChoice(anime: boolean | undefined): AnimeChoice {
  return anime === true ? "only" : anime === false ? "not" : "any";
}

/** Field errors this editor shows beside a control; the rest go to the top. */
function isPlacedError(key: string): boolean {
  return (
    key === "name" ||
    key === "conditions" ||
    key.startsWith("conditions.") ||
    key === "hd" ||
    key.startsWith("hd.") ||
    key === "uhd" ||
    key.startsWith("uhd.")
  );
}

/** A condition whose control needs the full width: label above, control below. */
function ConditionBlock({
  label,
  htmlFor,
  description,
  error,
  children,
}: {
  label: string;
  htmlFor?: string;
  description?: string;
  error?: string;
  children: ReactNode;
}) {
  return (
    <div className="space-y-2 py-3.5">
      <div className="space-y-0.5">
        <Label htmlFor={htmlFor} className="text-sm font-medium">
          {label}
        </Label>
        {description ? (
          <p className="text-muted-foreground text-xs leading-relaxed">{description}</p>
        ) : null}
      </div>
      {children}
      <FieldError>{error}</FieldError>
    </div>
  );
}

function parseIds(text: string): number[] | null {
  const tokens = text
    .split(/[\s,]+/)
    .map((token) => token.trim())
    .filter(Boolean);
  const ids = tokens.map((token) => Number(token));
  return ids.every((id) => Number.isInteger(id) && id > 0) ? ids : null;
}

function parseYear(value: string): number | undefined {
  const year = Number.parseInt(value, 10);
  return Number.isFinite(year) && year > 0 ? year : undefined;
}

/**
 * Adds or edits one routing rule: the titles it matches and where it sends
 * each quality tier. Saves on its own, like a server; the page's save bar is
 * for the settings above it.
 */
export function RequestRuleEditor({
  mediaType,
  source,
  servers,
  allServers,
  installations,
  users,
  brands,
  brandsHint,
  onDone,
}: {
  mediaType: RequestRouteMediaType;
  source: RequestRoute | null;
  servers: RequestIntegration[];
  allServers: RequestIntegration[];
  installations: RequestRouterInstallation[];
  users: readonly { id: number; username: string }[];
  brands: readonly DiscoverBrandCard[];
  /** Why the curated network/studio list cannot be offered; a TMDB ID still can. */
  brandsHint?: string;
  onDone: () => void;
}) {
  const [draft, setDraft] = useState<RouteRuleDraft>(() => ruleDraft(source));
  const [keywords, setKeywords] = useState(() => (source?.conditions.keyword_ids ?? []).join(", "));
  const [brandId, setBrandId] = useState("");
  const [etag, setETag] = useState(source?.etag);
  const [conflict, setConflict] = useState(false);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState<string | null>(null);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const createRoute = useCreateRequestRoute();
  const updateRoute = useUpdateRequestRoute({ inlineErrors: true });
  const deleteRoute = useDeleteRequestRoute();
  const nameId = useId();
  const animeId = useId();
  const yearId = useId();
  const keywordsId = useId();
  const brandInputId = useId();
  const isNew = source === null;

  const brandKey = mediaType === "series" ? "network_ids" : "company_ids";
  const brandNoun = mediaType === "series" ? "network" : "studio";
  const brandNames = new Map(
    brands
      .filter((brand) => brand.tmdb_id)
      .map((brand) => [String(brand.tmdb_id), brand.display_name]),
  );
  const conditions = draft.conditions;

  function edit(change: (current: RouteRuleDraft) => RouteRuleDraft) {
    setFieldErrors((current) => (Object.keys(current).length === 0 ? current : {}));
    setFormError(null);
    setDraft(change);
  }

  function setCondition<K extends keyof RequestRouteConditions>(
    key: K,
    value: RequestRouteConditions[K],
  ) {
    edit((current) => ({ ...current, conditions: { ...current.conditions, [key]: value } }));
  }

  function addBrandId() {
    const id = Number(brandId.trim());
    if (!Number.isInteger(id) || id <= 0) return;
    const current = conditions[brandKey] ?? [];
    if (!current.includes(id)) setCondition(brandKey, [...current, id]);
    setBrandId("");
  }

  async function reload() {
    if (!source) return;
    try {
      const latest = await getAdminRequestRouteV2(source.id);
      setDraft(ruleDraft(latest));
      setKeywords((latest.conditions.keyword_ids ?? []).join(", "));
      setETag(latest.etag);
      setConflict(false);
      setFieldErrors({});
      setFormError(null);
    } catch (error) {
      setFormError(error instanceof Error ? error.message : "Reload failed");
    }
  }

  function onSaveError(error: unknown) {
    if (isRequestEditorConflict(error)) setConflict(true);
    const validation = requestValidationErrors(error);
    if (validation) {
      setFieldErrors(validation.fields);
      setFormError(validation.message);
    }
  }

  function save() {
    const keywordIds = parseIds(keywords);
    if (keywordIds === null) {
      setFieldErrors({ "conditions.keyword_ids": "Enter TMDB keyword IDs as numbers." });
      return;
    }
    const withKeywords = {
      ...draft,
      conditions: { ...draft.conditions, keyword_ids: keywordIds },
    };
    setFieldErrors({});
    setFormError(null);
    if (isNew) {
      createRoute.mutate(ruleBody(withKeywords, mediaType), {
        onSuccess: onDone,
        onError: onSaveError,
      });
    } else if (etag) {
      updateRoute.mutate(
        { route: { id: source.id, etag }, body: ruleBody(withKeywords) },
        { onSuccess: onDone, onError: onSaveError },
      );
    }
  }

  const saving = createRoute.isPending || updateRoute.isPending;
  const unplaced = Object.entries(fieldErrors).filter(([key]) => !isPlacedError(key));
  const genres = tmdbGenresFor(mediaType).map((genre) => ({
    value: String(genre.id),
    label: genre.name,
  }));
  const tierChoice = [{ value: DEST_NEXT, label: "Next matching rule or default" }];

  return (
    <>
      <DialogHeader>
        <DialogTitle>{isNew ? "Add rule" : `Edit ${source.name}`}</DialogTitle>
        <DialogDescription>
          A title matches when it meets every condition you set; a condition with several values
          matches any of them.
        </DialogDescription>
      </DialogHeader>

      {formError || unplaced.length > 0 ? (
        <div className="border-destructive/40 bg-destructive/10 text-destructive space-y-1 rounded-md border px-3 py-2 text-sm">
          {formError ? <p>{formError}</p> : null}
          {unplaced.map(([key, detail]) => (
            <p key={key}>{detail}</p>
          ))}
        </div>
      ) : null}

      <div className="settings-field-list">
        <SettingFieldRow
          label="Name"
          htmlFor={nameId}
          status={<FieldError>{fieldErrors.name}</FieldError>}
        >
          <Input
            id={nameId}
            value={draft.name}
            maxLength={100}
            onChange={(event) => edit((current) => ({ ...current, name: event.target.value }))}
            placeholder="e.g. Anime"
            aria-invalid={Boolean(fieldErrors.name)}
            className={SETTINGS_CONTROL_WIDTH}
          />
        </SettingFieldRow>
        <SettingFieldRow label="Enabled" htmlFor={`${nameId}-enabled`}>
          <Switch
            id={`${nameId}-enabled`}
            checked={draft.enabled}
            onCheckedChange={(enabled) => edit((current) => ({ ...current, enabled }))}
          />
        </SettingFieldRow>
      </div>

      <section aria-labelledby={`${nameId}-conditions`} className="space-y-1">
        <h3 id={`${nameId}-conditions`} className="text-sm font-semibold">
          Titles it matches
        </h3>
        <FieldError>{fieldErrors.conditions}</FieldError>
        <div className="settings-field-list">
          <SettingFieldRow
            label="Anime"
            htmlFor={animeId}
            status={<FieldError>{fieldErrors["conditions.anime"]}</FieldError>}
          >
            <Select
              value={animeChoice(conditions.anime)}
              onValueChange={(value) =>
                setCondition("anime", value === "only" ? true : value === "not" ? false : undefined)
              }
            >
              <SelectTrigger id={animeId} className={SETTINGS_CONTROL_WIDTH}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="any">Any title</SelectItem>
                <SelectItem value="only">Anime only</SelectItem>
                <SelectItem value="not">Not anime</SelectItem>
              </SelectContent>
            </Select>
          </SettingFieldRow>

          <ConditionBlock label="Genres" error={fieldErrors["conditions.genre_ids"]}>
            <ChipToggleList
              label="Genres"
              options={genres}
              selected={(conditions.genre_ids ?? []).map(String)}
              onChange={(next) => setCondition("genre_ids", next.map(Number))}
            />
          </ConditionBlock>

          <ConditionBlock
            label="Original language"
            error={fieldErrors["conditions.original_languages"]}
          >
            <ValuePicker
              addLabel="Add a language"
              options={ROUTING_LANGUAGES.map((language) => ({
                value: language.code,
                label: language.name,
              }))}
              selected={conditions.original_languages ?? []}
              labelOf={routingLanguageName}
              onChange={(next) => setCondition("original_languages", next)}
            />
          </ConditionBlock>

          <ConditionBlock
            label="Country of origin"
            error={fieldErrors["conditions.origin_countries"]}
          >
            <ValuePicker
              addLabel="Add a country"
              options={ROUTING_COUNTRIES.map((country) => ({
                value: country.code,
                label: country.name,
              }))}
              selected={conditions.origin_countries ?? []}
              labelOf={routingCountryName}
              onChange={(next) => setCondition("origin_countries", next)}
            />
          </ConditionBlock>

          <ConditionBlock
            label="Release year"
            htmlFor={yearId}
            description={
              mediaType === "series" ? "The year the series first aired." : "Leave a side empty."
            }
            error={fieldErrors["conditions.year_from"] ?? fieldErrors["conditions.year_to"]}
          >
            <div className="flex flex-wrap items-center gap-2">
              <Input
                id={yearId}
                type="number"
                aria-label="From year"
                placeholder="From"
                className="w-28"
                value={conditions.year_from ?? ""}
                onChange={(event) => setCondition("year_from", parseYear(event.target.value))}
                aria-invalid={Boolean(fieldErrors["conditions.year_from"])}
              />
              <span className="text-muted-foreground text-sm" aria-hidden="true">
                –
              </span>
              <Input
                type="number"
                aria-label="To year"
                placeholder="To"
                className="w-28"
                value={conditions.year_to ?? ""}
                onChange={(event) => setCondition("year_to", parseYear(event.target.value))}
                aria-invalid={Boolean(fieldErrors["conditions.year_to"])}
              />
            </div>
            <div role="group" aria-label="Decades" className="flex flex-wrap gap-1.5">
              {ROUTING_DECADES.map((decade) => {
                const on = conditions.year_from === decade.from && conditions.year_to === decade.to;
                return (
                  <Button
                    key={decade.label}
                    type="button"
                    size="xs"
                    variant={on ? "default" : "outline"}
                    aria-pressed={on}
                    onClick={() =>
                      edit((current) => ({
                        ...current,
                        conditions: {
                          ...current.conditions,
                          year_from: on ? undefined : decade.from,
                          year_to: on ? undefined : decade.to,
                        },
                      }))
                    }
                  >
                    {decade.label}
                  </Button>
                );
              })}
            </div>
          </ConditionBlock>

          <ConditionBlock
            label={mediaType === "series" ? "Network" : "Studio"}
            htmlFor={brandInputId}
            error={fieldErrors[`conditions.${brandKey}`]}
          >
            <ValuePicker
              addLabel={`Add a ${brandNoun}`}
              options={[...brandNames].map(([value, label]) => ({ value, label }))}
              selected={(conditions[brandKey] ?? []).map(String)}
              labelOf={(value) => brandNames.get(value) ?? `TMDB ${value}`}
              onChange={(next) => setCondition(brandKey, next.map(Number))}
              unavailableHint={brandsHint}
            />
            <div className="flex items-center gap-2">
              <Input
                id={brandInputId}
                type="number"
                inputMode="numeric"
                aria-label={`${brandNoun === "network" ? "Network" : "Studio"} TMDB ID`}
                placeholder="Or a TMDB ID"
                className="w-40"
                value={brandId}
                onChange={(event) => setBrandId(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "Enter") {
                    event.preventDefault();
                    addBrandId();
                  }
                }}
              />
              <Button type="button" size="sm" variant="outline" onClick={addBrandId}>
                Add ID
              </Button>
            </div>
          </ConditionBlock>

          <ConditionBlock
            label="Requested by"
            description="Only requests from these accounts."
            error={fieldErrors["conditions.requester_user_ids"]}
          >
            <ValuePicker
              addLabel="Add an account"
              options={users.map((user) => ({ value: String(user.id), label: user.username }))}
              selected={(conditions.requester_user_ids ?? []).map(String)}
              labelOf={(value) =>
                users.find((user) => String(user.id) === value)?.username ?? `Account ${value}`
              }
              onChange={(next) => setCondition("requester_user_ids", next.map(Number))}
            />
          </ConditionBlock>

          <AdvancedSection
            // One open state per rule, not one shared by every rule editor.
            id={`requests.rule-conditions.${source?.id ?? "new"}`}
            count={1}
            forceOpen={keywords.trim() !== "" || Boolean(fieldErrors["conditions.keyword_ids"])}
          >
            <SettingFieldRow
              label="TMDB keyword IDs"
              htmlFor={keywordsId}
              description="Separate several with commas."
              status={<FieldError>{fieldErrors["conditions.keyword_ids"]}</FieldError>}
            >
              <Input
                id={keywordsId}
                value={keywords}
                onChange={(event) => {
                  setFieldErrors({});
                  setKeywords(event.target.value);
                }}
                placeholder="210024"
                className={SETTINGS_CONTROL_WIDTH}
                aria-invalid={Boolean(fieldErrors["conditions.keyword_ids"])}
              />
            </SettingFieldRow>
          </AdvancedSection>
        </div>
      </section>

      <section aria-labelledby={`${nameId}-destinations`} className="space-y-1">
        <h3 id={`${nameId}-destinations`} className="text-sm font-semibold">
          Where they go
        </h3>
        <div className="settings-field-list">
          <RouteDestinationFields
            sectionId={`${source?.id ?? `new-${mediaType}`}.hd`}
            label="HD server"
            tierLabel="HD"
            servers={servers}
            allServers={allServers}
            installations={installations}
            value={draft.hd}
            choices={tierChoice}
            selected={draft.hd.integration_id || DEST_NEXT}
            onSelect={(value) =>
              edit((current) => ({
                ...current,
                hd:
                  value === DEST_NEXT
                    ? { integration_id: "", overrides: {} }
                    : value === current.hd.integration_id
                      ? current.hd
                      : { integration_id: value, overrides: {} },
              }))
            }
            onOverridesChange={(overrides) =>
              edit((current) => ({ ...current, hd: { ...current.hd, overrides } }))
            }
            errors={fieldErrors}
            errorPrefix="hd"
          />
          <RouteDestinationFields
            sectionId={`${source?.id ?? `new-${mediaType}`}.uhd`}
            label="4K server"
            tierLabel="4K"
            servers={servers}
            allServers={allServers}
            installations={installations}
            value={draft.skipUhd ? { integration_id: "", overrides: {} } : draft.uhd}
            choices={[...tierChoice, { value: DEST_SKIP, label: "No 4K copy" }]}
            selected={draft.skipUhd ? DEST_SKIP : draft.uhd.integration_id || DEST_NEXT}
            onSelect={(value) =>
              edit((current) => ({
                ...current,
                skipUhd: value === DEST_SKIP,
                uhd:
                  value === DEST_SKIP || value === DEST_NEXT
                    ? { integration_id: "", overrides: {} }
                    : value === current.uhd.integration_id
                      ? current.uhd
                      : { integration_id: value, overrides: {} },
              }))
            }
            onOverridesChange={(overrides) =>
              edit((current) => ({ ...current, uhd: { ...current.uhd, overrides } }))
            }
            errors={fieldErrors}
            errorPrefix="uhd"
          />
        </div>
      </section>

      {conflict ? <EditorConflict onReload={reload} /> : null}

      <DialogFooter className="flex-wrap gap-2 sm:justify-between">
        <div>
          {!isNew ? (
            <Button
              type="button"
              variant="outline"
              className="text-destructive"
              onClick={() => setConfirmDelete(true)}
              disabled={deleteRoute.isPending || conflict || !etag}
            >
              <Trash2 aria-hidden="true" />
              Delete
            </Button>
          ) : null}
        </div>
        <div className="flex items-center gap-2">
          <Button type="button" variant="outline" onClick={onDone}>
            Cancel
          </Button>
          <Button
            type="button"
            onClick={save}
            disabled={saving || conflict || draft.name.trim() === "" || (!isNew && !etag)}
          >
            {saving ? "Saving…" : isNew ? "Add rule" : "Save rule"}
          </Button>
        </div>
      </DialogFooter>

      {!isNew ? (
        <AlertDialog open={confirmDelete} onOpenChange={setConfirmDelete}>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>Delete {source.name}?</AlertDialogTitle>
              <AlertDialogDescription>
                Titles it matched go to the next matching rule or the default destination.
              </AlertDialogDescription>
            </AlertDialogHeader>
            {conflict ? <EditorConflict onReload={reload} /> : null}
            <AlertDialogFooter>
              <AlertDialogCancel>Cancel</AlertDialogCancel>
              <AlertDialogAction
                className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
                disabled={deleteRoute.isPending || conflict || !etag}
                onClick={(event) => {
                  event.preventDefault();
                  if (!etag) return;
                  deleteRoute.mutate(
                    { id: source.id, etag },
                    {
                      onSuccess: () => {
                        setConfirmDelete(false);
                        onDone();
                      },
                      onError: (error) => {
                        if (isRequestEditorConflict(error)) setConflict(true);
                      },
                    },
                  );
                }}
              >
                Delete
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      ) : null}
    </>
  );
}
