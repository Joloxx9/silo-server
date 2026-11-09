import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { v2, type V2Result } from "@/api/v2/request";
import {
  captureNotificationAuthority,
  notificationScope,
  requireNotificationAuthority,
} from "@/api/v2/notifications";
import { SettingsGroup } from "@/components/settings/SettingsGroup";
import { Switch } from "@/components/ui/switch";
import { toast } from "sonner";

type Preferences = V2Result<"GET /api/v2/notifications/delivery-preferences">;

export function DeliveryPreferencesSection() {
  const authority = captureNotificationAuthority();
  const scope = notificationScope(authority);
  const key = ["notification-delivery-preferences", scope];
  const client = useQueryClient();
  const capability = useQuery({
    queryKey: ["newsletter-capability", scope],
    queryFn: () =>
      v2("GET /api/v2/notifications/newsletter/capability", { profileContext: authority }),
    retry: false,
  });
  const query = useQuery({
    queryKey: key,
    enabled: capability.data?.preferences === true,
    queryFn: async () => {
      requireNotificationAuthority(authority);
      const result = await v2("GET /api/v2/notifications/delivery-preferences", {
        profileContext: authority,
      });
      requireNotificationAuthority(authority);
      return result;
    },
  });
  const save = useMutation({
    mutationFn: async (body: Preferences) => {
      requireNotificationAuthority(authority);
      const result = await v2("PUT /api/v2/notifications/delivery-preferences", {
        body,
        profileContext: authority,
        retryAuthentication: false,
      });
      requireNotificationAuthority(authority);
      return result;
    },
    onSuccess: (result) => client.setQueryData(key, result),
    onError: (error) => toast.error(error.message),
  });
  if (!capability.data?.preferences) return null;
  if (query.isError)
    return (
      <p role="alert">
        Could not load delivery preferences.{" "}
        <button onClick={() => void query.refetch()}>Retry</button>
      </p>
    );
  const prefs = query.data;
  if (!prefs) return <p>Loading delivery preferences…</p>;
  const update = (patch: Partial<Preferences>) => save.mutate({ ...prefs, ...patch });
  const toggle = (
    field:
      | "push"
      | "push_episodes"
      | "push_requests"
      | "push_other"
      | "movies"
      | "episodes"
      | "personal"
      | "popular",
    label: string,
  ) => (
    <label key={field} className="flex items-center justify-between gap-4">
      <span>{label}</span>
      <Switch
        checked={prefs[field]}
        disabled={save.isPending}
        onCheckedChange={(value) => update({ [field]: value })}
      />
    </label>
  );
  return (
    <>
      <SettingsGroup
        title="Push preferences"
        description="Shared across your devices. Each device also needs permission from its operating system."
      >
        {toggle("push", "Receive push notifications")}
        {toggle("push_episodes", "New episodes of shows you follow")}
        {toggle("push_requests", "Updates to your requests")}
        {toggle("push_other", "Other notifications")}
      </SettingsGroup>
      <SettingsGroup
        title="New releases and newsletter"
        description="Uses your verified notification email address below. Only titles available to this profile are included."
      >
        {!capability.data.available && (
          <p className="text-muted-foreground">
            Email alerts and newsletters are currently unavailable. Your choices are kept.
          </p>
        )}
        <label className="flex items-center justify-between gap-4">
          New-release alerts
          <select
            className="bg-background rounded border p-2"
            value={prefs.alerts}
            disabled={save.isPending}
            onChange={(e) => update({ alerts: e.target.value as Preferences["alerts"] })}
          >
            <option value="off">Off</option>
            <option value="instant">As new titles arrive</option>
            <option value="daily">Daily summary</option>
          </select>
        </label>
        <label className="flex items-center justify-between gap-4">
          Newsletter
          <select
            className="bg-background rounded border p-2"
            value={prefs.newsletter}
            disabled={save.isPending}
            onChange={(e) => update({ newsletter: e.target.value as Preferences["newsletter"] })}
          >
            <option value="off">Off</option>
            <option value="daily">Daily</option>
            <option value="weekly">Weekly</option>
          </select>
        </label>
        {toggle("movies", "New movies")}
        {toggle("episodes", "New episodes")}
        {toggle("personal", "My viewing summary")}
        {toggle("popular", "Popular titles")}
        <p className="text-muted-foreground text-sm">
          Send times and newsletter design are set by the administrator. You can unsubscribe at any
          time.
        </p>
      </SettingsGroup>
    </>
  );
}
