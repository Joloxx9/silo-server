import { useEffectiveSettings } from "@/hooks/queries/settingValues";
import { SETTING_KEYS } from "@/lib/settingsContract";

const KEY = SETTING_KEYS.CATALOG_GROUP_BOX_SETS_IN_LIBRARY;

/** Whether this profile lists a movie library's box sets as single cards. */
export function useGroupBoxSets(): boolean {
  const query = useEffectiveSettings({ keys: [KEY] });
  return query.data?.[KEY]?.value === true;
}
