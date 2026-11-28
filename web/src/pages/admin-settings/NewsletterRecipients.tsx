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
  const recipients = useQuery({
    queryKey: ["newsletter-recipients", notificationScope(authority)],
    queryFn: () =>
      v2("GET /api/v2/admin/notifications/newsletter/recipients", { profileContext: authority }),
    retry: false,
  });
  // "" means the signed-in profile; otherwise "<user_id>:<profile_id>".
  const [target, setTarget] = useState("");
  const [mode, setMode] = useState<"weekly" | "daily">("weekly");
  const [email, setEmail] = useState("");
  const [html, setHtml] = useState<string>();
  const run = useMutation({
    mutationFn: async (send: boolean) => {
      requireNotificationAuthority(authority);
      const [user, ...profile] = target.split(":");
      const result = await v2("POST /api/v2/admin/notifications/newsletter/test", {
        body: {
          settings: config,
          mode,
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
        toast.info("Nothing arrived in this window and there is no recap, so no email.");
        return;
      }
      setHtml(result.html);
      if (result.sent) toast.success(`Test email sent to ${email.trim()}`);
    },
    onError: (e) => toast.error(e.message),
  });
  const items = recipients.data?.items ?? [];
  return (
    <>
      <details>
        <summary className="cursor-pointer">Recipients ({items.length})</summary>
        <p className="text-muted-foreground my-3 text-sm">
          Profiles with a verified email address. Each profile chooses what it receives in its own
          notification settings.
        </p>
        {recipients.isError && <p role="alert">Could not load recipients.</p>}
        {items.length > 0 && (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead className="text-muted-foreground">
                <tr>
                  <th className="py-1 pr-3 font-medium">Profile</th>
                  <th className="py-1 pr-3 font-medium">Email</th>
                  <th className="py-1 pr-3 font-medium">Newsletter</th>
                  <th className="py-1 pr-3 font-medium">New-release alerts</th>
                  <th className="py-1 font-medium">Last email</th>
                </tr>
              </thead>
              <tbody>
                {items.map((r) => (
                  <tr key={`${r.user_id}:${r.profile_id}`} className="border-t">
                    <td className="py-1 pr-3">
                      {r.profile_name || r.profile_id}
                      {r.account && <span className="text-muted-foreground"> · {r.account}</span>}
                    </td>
                    <td className="py-1 pr-3">{r.email}</td>
                    <td className="py-1 pr-3">{r.newsletter}</td>
                    <td className="py-1 pr-3">{r.alerts}</td>
                    <td className="py-1">
                      {r.last_sent_at ? new Date(r.last_sent_at).toLocaleString() : "Never"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        {recipients.isSuccess && items.length === 0 && (
          <p className="text-sm">No profile has verified an email address yet.</p>
        )}
      </details>
      <details>
        <summary className="cursor-pointer">Send a test email</summary>
        <p className="text-muted-foreground my-3 text-sm">
          Builds the newsletter one profile would get for the last day or week, from real titles,
          artwork and that profile&apos;s watchlist and history, using the settings on this page
          even before you save them. Sending a test does not change anyone&apos;s schedule.
        </p>
        <div className="flex flex-wrap items-end gap-2">
          <label className="flex flex-col gap-1 text-sm">
            Build it for
            <select
              className={selectClass}
              value={target}
              onChange={(e) => setTarget(e.target.value)}
            >
              <option value="">Me</option>
              {items.map((r) => (
                <option key={`${r.user_id}:${r.profile_id}`} value={`${r.user_id}:${r.profile_id}`}>
                  {r.profile_name || r.profile_id} ({r.email})
                </option>
              ))}
            </select>
          </label>
          <label className="flex flex-col gap-1 text-sm">
            Period
            <select
              className={selectClass}
              value={mode}
              onChange={(e) => setMode(e.target.value as "weekly" | "daily")}
            >
              <option value="weekly">Weekly</option>
              <option value="daily">Daily</option>
            </select>
          </label>
          <label className="flex min-w-60 flex-1 flex-col gap-1 text-sm">
            Send to
            <Input
              type="email"
              placeholder="name@example.com"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
          </label>
          <Button variant="outline" disabled={run.isPending} onClick={() => run.mutate(false)}>
            Preview with real data
          </Button>
          <Button disabled={run.isPending || !email.trim()} onClick={() => run.mutate(true)}>
            Send test
          </Button>
        </div>
        {html && (
          <iframe
            title="Newsletter test preview"
            sandbox=""
            srcDoc={html}
            className="mt-3 h-[48rem] w-full max-w-[680px] rounded border"
          />
        )}
      </details>
    </>
  );
}
