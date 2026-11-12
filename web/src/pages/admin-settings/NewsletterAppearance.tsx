import { ArrowDown, ArrowUp } from "lucide-react";
import type { V2Result } from "@/api/v2/request";
import { Switch } from "@/components/ui/switch";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";

type Config = V2Result<"GET /api/v2/admin/notifications/newsletter">;
type Content = Config["content"];
type Theme = Content["theme"];
type Layout = Content["layout"];
type Section = Layout["sections"][number];

const themes: Record<
  string,
  Pick<Theme, "background_color" | "card_color" | "text_color" | "muted_color" | "accent_color">
> = {
  Dark: {
    background_color: "#0d0f14",
    card_color: "#161a23",
    text_color: "#f2f3f5",
    muted_color: "#a9afbf",
    accent_color: "#8fa3ff",
  },
  Light: {
    background_color: "#eef0f4",
    card_color: "#ffffff",
    text_color: "#111318",
    muted_color: "#565c6b",
    accent_color: "#4253e0",
  },
  Midnight: {
    background_color: "#0f1115",
    card_color: "#181b22",
    text_color: "#f2f3f5",
    muted_color: "#9aa0ab",
    accent_color: "#7c5cff",
  },
  Warm: {
    background_color: "#f6f1ea",
    card_color: "#fffdf9",
    text_color: "#2b211a",
    muted_color: "#6f6255",
    accent_color: "#c2571a",
  },
};

const sectionNames: Record<string, string> = {
  personal: "Your week (personal summary)",
  movies: "New movies",
  shows: "New episodes and shows",
  popular: "Popular on the server",
};

const selectClass = "bg-background block w-full rounded border p-2";

export function NewsletterAppearance({
  content,
  change,
}: {
  content: Content;
  change: (content: Partial<Content>) => void;
}) {
  const { theme, layout, recap } = content;
  const setTheme = (patch: Partial<Theme>) => change({ theme: { ...theme, ...patch } });
  const setLayout = (patch: Partial<Layout>) => change({ layout: { ...layout, ...patch } });
  const setSection = (index: number, patch: Partial<Section>) =>
    setLayout({ sections: layout.sections.map((s, i) => (i === index ? { ...s, ...patch } : s)) });
  const move = (index: number, by: number) => {
    const sections = [...layout.sections];
    const [section] = sections.splice(index, 1);
    sections.splice(index + by, 0, section!);
    setLayout({ sections });
  };
  // The personal and popular sections are the recap switches: turning them
  // off also stops reading the data behind them.
  const enabled = (s: Section) =>
    s.id === "personal" ? recap.personal : s.id === "popular" ? recap.popular : s.enabled;
  const setEnabled = (index: number, s: Section, value: boolean) => {
    if (s.id === "personal" || s.id === "popular") change({ recap: { ...recap, [s.id]: value } });
    else setSection(index, { enabled: value });
  };

  return (
    <div className="space-y-6">
      {layout.custom_html && (
        <p className="text-muted-foreground text-sm" role="status">
          Your own HTML template is on (Email templates below), so the sections and details here do
          not change the email. Colors and texts still fill its variables.
        </p>
      )}
      <div>
        <h4 className="mb-2 font-medium">Colors</h4>
        <div className="mb-3 flex flex-wrap gap-2">
          {Object.entries(themes).map(([name, colors]) => (
            <Button key={name} variant="outline" size="sm" onClick={() => setTheme(colors)}>
              <span
                aria-hidden
                className="mr-2 inline-block size-3 rounded-full border"
                style={{ background: colors.background_color, borderColor: colors.accent_color }}
              />
              {name}
            </Button>
          ))}
        </div>
        <div className="grid gap-3 sm:grid-cols-5">
          {(
            [
              ["background_color", "Background"],
              ["card_color", "Cards"],
              ["text_color", "Text"],
              ["muted_color", "Secondary text"],
              ["accent_color", "Accent"],
            ] as const
          ).map(([field, label]) => (
            <label key={field} className="text-sm">
              {label}
              <span className="mt-1 flex items-center gap-2">
                <input
                  type="color"
                  className="h-9 w-10 cursor-pointer rounded border"
                  value={/^#[0-9a-f]{6}$/i.test(theme[field]) ? theme[field] : "#000000"}
                  onChange={(e) => setTheme({ [field]: e.target.value })}
                />
                <Input
                  aria-label={`${label} hex`}
                  className="font-mono"
                  value={theme[field]}
                  onChange={(e) => setTheme({ [field]: e.target.value })}
                />
              </span>
            </label>
          ))}
        </div>
      </div>

      <div>
        <h4 className="mb-2 font-medium">Header and footer</h4>
        <p className="text-muted-foreground mb-3 text-sm">
          Texts may use variables such as {"{{server_name}}"} and {"{{recipient_first_name}}"}.
          Leave a field empty to hide it.
        </p>
        <div className="grid gap-3 sm:grid-cols-2">
          <label>
            Logo URL
            <Input
              value={theme.logo_url}
              placeholder="https://…/logo.png"
              onChange={(e) => setTheme({ logo_url: e.target.value })}
            />
          </label>
          <label>
            Small line above the heading
            <Input
              value={layout.eyebrow}
              onChange={(e) => setLayout({ eyebrow: e.target.value })}
            />
          </label>
          <label>
            Heading
            <Input
              value={theme.header_title}
              onChange={(e) => setTheme({ header_title: e.target.value })}
            />
          </label>
          <label>
            Greeting
            <Input
              value={layout.greeting}
              onChange={(e) => setLayout({ greeting: e.target.value })}
            />
          </label>
          <label className="sm:col-span-2">
            Introduction
            <Input value={theme.intro} onChange={(e) => setTheme({ intro: e.target.value })} />
          </label>
          <label>
            Button label
            <Input
              value={theme.button_text}
              onChange={(e) => setTheme({ button_text: e.target.value })}
            />
          </label>
          <label>
            Footer
            <Input
              value={theme.footer_text}
              onChange={(e) => setTheme({ footer_text: e.target.value })}
            />
          </label>
        </div>
      </div>

      <div>
        <h4 className="mb-2 font-medium">Sections</h4>
        <p className="text-muted-foreground mb-3 text-sm">
          Order them with the arrows. Two half-width sections in a row sit side by side and stack on
          phones.
        </p>
        <ol className="space-y-2">
          {layout.sections.map((section, i) => {
            const catalog = section.id === "movies" || section.id === "shows";
            return (
              <li key={section.id} className="rounded border p-3">
                <div className="flex flex-wrap items-center gap-3">
                  <Switch
                    aria-label={`Show ${sectionNames[section.id]}`}
                    checked={enabled(section)}
                    onCheckedChange={(value) => setEnabled(i, section, value)}
                  />
                  <span className="min-w-0 flex-1 font-medium">{sectionNames[section.id]}</span>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label="Move up"
                    disabled={i === 0}
                    onClick={() => move(i, -1)}
                  >
                    <ArrowUp className="size-4" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label="Move down"
                    disabled={i === layout.sections.length - 1}
                    onClick={() => move(i, 1)}
                  >
                    <ArrowDown className="size-4" />
                  </Button>
                </div>
                <div className="mt-3 grid gap-3 sm:grid-cols-3">
                  {catalog && (
                    <label className="text-sm">
                      Style
                      <select
                        className={selectClass}
                        value={section.style}
                        onChange={(e) => setSection(i, { style: e.target.value })}
                      >
                        <option value="list">List with small posters</option>
                        <option value="grid">Poster grid</option>
                        <option value="cards">Large cards</option>
                      </select>
                    </label>
                  )}
                  {catalog && section.style === "grid" && (
                    <label className="text-sm">
                      Posters per row
                      <select
                        className={selectClass}
                        value={section.columns}
                        onChange={(e) => setSection(i, { columns: Number(e.target.value) })}
                      >
                        {[2, 3, 4, 5].map((n) => (
                          <option key={n} value={n}>
                            {n}
                          </option>
                        ))}
                      </select>
                    </label>
                  )}
                  <label className="text-sm">
                    Width
                    <select
                      className={selectClass}
                      value={section.width}
                      onChange={(e) => setSection(i, { width: e.target.value })}
                    >
                      <option value="full">Full width</option>
                      <option value="half">Half width</option>
                    </select>
                  </label>
                  {section.id === "popular" && (
                    <label className="text-sm">
                      Titles to list
                      <Input
                        type="number"
                        min={1}
                        max={20}
                        value={recap.popular_count}
                        onChange={(e) =>
                          change({ recap: { ...recap, popular_count: Number(e.target.value) } })
                        }
                      />
                    </label>
                  )}
                </div>
              </li>
            );
          })}
        </ol>
      </div>

      <div>
        <h4 className="mb-2 font-medium">Details</h4>
        <div className="grid gap-x-6 sm:grid-cols-2">
          {(
            [
              ["show_summary", "Date and count of new titles under the heading"],
              ["show_year", "Year after the title"],
              ["show_detail", "Genres, runtime and network"],
              ["show_ratings", "IMDb and Rotten Tomatoes ratings"],
              ["show_overview", "Synopsis"],
              ["show_episodes", "Episode list under each show"],
              ["show_button", "Button to open the server"],
            ] as const
          ).map(([field, label]) => (
            <label key={field} className="my-2 flex items-center justify-between gap-4">
              {label}
              <Switch
                checked={layout[field]}
                onCheckedChange={(value) => setLayout({ [field]: value })}
              />
            </label>
          ))}
          <label className="my-2 flex items-center justify-between gap-4">
            Send a viewing summary when no new titles arrived
            <Switch
              checked={recap.weekly_when_empty}
              onCheckedChange={(weekly_when_empty) =>
                change({ recap: { ...recap, weekly_when_empty } })
              }
            />
          </label>
        </div>
      </div>
    </div>
  );
}
