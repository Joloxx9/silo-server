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

## Transitions are guarded

Every status or outcome write made by an admin, a user, or the reconcile pass
names the states it may start from (`StateGuard`), and the store applies it with
a single `UPDATE … WHERE status = ANY(…) AND outcome = ANY(…)`. A write whose row
has already moved fails with `ErrInvalidState`. Two admins approving at once, or
an approval racing a decline, therefore apply exactly one transition. Services
never check the state in Go and then write it unconditionally. The exception is
the target aggregate: a target change always recomputes the request's status from
its targets, because after submission the targets are the source of truth.

Decline and cancel apply only to `pending` + `active` requests. An approved
request may already be on its way to a downstream service, so it stays in the
pipeline until it completes or fails. Retry reopens a `failed` request to
`approved` + `active` in one guarded write.

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
the approved request rather than an error. After `maxSubmitAttempts` the request
is marked `failed` for an admin to retry.

A submission converges the request's targets to the qualities it currently
wants. A failed target for a quality it no longer wants is deleted, but only
when that quality set was resolved without error: a failed entitlement lookup or
a connection skipped for a missing key also shrinks it, and must not erase a
failure an admin still needs to see. If nothing is left to send, the remaining
targets decide the status.

## Reconcile

Every five minutes the reconcile pass submits approved requests, asks the router
for target status, and completes requests whose media is present in the library.
Completing from the library skips an approved request while its submission lease
is live, so it cannot race a router call that is creating targets.
Every API process runs the task manager, so an advisory lock lets one server run
each pass. Candidates are taken in `last_reconciled_at` order and each one is
stamped when checked, even if it errors, so a large backlog rotates instead of
the same batch being checked every time.

## Re-requesting a failed title

Creating a request deletes the requester's own failed requests for the same
title inside the insert transaction, before the quota check, so the re-request
does not count against itself. The quota is checked only there, under the
requester's advisory lock. Other accounts' failed requests are left alone: they
are those users' history and count against their quota. Retrying one of them
after someone else has requested the title answers `ErrAlreadyRequested`, since
only one active request per title may exist.
