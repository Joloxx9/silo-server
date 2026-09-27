# Media requests

A user asks for a movie or series the server does not have, an admin approves it
(or the user's policy approves it automatically), a request-router plugin sends it
to a downstream service such as Sonarr or Radarr, and the request completes when
the media is in the library. The code lives in `internal/requests`; the reconcile
pass is the `reconcile_requests` task in `internal/taskmanager/tasks`.

## State

A request row carries two fields:

- `status`: `pending → approved → queued → downloading → completed`.
- `outcome`: `active`, or the terminal `declined`, `cancelled`, `failed`.

Once a request is submitted, it fans out into one `media_request_targets` row per
quality. Each target change recomputes the request's status and outcome in the
same transaction (`aggregateStatus`), so after submission the targets own the
request's state.

## Routing facts

Creating a request reads the title's TMDB detail once, after the cheap refusals
(already in the library, already requested). The server's copy of the title and
year replaces the client's, and a snapshot of what routing can match on is
stored with the request as `routing_facts`: TMDB genre, keyword, network and
company IDs, original language, origin countries, year, and whether TMDB tags it
anime. IDs rather than names, because names follow the configured TMDB
language. When TMDB cannot answer, the request is still created from the
client's copy, and the facts stay uncaptured until routing fetches them.

## Routing

Silo, not the router plugin, decides which server each quality tier of a request
goes to (`internal/requests/routing.go`). Routes (`request_routes`) belong to a
media type and hold conditions and a destination per tier: a server plus
overrides for its plugin config (root folder, quality profile, tags, series type,
minimum availability, ...). Conditions match on the request's routing facts and
requester: anime, genre, keyword, original language, origin country, year range,
network, studio, requesting account. Every set condition must hold, and a list
matches any of its values.

Each tier is decided on its own: the first enabled route, in position order,
whose conditions match and that has a destination for the tier wins, and the
media type's fallback route (no conditions) comes last. A route with no
destination for a tier lets that tier fall through; `skip_uhd` stops a matching
title from getting a 4K copy at all, even with `force_dual_quality`. Without
`force_dual_quality`, a title no route sends to a 4K server gets no 4K copy.

A routed submission calls the plugin once per tier with only the chosen server.
Its config carries the route's overrides and marks it the tier's default in the
Sonarr/Radarr plugin's terms, with the plugin's own anime overlay off, so the
existing plugin follows the route without knowing about routing. Each target
records the route that sent it. A server a route sends to cannot be deleted
until the route stops using it, so deleting a server never silently reroutes
titles, and it cannot be switched to the type the route's media type cannot use
(Sonarr for movies, Radarr for series) or stop taking that media type. A chosen
server that is disabled, not set up (no installation, no key), of the wrong type
or not taking the media type anyway is an admin-fixable
problem: when nothing has been
sent yet, the submission retries with backoff; a later tier that fails that way
becomes a failed target. Status checks go through the plugin installation that
owns each target's server, and one plugin failing does not discard the statuses
another reported. A media type with no routes keeps the plugin's own routing:
every usable connection is handed over and the plugin picks.

Admins manage routes through `/api/v2/admin/request-routes` (in the web admin,
Settings › Requests, which also hides the server switches routing now owns).
Each media type
always has its fallback ("Everything else"); until it is saved it has no servers
and routing leaves the media type to the plugin. Saving it requires an HD server,
since a saved fallback moves the media type to Silo's routing. A rule cannot be added before
the fallback has an HD server, because the first rule switches the media type to
Silo's routing and titles no rule matches would otherwise have nowhere to go.
Rules must narrow (at least one condition) and must do something (a destination,
or skip 4K), and cannot override the config keys routing sets itself.

The migration that introduced routes carried the Sonarr/Radarr plugin's routing
over unchanged: each media type's first usable default and default-4K servers
(by name) became its fallback route, and a default server's anime settings
became an Anime route. Once a media type has routes, the connections' own
default and anime switches no longer decide anything.

A request created before facts were captured, or while TMDB was unreachable, has
them fetched when it is first routed; if TMDB still cannot answer, the
submission retries rather than route on missing facts.

## Transitions are guarded

Every status or outcome write made by an admin, a user, or the reconcile pass
names the states it may start from (`StateGuard`), and the store applies it with
a single `UPDATE … WHERE status = ANY(…) AND outcome = ANY(…)`. A write whose row
has already moved fails with `ErrInvalidState`. Two admins approving at once, or
an approval racing a decline, therefore apply exactly one transition. Services
never check the state in Go and then write it unconditionally. The exception is
the target aggregate: a target change always recomputes the request's status from
its targets, because after submission the targets are the source of truth.

Decline and cancel apply to requests nothing has been sent for: `pending` ones,
and `approved` ones with no target and no live submission lease (waiting for the
library, or backing off after a failed send). Once a submission is in flight or
a target exists, the request stays in the pipeline until it completes or fails,
because withdrawing it could leave the downstream service's state diverged from
Silo's. Retry reopens a `failed` request to `approved` + `active` in one guarded
write.

## Submission is claimed

Approval commits before anything is sent. The submission itself runs only for
the caller that claims it: `ClaimSubmission` sets `submit_lease_until` and counts
the attempt, and succeeds only while the request is `approved` and `active`, no
lease is live, and its `next_submit_at` backoff has passed. Admin approval, auto-approval and the
reconcile pass on every server all go through the claim, so a request is never
submitted twice at once. The claim is a lease: if the claiming server dies
mid-call, the reconcile pass picks the request up after the lease.

A failed attempt keeps the approval. It records `last_error`, releases the lease,
schedules the next attempt in `next_submit_at` (5 minutes, doubling to an hour),
and answers the caller with
the approved request rather than an error. Only the attempt holding the current
lease can do this: one that outlived its lease while another server claimed the
request leaves the newer claim alone. After `maxSubmitAttempts` the request
is marked `failed` for an admin to retry, under the same lease check.

A successful attempt records its targets under that check too, in one
transaction with the status they imply. An attempt that outlived its lease while
the request was withdrawn, completed from the library, or claimed again drops
its result and leaves the request as it finds it. The router call itself carries
no idempotency key, so a service the stale call reached may still hold the title.

A submission converges the request's targets to the qualities it currently
wants. A failed target for a quality it no longer wants is deleted, but only
when that quality set was resolved without error. A failed entitlement lookup
or a connection skipped for a missing key makes the set look smaller than it
is, so in either case every failed target is kept for an admin to see. If
nothing is left to send, the remaining targets decide the status.

## Following a title

A profile that finds a title someone else already requested can follow it
instead of requesting it again (`PUT`/`DELETE
/api/v2/requests/follows/{media_type}/{tmdb_id}`). Following needs the same
access as requesting: requests enabled, the account allowed to request and not
blocked by its request limit, and the title within the profile's rating
ceiling. It is refused for a title with no open request (request it instead)
and for one already in the library; the insert itself checks for the open
request and holds a share lock on its row until the follow commits, so a
follow cannot land just after the request was declined, cancelled or
completed, and miss that transition's follow cleanup.

A follow belongs to the title and the profile (`media_request_follows`, keyed
by account and profile id, since profile ids repeat across accounts), not to
one request, so it survives the request failing and being retried or requested
again. Declining or cancelling the request clears the title's follows: the title
is no longer on its way, and the follower can request it themselves. The
requesting profile never needs a follow: the fulfilled notification always
reaches it. When a request's fulfilled notification goes out, it is also sent
to every follower of the title, marked `follower` so its wording does not say
"your request", and those follows are then cleared. A dispatch failure leaves
the follows for the retry, and the server-channel announcement waits until an
attempt has reached every recipient, so a retry does not repeat it.

Request state carries `following` (the viewer requested or follows the title)
and `requested_by_viewer` (the viewing profile made the request, so there is
nothing to follow); `GET /requests/status` advertises `follow_supported`.

## Without a router

Requests do not need Sonarr, Radarr or any other router plugin. When no
enabled router connection serves the request's media type, approval (by an
admin or by the requester's auto-approve policy) leaves the request `approved`,
and the reconcile pass completes it once the title is in the library. A
connection that exists but cannot be used (no API key, not bound to a plugin
installation) is a setup problem instead: the submission retries with backoff
and records the reason in `last_error`, so fixing the connection lets the
request through.

The library also completes requests that never reached a router: a `pending`
request whose title appears needs no approval any more, and a `failed` request
whose title appears is complete, unless it failed after delivering one quality,
in which case the failure stays for an admin to retry. Both go through the same
guarded write as any other completion, and the requester is notified.

## Reconcile

Every five minutes the reconcile pass submits approved requests, asks the router
for target status, and completes open and failed requests whose media is present
in the library.
Completing from the library skips an approved request while its submission lease
is live, so it cannot race a router call that is creating targets.
Every API process runs the task manager, so an advisory lock lets one server run
each pass. A pass has two rotations. In-flight requests (`approved`, `queued`,
`downloading`) get router calls. Requests only the library can complete
(`pending`, and `failed` in the last 30 days) get a presence check and nothing
else, so a backlog of them cannot slow the router polling. Each rotation takes
candidates in `last_reconciled_at` order, stamps each one when checked, even if
it errors, and looks presence up in one batch per media type. The 30-day bound
keeps an upgrade from completing, and notifying, a backlog of old failures.

## Re-requesting a failed title

Creating a request deletes the requester's own failed requests for the same
title inside the insert transaction, before the quota check, so the re-request
does not count against itself. The quota is checked only there, under the
requester's advisory lock. Other accounts' failed requests are left alone: they
are those users' history and count against their quota. Retrying one of them
after someone else has requested the title answers `ErrAlreadyRequested`, since
only one active request per title may exist.
