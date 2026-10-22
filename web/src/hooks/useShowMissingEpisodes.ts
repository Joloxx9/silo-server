import { useEffectiveSettings } from "@/hooks/queries/settingValues";
import { SETTING_KEYS } from "@/lib/settingsContract";

const MISSING_KEY = SETTING_KEYS.CATALOG_SHOW_MISSING_EPISODES;

/**
 * Whether this profile asked to see the seasons and episodes the library
 * lacks on a series page. Off until the profile turns it on.
 *
 * No capability check: this web client ships with the server that defines
 * the setting, so the server always knows it.
 */
export function useShowMissingEpisodes(): boolean {
  const query = useEffectiveSettings({ keys: [MISSING_KEY] });
  return query.data?.[MISSING_KEY]?.value === true;
}
