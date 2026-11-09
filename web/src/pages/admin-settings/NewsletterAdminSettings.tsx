import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { v2, type V2Result } from "@/api/v2/request";
import {
  captureNotificationAuthority,
  notificationScope,
  requireNotificationAuthority,
} from "@/api/v2/notifications";
import { SettingsGroup } from "@/components/settings/SettingsGroup";
import { Switch } from "@/components/ui/switch";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { toast } from "sonner";

type Config = V2Result<"GET /api/v2/admin/notifications/newsletter">;

export function NewsletterAdminSettings() {
  return <NewsletterAdminForm key={notificationScope()} />;
}

function NewsletterAdminForm() {
  const authority = captureNotificationAuthority();
  const query = useQuery({
    queryKey: ["newsletter-settings", notificationScope(authority)],
    queryFn: () => v2("GET /api/v2/admin/notifications/newsletter", { profileContext: authority }),
    retry: false,
  });
  const [draft, setDraft] = useState<Config>();
  const [preview, setPreview] = useState<string>();
  const [archiveID, setArchiveID] = useState<number>();
  const config = draft ?? query.data;
  const sources = useQuery({
    queryKey: ["newsletter-import-sources", notificationScope(authority)],
    queryFn: () =>
      v2("GET /api/v2/admin/notifications/newsletter/import", { profileContext: authority }),
    retry: false,
  });
  const archive = useQuery({
    queryKey: ["newsletter-import-archive", notificationScope(authority), archiveID],
    enabled: archiveID !== undefined,
    queryFn: () =>
      v2("GET /api/v2/admin/notifications/newsletter/import/{installation_id}", {
        path: { installation_id: archiveID! },
        profileContext: authority,
      }),
    retry: false,
  });
  const migrate = useMutation({
    mutationFn: async (installation_id: number) => {
      requireNotificationAuthority(authority);
      const result = await v2("POST /api/v2/admin/notifications/newsletter/import", {
        body: { installation_id },
        profileContext: authority,
        retryAuthentication: false,
      });
      requireNotificationAuthority(authority);
      return result;
    },
    onSuccess: (result) => {
      setDraft(undefined);
      setArchiveID(result.installation_id);
      void query.refetch();
      void sources.refetch();
      toast.success(
        `Import saved: ${result.subscriptions} subscriptions transferred, ${result.needs_review} recipients need review. Delivery remains off.`,
      );
    },
    onError: (e) => toast.error(e.message),
  });
  const save = useMutation({
    mutationFn: async () => {
      requireNotificationAuthority(authority);
      const result = await v2("PUT /api/v2/admin/notifications/newsletter", {
        body: config!,
        profileContext: authority,
        retryAuthentication: false,
      });
      requireNotificationAuthority(authority);
      return result;
    },
    onSuccess: (result) => {
      setDraft(result);
      toast.success("Newsletter settings saved");
    },
    onError: (e) => toast.error(e.message),
  });
  const render = useMutation({
    mutationFn: async () => {
      requireNotificationAuthority(authority);
      const result = await v2("POST /api/v2/admin/notifications/newsletter/preview", {
        body: config!,
        profileContext: authority,
      });
      requireNotificationAuthority(authority);
      return result;
    },
    onSuccess: (result) => setPreview(result.html),
    onError: (e) => toast.error(e.message),
  });
  if (query.isError)
    return (
      <SettingsGroup title="Newsletter">
        <p role="alert">Could not load newsletter settings.</p>
        <Button onClick={() => void query.refetch()}>Retry</Button>
      </SettingsGroup>
    );
  if (!config) return null;
  const change = (content: Partial<Config["content"]>) =>
    setDraft({ ...config, content: { ...config.content, ...content } });
  const schedule = config.content.schedule;
  return (
    <SettingsGroup
      title="Newsletter and new-release emails"
      description="Built into Silo. Uses the shared outgoing-mail settings. Each profile chooses whether to subscribe and verifies its own address."
    >
      <fieldset
        disabled={save.isPending || migrate.isPending || render.isPending}
        className="space-y-4"
      >
        <label className="flex items-center justify-between">
          Enable newsletters and new-release alerts
          <Switch
            checked={config.enabled}
            onCheckedChange={(enabled) => setDraft({ ...config, enabled })}
          />
        </label>
        <label className="flex items-center justify-between">
          Pause scheduled delivery
          <Switch
            checked={config.content.delivery.paused}
            onCheckedChange={(paused) =>
              change({ delivery: { ...config.content.delivery, paused } })
            }
          />
        </label>
        <div className="grid gap-4 sm:grid-cols-2">
          <label>
            Timezone
            <Input
              value={schedule.timezone}
              placeholder="Europe/Warsaw"
              onChange={(e) => change({ schedule: { ...schedule, timezone: e.target.value } })}
            />
          </label>
          <label>
            Language
            <select
              className="bg-background block w-full rounded border p-2"
              value={config.content.language}
              onChange={(e) => change({ language: e.target.value })}
            >
              <option value="en">English</option>
              <option value="pl">Polski</option>
            </select>
          </label>
          <label>
            Daily send time
            <Input
              type="time"
              value={`${String(schedule.daily_hour).padStart(2, "0")}:${String(schedule.daily_minute).padStart(2, "0")}`}
              onChange={(e) => {
                const [daily_hour = 0, daily_minute = 0] = e.target.value.split(":").map(Number);
                change({ schedule: { ...schedule, daily_hour, daily_minute } });
              }}
            />
          </label>
          <label>
            Weekly send time
            <Input
              type="time"
              value={`${String(schedule.weekly_hour).padStart(2, "0")}:${String(schedule.weekly_minute).padStart(2, "0")}`}
              onChange={(e) => {
                const [weekly_hour = 0, weekly_minute = 0] = e.target.value.split(":").map(Number);
                change({ schedule: { ...schedule, weekly_hour, weekly_minute } });
              }}
            />
          </label>
          <label>
            Weekly send day
            <select
              className="bg-background block w-full rounded border p-2"
              value={schedule.weekly_day}
              onChange={(e) =>
                change({ schedule: { ...schedule, weekly_day: Number(e.target.value) } })
              }
            >
              {["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"].map(
                (day, i) => (
                  <option key={day} value={i}>
                    {day}
                  </option>
                ),
              )}
            </select>
          </label>
          <label>
            Maximum titles per email
            <Input
              type="number"
              min={1}
              max={500}
              value={config.content.delivery.max_items}
              onChange={(e) =>
                change({
                  delivery: { ...config.content.delivery, max_items: Number(e.target.value) },
                })
              }
            />
          </label>
        </div>
        <label className="block">
          Heading
          <Input
            value={config.content.theme.header_title}
            onChange={(e) =>
              change({ theme: { ...config.content.theme, header_title: e.target.value } })
            }
          />
        </label>
        <label className="block">
          Introduction
          <Input
            value={config.content.theme.intro}
            onChange={(e) => change({ theme: { ...config.content.theme, intro: e.target.value } })}
          />
        </label>
        <details>
          <summary className="cursor-pointer">Design and newsletter content</summary>
          <div className="my-4 grid gap-4 sm:grid-cols-2">
            {(
              [
                ["accent_color", "Accent color"],
                ["background_color", "Background color"],
                ["card_color", "Card color"],
                ["text_color", "Text color"],
                ["muted_color", "Secondary text color"],
                ["logo_url", "Logo URL"],
                ["footer_text", "Footer"],
                ["button_text", "Watch button label"],
              ] as const
            ).map(([field, label]) => (
              <label key={field}>
                {label}
                <Input
                  value={config.content.theme[field]}
                  onChange={(e) =>
                    change({ theme: { ...config.content.theme, [field]: e.target.value } })
                  }
                />
              </label>
            ))}
          </div>
          {(
            [
              ["personal", "Include personal viewing summaries"],
              ["popular", "Include popular titles"],
              ["weekly_when_empty", "Send a viewing summary when no new titles arrived"],
            ] as const
          ).map(([field, label]) => (
            <label key={field} className="my-3 flex items-center justify-between gap-4">
              {label}
              <Switch
                checked={config.content.recap[field]}
                onCheckedChange={(value) =>
                  change({ recap: { ...config.content.recap, [field]: value } })
                }
              />
            </label>
          ))}
          <label className="my-3 block">
            Popular titles to include
            <Input
              type="number"
              min={1}
              max={20}
              value={config.content.recap.popular_count}
              onChange={(e) =>
                change({
                  recap: { ...config.content.recap, popular_count: Number(e.target.value) },
                })
              }
            />
          </label>
        </details>
        <details>
          <summary className="cursor-pointer">Delivery limits and quiet hours</summary>
          <label className="my-3 flex items-center justify-between">
            Embed artwork
            <Switch
              checked={config.content.delivery.embed_images}
              onCheckedChange={(embed_images) =>
                change({ delivery: { ...config.content.delivery, embed_images } })
              }
            />
          </label>
          <label className="my-3 flex items-center justify-between">
            Quiet hours for instant alerts
            <Switch
              checked={config.content.delivery.quiet_enabled}
              onCheckedChange={(quiet_enabled) =>
                change({ delivery: { ...config.content.delivery, quiet_enabled } })
              }
            />
          </label>
          <div className="my-4 grid gap-4 sm:grid-cols-2">
            {(
              [
                ["quiet_start", "Quiet hours start (0–23)", 0, 23],
                ["quiet_end", "Quiet hours end (0–23)", 0, 23],
                ["settle_minutes", "Wait for new titles to settle (minutes)", 0, 240],
                ["instant_gap_minutes", "Minimum interval between alerts (minutes)", 0, 1440],
                ["instant_daily_cap", "Alerts per day (0 = unlimited)", 0, 500],
                ["max_age_hours", "Maximum age of new titles (hours)", 1, 1440],
                ["max_images", "Artwork images per email", 0, 150],
                ["synopsis_length", "Synopsis length (characters)", 0, 10000],
                ["burst_threshold", "Compact summaries above this many items (0 = off)", 0, 500],
                ["max_movies", "Movie limit (0 = overall limit)", 0, 500],
                ["max_shows", "Series limit (0 = overall limit)", 0, 500],
                ["send_delay_seconds", "Delay between recipients (seconds)", 0, 600],
              ] as const
            ).map(([field, label, min, max]) => (
              <label key={field}>
                {label}
                <Input
                  type="number"
                  min={min}
                  max={max}
                  value={config.content.delivery[field]}
                  onChange={(e) =>
                    change({
                      delivery: { ...config.content.delivery, [field]: Number(e.target.value) },
                    })
                  }
                />
              </label>
            ))}
          </div>
        </details>
        <details>
          <summary className="cursor-pointer">Custom template variables</summary>
          {config.content.custom_vars.map((variable, i) => (
            <div key={i} className="my-3 flex flex-wrap gap-2">
              <Input
                aria-label="Variable name"
                value={variable.key}
                onChange={(e) =>
                  change({
                    custom_vars: config.content.custom_vars.map((v, n) =>
                      n === i ? { ...v, key: e.target.value } : v,
                    ),
                  })
                }
              />
              <Input
                aria-label="Variable value"
                value={variable.value}
                onChange={(e) =>
                  change({
                    custom_vars: config.content.custom_vars.map((v, n) =>
                      n === i ? { ...v, value: e.target.value } : v,
                    ),
                  })
                }
              />
              <Button
                variant="outline"
                onClick={() =>
                  change({ custom_vars: config.content.custom_vars.filter((_, n) => n !== i) })
                }
              >
                Remove variable
              </Button>
            </div>
          ))}
          <Button
            variant="outline"
            onClick={() =>
              change({ custom_vars: [...config.content.custom_vars, { key: "", value: "" }] })
            }
          >
            Add variable
          </Button>
        </details>
        <details>
          <summary className="cursor-pointer">Email templates</summary>
          <p className="text-muted-foreground my-3 text-sm">
            Existing Email Alerts template variables are supported. Preview uses example titles and
            does not send email.
          </p>
          {(["subject", "html", "text"] as const).map((field) => (
            <label key={field} className="my-3 block capitalize">
              {field}
              <textarea
                className="bg-background mt-1 min-h-28 w-full rounded border p-2 font-mono text-sm"
                value={config.content.templates[field]}
                onChange={(e) =>
                  change({ templates: { ...config.content.templates, [field]: e.target.value } })
                }
              />
            </label>
          ))}
        </details>
        <div className="flex flex-wrap gap-2">
          <Button disabled={save.isPending} onClick={() => save.mutate()}>
            Save newsletter settings
          </Button>
          <Button variant="outline" disabled={render.isPending} onClick={() => render.mutate()}>
            Preview email
          </Button>
        </div>
        {preview && (
          <iframe
            title="Newsletter preview"
            sandbox=""
            srcDoc={preview}
            className="h-[36rem] w-full rounded border"
          />
        )}
      </fieldset>
      <details>
        <summary className="cursor-pointer">Import Email Alerts plugin</summary>
        <p className="text-muted-foreground my-3 text-sm">
          Disable the Email Alerts plugin and native newsletter delivery before importing. Settings,
          recipients and history are archived. Existing native settings and user choices are kept.
          Only subscribers with a matching verified profile address transfer automatically; other
          recipients must verify their address and choose delivery in their profile settings.
          Imported SMTP settings remain disabled until you enable outgoing mail.
        </p>
        {sources.isError && <p role="alert">Could not load plugin imports.</p>}
        {sources.data?.items.length === 0 && <p>No Email Alerts installation or archive found.</p>}
        {sources.data?.items.map((source) => (
          <div key={source.installation_id} className="my-3 flex flex-wrap items-center gap-3">
            <span>Email Alerts {source.version}</span>
            {source.imported ? (
              <Button variant="outline" onClick={() => setArchiveID(source.installation_id)}>
                Review archived subscribers and history
              </Button>
            ) : (
              <Button
                disabled={source.enabled || config.enabled || migrate.isPending || save.isPending}
                onClick={() => migrate.mutate(source.installation_id)}
              >
                Import plugin data
              </Button>
            )}
            {source.enabled && <span>Disable the plugin first.</span>}
            {source.result && (
              <span>
                {source.result.subscriptions} transferred; {source.result.needs_review} need review.
              </span>
            )}
          </div>
        ))}
        {archive.isError && <p role="alert">Could not load the import archive.</p>}
        {archive.data && (
          <div className="space-y-4 overflow-x-auto">
            <table className="w-full text-left text-sm">
              <caption className="text-left">Archived plugin subscriptions</caption>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Email</th>
                  <th>Mode</th>
                  <th>Status at import</th>
                </tr>
              </thead>
              <tbody>
                {archive.data.recipients.map((r, i) => (
                  <tr key={i}>
                    <td>{r.name}</td>
                    <td>{r.email}</td>
                    <td>{r.mode}</td>
                    <td>
                      {r.enabled ? "Enabled" : "Off"}
                      {r.verified ? ", verified" : ", unverified"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            <table className="w-full text-left text-sm">
              <caption className="text-left">Plugin send history</caption>
              <thead>
                <tr>
                  <th>Time</th>
                  <th>Recipient</th>
                  <th>Subject</th>
                  <th>Result</th>
                </tr>
              </thead>
              <tbody>
                {archive.data.history.map((h, i) => (
                  <tr key={i}>
                    <td>{h.time}</td>
                    <td>{h.recipient}</td>
                    <td>{h.subject}</td>
                    <td>{h.error || `${h.items} items`}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </details>
    </SettingsGroup>
  );
}
