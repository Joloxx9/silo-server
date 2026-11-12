package newsletter

// designedHTMLTemplate renders the layout chosen on the admin page. It reads
// the layout_rows variable layoutVars builds; the words come from t_*
// variables, so one template serves every language.
const designedHTMLTemplate = `<!doctype html>
<html lang="{{lang}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="{{#is_light}}light{{/is_light}}{{^is_light}}dark{{/is_light}}">
<title>{{header_title}}</title>
<style>
  @media only screen and (max-width: 600px) {
    .half { display: block !important; width: 100% !important; padding: 0 !important; }
    .wrap { padding: 16px 8px !important; }
  }
  a { color: {{accent_color}}; }
</style>
</head>
<body style="margin:0;padding:0;background:{{background_color}};color:{{text_color}};font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">
<div style="display:none;max-height:0;overflow:hidden;opacity:0;">{{summary}} · {{server_name}}</div>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:{{background_color}};">
<tr><td align="center" class="wrap" style="padding:24px 12px;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:640px;">

<tr><td style="padding:8px 4px 20px 4px;">
  {{#logo_url}}<img src="{{logo_url}}" alt="{{server_name}}" style="display:block;max-width:240px;max-height:80px;border:0;margin-bottom:14px;">{{/logo_url}}
  {{#eyebrow}}<div style="font-size:13px;letter-spacing:3px;text-transform:uppercase;color:{{accent_color}};font-weight:700;">{{eyebrow}}</div>{{/eyebrow}}
  {{#header_title}}<div style="font-size:28px;line-height:1.25;font-weight:700;color:{{text_color}};margin-top:6px;">{{header_title}}</div>{{/header_title}}
  {{#show_summary}}<div style="font-size:14px;color:{{muted_color}};margin-top:6px;">{{date}} · {{summary}}</div>{{/show_summary}}
  <div style="font-size:15px;line-height:1.55;color:{{muted_color}};margin-top:10px;">{{#greeting}}{{greeting}} {{/greeting}}{{#has_items}}{{intro}}{{/has_items}}{{^has_items}}{{t_recap}}{{/has_items}}</div>
</td></tr>

{{#layout_rows}}
<tr><td style="padding:0 0 8px 0;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0"><tr>
{{#blocks}}<td {{#half}}class="half" width="50%" {{/half}}valign="top" style="padding:{{pad}};">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0">

{{#is_personal}}
<tr><td style="background:{{card_color}};border-radius:14px;padding:20px;">
  <div style="font-size:12px;letter-spacing:2px;text-transform:uppercase;color:{{accent_color}};font-weight:700;">{{#is_daily}}{{t_your_day}}{{/is_daily}}{{^is_daily}}{{t_your_week}}{{/is_daily}} · {{recap_period}}</div>
  {{#personal_has_activity}}
  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="margin-top:12px;"><tr>
    <td width="34%" valign="top" style="padding:4px;"><div style="font-size:22px;line-height:1.2;font-weight:700;color:{{text_color}};">{{personal_watch_time}}</div><div style="font-size:13px;color:{{muted_color}};">{{t_watched}}</div></td>
    <td width="33%" valign="top" style="padding:4px;"><div style="font-size:22px;line-height:1.2;font-weight:700;color:{{text_color}};">{{personal_movies}}</div><div style="font-size:13px;color:{{muted_color}};">{{personal_movies_label}}</div></td>
    <td width="33%" valign="top" style="padding:4px;"><div style="font-size:22px;line-height:1.2;font-weight:700;color:{{text_color}};">{{personal_episodes}}</div><div style="font-size:13px;color:{{muted_color}};">{{personal_episodes_label}}</div></td>
  </tr></table>
  {{#personal_top_show}}<div style="font-size:14px;margin-top:12px;">{{t_top_show}} <strong>{{personal_top_show}}</strong> ({{personal_top_show_label}})</div>{{/personal_top_show}}
  {{#has_personal_recent}}<div style="font-size:13px;color:{{muted_color}};margin-top:8px;">{{t_recently}} {{#personal_recent}}{{name}}{{^is_last}}, {{/is_last}}{{/personal_recent}}</div>{{/has_personal_recent}}
  {{/personal_has_activity}}
  {{^personal_has_activity}}<div style="font-size:15px;margin-top:10px;">{{#is_daily}}{{t_no_plays_day}}{{/is_daily}}{{^is_daily}}{{t_no_plays_week}}{{/is_daily}}</div>{{/personal_has_activity}}
</td></tr>
{{/is_personal}}

{{#is_quiet}}
<tr><td style="background:{{card_color}};border-radius:14px;padding:20px;font-size:15px;line-height:1.55;">{{#is_daily}}{{t_quiet_day}}{{/is_daily}}{{^is_daily}}{{t_quiet_week}}{{/is_daily}}</td></tr>
{{/is_quiet}}

{{#is_popular}}
<tr><td style="padding:16px 4px 8px 4px;font-size:12px;letter-spacing:2px;text-transform:uppercase;color:{{accent_color}};font-weight:700;">{{heading}}</td></tr>
<tr><td style="background:{{card_color}};border-radius:14px;padding:12px 18px;">
  {{#popular}}<div style="font-size:14px;padding:6px 0;"><span style="color:{{accent_color}};font-weight:700;">{{rank}}.</span> {{name}} <span style="color:{{muted_color}};">· {{viewers}} {{viewers_label}}</span></div>{{/popular}}
</td></tr>
{{/is_popular}}

{{#is_catalog}}
<tr><td style="padding:16px 4px 8px 4px;font-size:12px;letter-spacing:2px;text-transform:uppercase;color:{{accent_color}};font-weight:700;">{{heading}}</td></tr>
{{/is_catalog}}

{{#is_list}}{{#items}}
<tr><td style="padding:0 0 10px 0;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:{{card_color}};border-radius:14px;">
<tr>
  {{#poster}}<td width="112" valign="top" style="padding:12px;"><img src="{{poster}}" width="100" alt="" style="display:block;width:100px;border-radius:8px;border:0;"></td>{{/poster}}
  <td valign="top" style="padding:14px 14px 14px {{#poster}}2px{{/poster}}{{^poster}}14px{{/poster}};">
    <div style="font-size:17px;font-weight:700;line-height:1.3;">{{#url}}<a href="{{url}}" style="color:{{text_color}};text-decoration:none;">{{/url}}{{name}}{{#url}}</a>{{/url}}{{#year}} <span style="color:{{muted_color}};font-weight:400;">({{year}})</span>{{/year}}</div>
    {{#badge}}<div style="font-size:13px;color:{{accent_color}};font-weight:600;margin-top:4px;">{{badge}}</div>{{/badge}}
    {{#detail}}<div style="font-size:13px;color:{{muted_color}};margin-top:4px;">{{detail}}</div>{{/detail}}
    {{#has_ratings}}<div style="font-size:13px;margin-top:6px;">{{#imdb_rating}}<span style="background:#f5c518;color:#000000;font-weight:700;border-radius:4px;padding:1px 5px;">IMDb</span> {{imdb_rating}}&nbsp;&nbsp;{{/imdb_rating}}{{#rt_critic}}{{#rt_critic_fresh}}🍅{{/rt_critic_fresh}}{{^rt_critic_fresh}}🤢{{/rt_critic_fresh}} {{rt_critic}}%&nbsp;&nbsp;{{/rt_critic}}{{#rt_audience}}{{#rt_audience_fresh}}🍿{{/rt_audience_fresh}}{{^rt_audience_fresh}}🥤{{/rt_audience_fresh}} {{rt_audience}}%{{/rt_audience}}</div>{{/has_ratings}}
    {{#overview}}<div style="font-size:13px;line-height:1.5;color:{{muted_color}};margin-top:8px;">{{overview}}</div>{{/overview}}
    {{#has_episodes}}<div style="margin-top:8px;">{{#episodes}}<div style="font-size:13px;padding:2px 0;">{{#url}}<a href="{{url}}" style="color:{{text_color}};text-decoration:none;">{{/url}}<span style="color:{{accent_color}};">{{episode_code}}</span> {{episode_name}}{{#url}}</a>{{/url}}{{#runtime}} <span style="color:{{muted_color}};">· {{runtime}}</span>{{/runtime}}</div>{{/episodes}}</div>{{/has_episodes}}
  </td>
</tr>
</table>
</td></tr>
{{/items}}{{/is_list}}

{{#is_grid}}
<tr><td style="padding:0;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="{{gap}}" style="table-layout:fixed;border-collapse:separate;">
{{#grid_rows}}
<tr>
  {{#cells}}
  <td width="{{cell_percent}}" valign="top" style="background:{{card_color}};border-radius:12px;padding:8px;">
    {{#url}}<a href="{{url}}" style="text-decoration:none;color:{{text_color}};">{{/url}}
    {{#poster}}<img src="{{poster}}" width="{{cell_width}}" alt="" style="display:block;width:100%;height:auto;border-radius:8px;border:0;">{{/poster}}
    {{^poster}}<div style="height:{{cell_height}}px;border-radius:8px;background:{{background_color}};"></div>{{/poster}}
    <div style="font-size:14px;font-weight:700;line-height:1.3;color:{{text_color}};margin-top:8px;">{{name}}{{#year}} <span style="color:{{muted_color}};font-weight:400;">({{year}})</span>{{/year}}</div>
    {{#url}}</a>{{/url}}
    {{#badge}}<div style="font-size:12px;color:{{accent_color}};font-weight:600;margin-top:3px;">{{badge}}</div>{{/badge}}
    {{#detail}}<div style="font-size:12px;color:{{muted_color}};margin-top:3px;">{{detail}}</div>{{/detail}}
    {{#has_ratings}}<div style="font-size:12px;margin-top:5px;">{{#imdb_rating}}<span style="background:#f5c518;color:#000000;font-weight:700;border-radius:4px;padding:0 4px;">IMDb</span> {{imdb_rating}} {{/imdb_rating}}{{#rt_critic}}{{#rt_critic_fresh}}🍅{{/rt_critic_fresh}}{{^rt_critic_fresh}}🤢{{/rt_critic_fresh}} {{rt_critic}}% {{/rt_critic}}{{#rt_audience}}{{#rt_audience_fresh}}🍿{{/rt_audience_fresh}}{{^rt_audience_fresh}}🥤{{/rt_audience_fresh}} {{rt_audience}}%{{/rt_audience}}</div>{{/has_ratings}}
  </td>
  {{/cells}}
  {{#fill}}<td width="{{cell_percent}}"></td>{{/fill}}
</tr>
{{/grid_rows}}
</table>
</td></tr>
{{/is_grid}}

{{#is_cards}}{{#items}}
<tr><td style="padding:0 0 14px 0;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:{{card_color}};border-radius:14px;overflow:hidden;">
  <tr><td {{#backdrop_bg}}background="{{backdrop_bg}}" {{/backdrop_bg}}style="padding:20px;background-color:{{card_color}};{{#backdrop_bg}}background-image:url('{{backdrop_bg}}');background-size:cover;background-position:center;{{/backdrop_bg}}">
    {{#poster}}<img src="{{poster}}" alt="" width="112" style="display:block;width:112px;border-radius:8px;border:0;margin-bottom:14px;">{{/poster}}
    <div style="font-size:20px;font-weight:700;line-height:1.3;">{{#url}}<a href="{{url}}" style="color:{{text_color}};text-decoration:none;">{{/url}}{{name}}{{#url}}</a>{{/url}}{{#year}} <span style="font-weight:400;color:{{muted_color}};">({{year}})</span>{{/year}}</div>
    {{#detail}}<div style="font-size:13px;color:{{muted_color}};margin-top:4px;">{{detail}}</div>{{/detail}}
    {{#has_ratings}}<div style="font-size:13px;margin-top:8px;">{{#imdb_rating}}<span style="background:#f5c518;color:#000000;font-weight:700;border-radius:4px;padding:1px 5px;">IMDb</span> {{imdb_rating}}&nbsp;&nbsp;{{/imdb_rating}}{{#rt_critic}}{{#rt_critic_fresh}}🍅{{/rt_critic_fresh}}{{^rt_critic_fresh}}🤢{{/rt_critic_fresh}} {{rt_critic}}%&nbsp;&nbsp;{{/rt_critic}}{{#rt_audience}}{{#rt_audience_fresh}}🍿{{/rt_audience_fresh}}{{^rt_audience_fresh}}🥤{{/rt_audience_fresh}} {{rt_audience}}%{{/rt_audience}}</div>{{/has_ratings}}
    {{#badge}}<div style="display:inline-block;margin-top:10px;padding:4px 10px;border-radius:999px;background:{{accent_color}};color:{{accent_text_color}};font-size:12px;font-weight:700;">{{badge}}</div>{{/badge}}
    {{#tagline}}<div style="font-size:14px;font-style:italic;color:{{accent_color}};margin-top:10px;">{{tagline}}</div>{{/tagline}}
    {{#overview}}<div style="font-size:14px;line-height:1.55;margin-top:10px;">{{overview}}</div>{{/overview}}
    {{#cast}}<div style="font-size:12px;color:{{muted_color}};margin-top:10px;">{{t_starring}} {{cast}}</div>{{/cast}}
  </td></tr>
  {{#has_episodes}}<tr><td style="padding:0 20px 18px 20px;">
    {{#episodes}}
    <div style="border-top:1px solid {{divider_color}};padding-top:12px;margin-top:12px;">
      <div style="font-size:15px;font-weight:700;">{{#url}}<a href="{{url}}" style="color:{{text_color}};text-decoration:none;">{{/url}}<span style="color:{{accent_color}};">{{episode_code}}</span> · {{episode_name}}{{#url}}</a>{{/url}}</div>
      <div style="font-size:12px;color:{{muted_color}};margin-top:3px;">{{#air_date}}{{t_aired}} {{air_date}}{{/air_date}}{{#runtime}} · {{runtime}}{{/runtime}}</div>
      {{#episode_synopsis}}<div style="font-size:14px;line-height:1.55;margin-top:6px;">{{episode_synopsis}}</div>{{/episode_synopsis}}
    </div>
    {{/episodes}}
  </td></tr>{{/has_episodes}}
</table>
</td></tr>
{{/items}}{{/is_cards}}

</table>
</td>{{/blocks}}
</tr></table>
</td></tr>
{{/layout_rows}}

{{#has_more}}
<tr><td style="padding:4px 4px 12px 4px;font-size:14px;color:{{muted_color}};">{{t_and_more}} {{more_count}} {{t_more}}</td></tr>
{{/has_more}}

{{#show_button}}
<tr><td align="center" style="padding:24px 0 8px 0;">
  <a href="{{server_url}}" style="display:inline-block;background:{{accent_color}};color:{{accent_text_color}};text-decoration:none;font-weight:700;font-size:15px;padding:12px 28px;border-radius:999px;">{{button_label}}</a>
</td></tr>
{{/show_button}}

<tr><td align="center" style="padding:20px 4px 0 4px;font-size:12px;line-height:1.6;color:{{muted_color}};">
  {{footer_text}}<br>
  {{#manage_url}}<a href="{{manage_url}}" style="color:{{muted_color}};">{{t_manage}}</a>{{/manage_url}}{{#manage_url}}{{#unsubscribe_url}} · {{/unsubscribe_url}}{{/manage_url}}{{#unsubscribe_url}}<a href="{{unsubscribe_url}}" style="color:{{muted_color}};">{{t_unsubscribe}}</a>{{/unsubscribe_url}}
</td></tr>

</table>
</td></tr>
</table>
</body>
</html>
`
