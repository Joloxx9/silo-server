import {
  settingsCapabilitiesSupportKey,
  useEffectiveSettings,
  useSettingsCapabilities,
} from "@/hooks/queries/settingValues";
import { SETTING_KEYS } from "@/lib/settingsContract";

const MISSING_KEY = SETTING_KEYS.CATALOG_SHOW_MISSING_EPISODES;

/**
 * Whether this profile asked to see the seasons and episodes the library
 * lacks on a series page.
 *
 * Off for every profile that has not opted in, and off against a server that
 * does not know the setting, which would not understand include_missing
 * either.
 */
export function useShowMissingEpisodes(): boolean {
  const capabilitiesQuery = useSettingsCapabilities();
  const isSupported = settingsCapabilitiesSupportKey(capabilitiesQuery.data, MISSING_KEY);
  const query = useEffectiveSettings({ keys: [MISSING_KEY], enabled: isSupported });
  if (!isSupported) return false;
  return query.data?.[MISSING_KEY]?.value === true;
}
