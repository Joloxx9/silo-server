import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { v2, type V2Result } from "@/api/v2/request";
import {
  type captureNotificationAuthority,
  notificationScope,
  requireNotificationAuthority,
} from "@/api/v2/notifications";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { toast } from "sonner";
import { useNewsletterText } from "@/lib/newsletterText";

type Config = V2Result<"GET /api/v2/admin/notifications/newsletter">;
type Authority = ReturnType<typeof captureNotificationAuthority>;

const selectClass = "bg-background h-9 rounded border px-2 text-sm";

// NewsletterRecipients lists the profiles with a verified address and sends a
// test newsletter built from one profile's real catalog access and history.
export function NewsletterRecipients({
  config,
  authority,
}: {
  config: Config;
  authority: Authority;
}) {
  const tx = useNewsletterText();
  const recipients = useQuery({
    queryKey: ["newsletter-recipients", notificationScope(authority)],
    queryFn: () =>
      v2("GET /api/v2/admin/notifications/newsletter/recipients", { profileContext: authority }),
    retry: false,
  });
  // "" means the signed-in profile; otherwise "<user_id>:<profile_id>".
  const [target, setTarget] = useState("");
  const [email, setEmail] = useState("");
  const [html, setHtml] = useState<string>();
  const run = useMutation({
    mutationFn: async (send: boolean) => {
      requireNotificationAuthority(authority);
      const [user, ...profile] = target.split(":");
      const result = await v2("POST /api/v2/admin/notifications/newsletter/test", {
        body: {
          settings: config,
          mode: "weekly",
          ...(target ? { user_id: Number(user), profile_id: profile.join(":") } : {}),
          ...(send ? { email: email.trim() } : {}),
        },
        profileContext: authority,
        retryAuthentication: false,
      });
      requireNotificationAuthority(authority);
      return result;
    },
    onSuccess: (result) => {
      if (result.empty) {
        setHtml(undefined);
        toast.info(tx("Nothing arrived in this window and there is no recap, so no email."));
        return;
      }
      setHtml(result.html);
      if (result.sent) toast.success(tx("Test email sent to {email}", { email: email.trim() }));
    },
    onError: (e) => toast.error(e.message),
  });
  const items = recipients.data?.items ?? [];
  return (
    <>
      <details open className="rounded border p-4">
        <summary className="cursor-pointer font-medium">{tx("Send a test email")}</summary>
        <p className="text-muted-foreground my-3 text-sm">
          {tx(
            "Builds the newsletter one profile would get for the last day or week, from real titles, artwork and that profile's watchlist and history, using the settings on this page even before you save them. Sending a test does not change anyone's schedule.",
          )}
        </p>
        <div className="flex flex-wrap items-end gap-2">
          <label className="flex flex-col gap-1 text-sm">
            {tx("Build it for")}
            <select
              className={selectClass}
              value={target}
              onChange={(e) => {
                setTarget(e.target.value);
                const picked = items.find((r) => `${r.user_id}:${r.profile_id}` === e.target.value);
                if (picked?.email) setEmail(picked.email);
              }}
            >
              <option value="">{tx("Me")}</option>
              {items.map((r) => (
                <option key={`${r.user_id}:${r.profile_id}`} value={`${r.user_id}:${r.profile_id}`}>
                  {r.profile_name || r.profile_id}
                  {r.email ? ` (${r.email})` : ""}
                </option>
              ))}
            </select>
          </label>
          <label className="flex min-w-60 flex-1 flex-col gap-1 text-sm">
            {tx("Send to")}
            <Input
              type="email"
              placeholder="name@example.com"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
          </label>
          <Button variant="outline" disabled={run.isPending} onClick={() => run.mutate(false)}>
            {tx("Preview with real data")}
          </Button>
          <Button disabled={run.isPending || !email.trim()} onClick={() => run.mutate(true)}>
            {tx("Send test")}
          </Button>
        </div>
        {html && (
          <iframe
            title={tx("Newsletter test preview")}
            sandbox=""
            srcDoc={html}
            className="mt-3 h-[48rem] w-full max-w-[680px] rounded border"
          />
        )}
      </details>
      <details open className="rounded border p-4">
        <summary className="cursor-pointer font-medium">
          {tx("Profiles ({total}), with an address: {n}", {
            total: items.length,
            n: items.filter((r) => r.email).length,
          })}
        </summary>
        <p className="text-muted-foreground my-3 text-sm">
          {tx(
            "Every profile on the server. Give a profile an email address here, for example a household profile that never signs in, and choose what it receives. Child profiles cannot receive email.",
          )}
        </p>
        {recipients.isError && <p role="alert">{tx("Could not load recipients.")}</p>}
        <div className="space-y-2">
          {items.map((r) => (
            <RecipientRow
              key={`${r.user_id}:${r.profile_id}`}
              recipient={r}
              authority={authority}
              onSaved={() => void recipients.refetch()}
            />
          ))}
        </div>
      </details>
    </>
  );
}

type Recipient = V2Result<"GET /api/v2/admin/notifications/newsletter/recipients">["items"][number];

function RecipientRow({
  recipient: r,
  authority,
  onSaved,
}: {
  recipient: Recipient;
  authority: Authority;
  onSaved: () => void;
}) {
  const tx = useNewsletterText();
  // The primary profile is offered the account's own address; it is saved
  // only when the admin presses Save.
  const [email, setEmail] = useState(r.email || (r.is_primary ? r.account_email : ""));
  const [newsletter, setNewsletter] = useState(r.newsletter);
  const [alerts, setAlerts] = useState(r.alerts);
  const dirty = email.trim() !== r.email || newsletter !== r.newsletter || alerts !== r.alerts;
  const save = useMutation({
    mutationFn: async () => {
      requireNotificationAuthority(authority);
      await v2("PUT /api/v2/admin/notifications/newsletter/recipients/{user_id}/{profile_id}", {
        path: { user_id: r.user_id, profile_id: r.profile_id },
        body: { email: email.trim(), newsletter, alerts },
        profileContext: authority,
        retryAuthentication: false,
      });
      requireNotificationAuthority(authority);
    },
    onSuccess: () => {
      toast.success(tx("Saved {name}", { name: r.profile_name || r.profile_id }));
      onSaved();
    },
    onError: (e) => toast.error(e.message),
  });
  const noEmail = !email.trim() || r.is_child;
  const [open, setOpen] = useState(
    () => rememberedOpen(r) ?? ((!!r.email || r.is_primary) && !r.is_child),
  );
  const summary = r.is_child
    ? tx("Child profile")
    : r.email
      ? `${r.email} · ${tx("Newsletter")}: ${tx(r.newsletter)} · ${tx("New-release alerts")}: ${tx(r.alerts)}`
      : tx("No address");
  return (
    <details
      className="rounded border p-3"
      open={open}
      onToggle={(e) => {
        const next = e.currentTarget.open;
        setOpen(next);
        rememberOpen(r, next);
      }}
    >
      <summary className="flex cursor-pointer flex-wrap items-baseline gap-x-2 text-sm">
        <span className="font-medium">{r.profile_name || r.profile_id}</span>
        {r.account && <span className="text-muted-foreground">{r.account}</span>}
        <span className="text-muted-foreground min-w-0 truncate">{summary}</span>
        <span className="text-muted-foreground ms-auto">
          {tx("Last email")}:{" "}
          {r.last_sent_at ? new Date(r.last_sent_at).toLocaleString() : tx("Never")}
        </span>
      </summary>
      <div className="mt-2 flex flex-wrap items-end gap-2">
        <label className="flex min-w-56 flex-1 flex-col gap-1 text-sm">
          {tx("Email")}
          <Input
            type="email"
            placeholder="name@example.com"
            value={email}
            disabled={r.is_child}
            onChange={(e) => setEmail(e.target.value)}
          />
        </label>
        <label className="flex flex-col gap-1 text-sm">
          {tx("Newsletter")}
          <select
            className={selectClass}
            value={noEmail ? "off" : newsletter}
            disabled={noEmail}
            onChange={(e) => setNewsletter(e.target.value as Recipient["newsletter"])}
          >
            <option value="off">{tx("off")}</option>
            <option value="weekly">{tx("weekly")}</option>
          </select>
        </label>
        <label className="flex flex-col gap-1 text-sm">
          {tx("New-release alerts")}
          <select
            className={selectClass}
            value={noEmail ? "off" : alerts}
            disabled={noEmail}
            onChange={(e) => setAlerts(e.target.value as Recipient["alerts"])}
          >
            <option value="off">{tx("off")}</option>
            <option value="instant">{tx("instant")}</option>
            <option value="daily">{tx("daily")}</option>
          </select>
        </label>
        <Button disabled={!dirty || save.isPending || r.is_child} onClick={() => save.mutate()}>
          {tx("Save")}
        </Button>
      </div>
      {r.is_child ? (
        <p className="text-muted-foreground mt-2 text-xs">
          {tx("Child profiles cannot receive email.")}
        </p>
      ) : (
        !email.trim() && (
          <p className="text-muted-foreground mt-2 text-xs">
            {tx("Enter an email address to turn on the newsletter and alerts.")}
          </p>
        )
      )}
      {!r.email && r.is_primary && email.trim() && email.trim() === r.account_email && (
        <p className="text-muted-foreground mt-2 text-xs">
          {tx("Suggested from the account's sign-in address. Press Save to use it.")}
        </p>
      )}
    </details>
  );
}

// Which profile rows the admin collapsed or expanded, per browser. A missing
// or blocked store falls back to the default.
const openKey = "silo.newsletter.recipientOpen";

function rememberedOpen(r: Recipient): boolean | undefined {
  try {
    const saved = JSON.parse(localStorage.getItem(openKey) ?? "{}") as Record<string, boolean>;
    return saved[`${r.user_id}:${r.profile_id}`];
  } catch {
    return undefined;
  }
}

function rememberOpen(r: Recipient, open: boolean) {
  try {
    const saved = JSON.parse(localStorage.getItem(openKey) ?? "{}") as Record<string, boolean>;
    saved[`${r.user_id}:${r.profile_id}`] = open;
    localStorage.setItem(openKey, JSON.stringify(saved));
  } catch {
    // Remembering is a convenience; the row still works without it.
  }
}
