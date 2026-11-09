package newsletter

const defaultSubjectTemplate = `{{#is_digest}}{{#has_items}}{{server_name}} {{#is_daily}}daily{{/is_daily}}{{#is_weekly}}weekly{{/is_weekly}} digest: {{summary}}{{/has_items}}{{^has_items}}Your {{#is_daily}}day{{/is_daily}}{{#is_weekly}}week{{/is_weekly}} on {{server_name}}{{/has_items}}{{/is_digest}}{{^is_digest}}{{#single_item}}New on {{server_name}}: {{first_display_title}}{{/single_item}}{{^single_item}}New on {{server_name}}: {{summary}}{{/single_item}}{{/is_digest}}`

const defaultTextTemplate = `{{header_title}}
{{date}}

{{#recipient_first_name}}Hi {{recipient_first_name}},{{/recipient_first_name}}
{{#has_items}}{{intro}} {{summary}}.{{/has_items}}{{^has_items}}Here's your recap from {{server_name}}.{{/has_items}}

{{#has_personal}}
==== {{#is_daily}}Your day{{/is_daily}}{{^is_daily}}Your week{{/is_daily}} ({{recap_period}}) ====
{{#personal_has_activity}}
Watched {{personal_watch_time}}: {{personal_movies}} {{personal_movies_label}}, {{personal_episodes}} {{personal_episodes_label}}
{{#personal_top_show}}Most watched show: {{personal_top_show}} ({{personal_top_show_label}}){{/personal_top_show}}
{{#has_personal_recent}}Recently: {{#personal_recent}}{{name}}{{^is_last}}, {{/is_last}}{{/personal_recent}}{{/has_personal_recent}}
{{/personal_has_activity}}
{{^personal_has_activity}}Nothing watched yet. Here's something to start with.{{/personal_has_activity}}
{{/has_personal}}

{{#has_shows}}
==== TV ====
{{#shows}}

{{show_name}}{{#network}} ({{network}}){{/network}}
{{#show_url}}{{show_url}}{{/show_url}}
{{episodes_label}}: {{episode_range}}
{{#show_synopsis}}{{show_synopsis}}{{/show_synopsis}}
{{#episodes}}
  - {{episode_code}} {{episode_name}}{{#runtime}} [{{runtime}}]{{/runtime}}
    {{#url}}{{url}}{{/url}}
    {{#episode_synopsis}}{{episode_synopsis}}{{/episode_synopsis}}
{{/episodes}}
{{/shows}}
{{/has_shows}}

{{#has_movies}}
==== Movies ====
{{#movies}}

{{title}}{{#year}} ({{year}}){{/year}}
{{#url}}{{url}}{{/url}}
{{#genres}}{{genres}}{{/genres}}{{#runtime}} · {{runtime}}{{/runtime}}{{#rating}} · ★ {{rating}}{{/rating}}
{{#synopsis}}{{synopsis}}{{/synopsis}}
{{/movies}}
{{/has_movies}}

{{#has_more}}...and {{more_count}} more on {{server_url}}{{/has_more}}

{{#has_popular}}
==== Popular on {{server_name}} ====
{{#popular}}
{{rank}}. {{name}} ({{viewers}} {{viewers_label}})
{{/popular}}
{{/has_popular}}

--
{{footer_text}}
{{#manage_url}}Manage your alerts: {{manage_url}}{{/manage_url}}
{{#unsubscribe_url}}Unsubscribe: {{unsubscribe_url}}{{/unsubscribe_url}}
`

const defaultHTMLTemplate = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="dark light">
<title>{{header_title}}</title>
<style>
  @media (max-width: 600px) {
    .stack { display: block !important; width: 100% !important; }
    .poster-cell { padding: 0 0 14px 0 !important; }
    .wrap { padding: 12px !important; }
  }
  a { color: {{accent_color}}; }
</style>
</head>
<body style="margin:0;padding:0;background:{{background_color}};color:{{text_color}};font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
<div style="display:none;max-height:0;overflow:hidden;opacity:0;">{{summary}} on {{server_name}}.</div>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:{{background_color}};">
<tr><td align="center" class="wrap" style="padding:28px 16px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:640px;">

  <!-- Header -->
  <tr><td style="padding:0 4px 22px 4px;">
    {{#logo_url}}<img src="{{logo_url}}" alt="{{server_name}}" height="36" style="display:block;height:36px;border:0;margin-bottom:18px;">{{/logo_url}}
    {{^logo_url}}<div style="font-size:13px;letter-spacing:2px;text-transform:uppercase;color:{{accent_color}};font-weight:700;margin-bottom:10px;">{{server_name}}</div>{{/logo_url}}
    <div style="font-size:26px;line-height:1.25;font-weight:700;color:{{text_color}};">{{header_title}}</div>
    <div style="font-size:14px;color:{{muted_color}};margin-top:6px;">{{date}} · {{summary}}</div>
    <div style="font-size:15px;line-height:1.55;color:{{text_color}};margin-top:16px;">
      {{#recipient_first_name}}Hi {{recipient_first_name}}, {{/recipient_first_name}}{{#has_items}}{{intro}}{{/has_items}}{{^has_items}}here's your recap from {{server_name}}.{{/has_items}}
    </div>
  </td></tr>

  {{#has_personal}}
  <tr><td style="padding:0 0 18px 0;">
    <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:{{card_color}};border-radius:14px;">
      <tr><td style="padding:20px;">
        <div style="font-size:12px;letter-spacing:1.5px;text-transform:uppercase;color:{{accent_color}};font-weight:700;">{{#is_daily}}Your day{{/is_daily}}{{^is_daily}}Your week{{/is_daily}} · {{recap_period}}</div>
        {{#personal_has_activity}}
        <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin-top:12px;"><tr>
          <td width="34%" valign="top" style="padding:4px;"><div style="font-size:22px;font-weight:700;">{{personal_watch_time}}</div><div style="font-size:13px;color:{{muted_color}};">watched</div></td>
          <td width="33%" valign="top" style="padding:4px;"><div style="font-size:22px;font-weight:700;">{{personal_movies}}</div><div style="font-size:13px;color:{{muted_color}};">{{personal_movies_label}}</div></td>
          <td width="33%" valign="top" style="padding:4px;"><div style="font-size:22px;font-weight:700;">{{personal_episodes}}</div><div style="font-size:13px;color:{{muted_color}};">{{personal_episodes_label}}</div></td>
        </tr></table>
        {{#personal_top_show}}<div style="font-size:14px;margin-top:12px;">Most watched show: <b>{{personal_top_show}}</b> ({{personal_top_show_label}})</div>{{/personal_top_show}}
        {{#has_personal_recent}}<div style="font-size:13px;color:{{muted_color}};margin-top:8px;">Recently: {{#personal_recent}}{{name}}{{^is_last}}, {{/is_last}}{{/personal_recent}}</div>{{/has_personal_recent}}
        {{/personal_has_activity}}
        {{^personal_has_activity}}<div style="font-size:14px;color:{{muted_color}};margin-top:10px;">Nothing watched {{#is_daily}}today{{/is_daily}}{{^is_daily}}this week{{/is_daily}}. Here's something to start with.</div>{{/personal_has_activity}}
      </td></tr>
    </table>
  </td></tr>
  {{/has_personal}}

  {{#is_digest}}{{^has_items}}
  <tr><td style="padding:0 0 18px 0;"><div style="background:{{card_color}};border-radius:14px;padding:20px;font-size:15px;line-height:1.55;">Nothing new arrived {{#is_daily}}today{{/is_daily}}{{^is_daily}}this week{{/is_daily}}. A good moment to catch up!</div></td></tr>
  {{/has_items}}{{/is_digest}}

  {{#has_shows}}
  <tr><td style="padding:6px 4px 10px 4px;font-size:12px;letter-spacing:1.5px;text-transform:uppercase;color:{{muted_color}};font-weight:700;">TV</td></tr>
  {{#shows}}
  <tr><td style="padding:0 0 18px 0;">
    <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:{{card_color}};border-radius:14px;overflow:hidden;">
      <tr><td {{#show_backdrop_bg}}background="{{show_backdrop_bg}}" {{/show_backdrop_bg}}style="padding:20px;background-color:{{card_color}};{{#show_backdrop_bg}}background-image:url('{{show_backdrop_bg}}');background-size:cover;background-position:center;{{/show_backdrop_bg}}">
        <table role="presentation" width="100%" cellpadding="0" cellspacing="0"><tr>
          {{#show_poster}}<td class="stack poster-cell" width="112" valign="top" style="padding-right:18px;"><img src="{{show_poster}}" alt="{{show_name}}" width="112" style="display:block;width:112px;border-radius:8px;border:0;"></td>{{/show_poster}}
          <td class="stack" valign="top">
            <div style="font-size:20px;font-weight:700;line-height:1.3;">{{#show_url}}<a href="{{show_url}}" style="color:{{text_color}};text-decoration:none;">{{/show_url}}{{show_name}}{{#show_url}}</a>{{/show_url}}</div>
            <div style="font-size:13px;color:{{muted_color}};margin-top:4px;">{{#network}}{{network}} · {{/network}}{{#show_genres}}{{show_genres}} · {{/show_genres}}{{#show_rating}}★ {{show_rating}}{{/show_rating}}</div>
            {{#show_has_ratings}}<div style="font-size:13px;margin-top:8px;">{{#show_imdb_rating}}<span style="background:#f5c518;color:#000000;font-weight:700;border-radius:4px;padding:1px 5px;">IMDb</span> {{show_imdb_rating}}&nbsp;&nbsp;{{/show_imdb_rating}}{{#show_rt_critic}}{{#show_rt_critic_fresh}}🍅{{/show_rt_critic_fresh}}{{^show_rt_critic_fresh}}🤢{{/show_rt_critic_fresh}} {{show_rt_critic}}%&nbsp;&nbsp;{{/show_rt_critic}}{{#show_rt_audience}}{{#show_rt_audience_fresh}}🍿{{/show_rt_audience_fresh}}{{^show_rt_audience_fresh}}🥤{{/show_rt_audience_fresh}} {{show_rt_audience}}%{{/show_rt_audience}}</div>{{/show_has_ratings}}
            <div style="display:inline-block;margin-top:10px;padding:4px 10px;border-radius:999px;background:{{accent_color}};color:#ffffff;font-size:12px;font-weight:700;">{{episodes_label}} · {{episode_range}}</div>
            {{#show_synopsis}}<div style="font-size:14px;line-height:1.55;color:{{#show_backdrop_bg}}{{text_color}}{{/show_backdrop_bg}}{{^show_backdrop_bg}}{{muted_color}}{{/show_backdrop_bg}};margin-top:12px;">{{show_synopsis}}</div>{{/show_synopsis}}
          </td>
        </tr></table>
      </td></tr>
      <tr><td style="padding:0 20px 20px 20px;">

        {{#episodes}}
        <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin-top:16px;border-top:1px solid rgba(255,255,255,0.08);">
          <tr><td style="padding-top:14px;">
            <div style="font-size:15px;font-weight:700;">{{#url}}<a href="{{url}}" style="color:{{text_color}};text-decoration:none;">{{/url}}<span style="color:{{accent_color}};">{{episode_code}}</span> · {{episode_name}}{{#url}}</a>{{/url}}</div>
            <div style="font-size:12px;color:{{muted_color}};margin-top:3px;">{{#air_date}}Aired {{air_date}}{{/air_date}}{{#runtime}} · {{runtime}}{{/runtime}}</div>
            {{#episode_synopsis}}<div style="font-size:14px;line-height:1.55;color:{{text_color}};opacity:0.88;margin-top:8px;">{{episode_synopsis}}</div>{{/episode_synopsis}}
          </td></tr>
        </table>
        {{/episodes}}

      </td></tr>
    </table>
  </td></tr>
  {{/shows}}
  {{/has_shows}}

  {{#has_movies}}
  <tr><td style="padding:6px 4px 10px 4px;font-size:12px;letter-spacing:1.5px;text-transform:uppercase;color:{{muted_color}};font-weight:700;">Movies</td></tr>
  {{#movies}}
  <tr><td style="padding:0 0 18px 0;">
    <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:{{card_color}};border-radius:14px;overflow:hidden;">
      <tr><td {{#backdrop_bg}}background="{{backdrop_bg}}" {{/backdrop_bg}}style="padding:20px;background-color:{{card_color}};{{#backdrop_bg}}background-image:url('{{backdrop_bg}}');background-size:cover;background-position:center;{{/backdrop_bg}}">
        <table role="presentation" width="100%" cellpadding="0" cellspacing="0"><tr>
          {{#poster}}<td class="stack poster-cell" width="112" valign="top" style="padding-right:18px;"><img src="{{poster}}" alt="{{title}}" width="112" style="display:block;width:112px;border-radius:8px;border:0;"></td>{{/poster}}
          <td class="stack" valign="top">
            <div style="font-size:20px;font-weight:700;line-height:1.3;">{{#url}}<a href="{{url}}" style="color:{{text_color}};text-decoration:none;">{{/url}}{{title}}{{#year}} <span style="font-weight:400;color:{{muted_color}};">({{year}})</span>{{/year}}{{#url}}</a>{{/url}}</div>
            <div style="font-size:13px;color:{{muted_color}};margin-top:4px;">{{#genres}}{{genres}}{{/genres}}{{#runtime}} · {{runtime}}{{/runtime}}{{#content_rating}} · {{content_rating}}{{/content_rating}}{{#rating}} · ★ {{rating}}{{/rating}}</div>
            {{#has_ratings}}<div style="font-size:13px;margin-top:8px;">{{#imdb_rating}}<span style="background:#f5c518;color:#000000;font-weight:700;border-radius:4px;padding:1px 5px;">IMDb</span> {{imdb_rating}}&nbsp;&nbsp;{{/imdb_rating}}{{#rt_critic}}{{#rt_critic_fresh}}🍅{{/rt_critic_fresh}}{{^rt_critic_fresh}}🤢{{/rt_critic_fresh}} {{rt_critic}}%&nbsp;&nbsp;{{/rt_critic}}{{#rt_audience}}{{#rt_audience_fresh}}🍿{{/rt_audience_fresh}}{{^rt_audience_fresh}}🥤{{/rt_audience_fresh}} {{rt_audience}}%{{/rt_audience}}</div>{{/has_ratings}}
            {{#tagline}}<div style="font-size:14px;font-style:italic;color:{{accent_color}};margin-top:10px;">{{tagline}}</div>{{/tagline}}
            {{#synopsis}}<div style="font-size:14px;line-height:1.55;color:{{text_color}};opacity:0.88;margin-top:10px;">{{synopsis}}</div>{{/synopsis}}
            {{#cast}}<div style="font-size:12px;color:{{muted_color}};margin-top:10px;">Starring {{cast}}</div>{{/cast}}
          </td>
        </tr></table>
      </td></tr>
    </table>
  </td></tr>
  {{/movies}}
  {{/has_movies}}

  {{#has_more}}
  <tr><td style="padding:4px 4px 18px 4px;font-size:14px;color:{{muted_color}};">…and {{more_count}} more. {{#server_url}}<a href="{{server_url}}" style="color:{{accent_color}};">Browse {{server_name}}</a>{{/server_url}}</td></tr>
  {{/has_more}}

  {{#has_popular}}
  <tr><td style="padding:6px 4px 10px 4px;font-size:12px;letter-spacing:1.5px;text-transform:uppercase;color:{{muted_color}};font-weight:700;">Popular on {{server_name}}</td></tr>
  <tr><td style="padding:0 0 18px 0;">
    <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:{{card_color}};border-radius:14px;"><tr><td style="padding:12px 20px;">
      {{#popular}}<div style="font-size:14px;padding:6px 0;"><span style="color:{{accent_color}};font-weight:700;">{{rank}}.</span> {{name}} <span style="color:{{muted_color}};">· {{viewers}} {{viewers_label}}</span></div>{{/popular}}
    </td></tr></table>
  </td></tr>
  {{/has_popular}}

  <!-- Footer -->
  <tr><td style="padding:18px 4px 0 4px;border-top:1px solid rgba(255,255,255,0.08);font-size:12px;line-height:1.6;color:{{muted_color}};">
    {{footer_text}}<br>
    {{#manage_url}}<a href="{{manage_url}}" style="color:{{muted_color}};">Manage alerts</a>{{/manage_url}}{{#manage_url}}{{#unsubscribe_url}} · {{/unsubscribe_url}}{{/manage_url}}{{#unsubscribe_url}}<a href="{{unsubscribe_url}}" style="color:{{muted_color}};">Unsubscribe</a>{{/unsubscribe_url}}
  </td></tr>

</table>
</td></tr>
</table>
</body>
</html>
`

// verifyHTML is the fixed address-confirmation email. It uses the same theme
// colours as the alert template.
const verifyHTML = `<!doctype html>
<html><body style="margin:0;padding:32px 16px;background:{{background_color}};color:{{text_color}};font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0"><tr><td align="center">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:520px;background:{{card_color}};border-radius:14px;"><tr><td style="padding:28px;">
<div style="font-size:13px;letter-spacing:2px;text-transform:uppercase;color:{{accent_color}};font-weight:700;">{{server_name}}</div>
<div style="font-size:22px;font-weight:700;margin-top:10px;">Confirm your email address</div>
<div style="font-size:15px;line-height:1.55;margin-top:12px;color:{{muted_color}};">{{#recipient_first_name}}Hi {{recipient_first_name}}, {{/recipient_first_name}}confirm this address to start getting new media alerts from {{server_name}}.</div>
<table role="presentation" cellpadding="0" cellspacing="0" style="margin-top:22px;"><tr><td style="border-radius:8px;background:{{accent_color}};"><a href="{{verify_url}}" style="display:inline-block;padding:12px 22px;font-size:15px;font-weight:700;color:#ffffff;text-decoration:none;">Confirm address</a></td></tr></table>
<div style="font-size:12px;line-height:1.6;margin-top:22px;color:{{muted_color}};">If you didn't ask for this, ignore this email and nothing will be sent.</div>
</td></tr></table></td></tr></table></body></html>`

const verifyText = `Confirm your email address

{{#recipient_first_name}}Hi {{recipient_first_name}}, {{/recipient_first_name}}confirm this address to start getting new media alerts from {{server_name}}:

{{verify_url}}

If you didn't ask for this, ignore this email and nothing will be sent.
`
