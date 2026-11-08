# Spark server (fork of Silo)

This branch (`spark`) is upstream Silo plus this fork's own changes, built as
the Spark server. `main` mirrors upstream Silo untouched: Silo stays Silo,
Spark lives here.

- Your own code changes: commit them on `spark` like any change.
- Brand: `fork/brand.env` (name and theme color) and `fork/overlay/`, which
  mirrors the paths of the web app icons it replaces. The art comes from
  `brand/generate.py` in `Joloxx9/silo-mac-build`.
- `fork/apply-brand.sh` applies the brand at image build time
  (`.github/workflows/fork-image.yml`) and is never committed, so merging
  upstream does not conflict with the rebrand. It renames the bare word "Silo"
  in the web app's own strings and the server's default names, never
  hyphenated protocol or repository names, and stops the build when an
  upstream change no longer fits one of its edits.
- Image: `ghcr.io/joloxx9/spark-server:latest`, built on every push to `spark`.
  Paths and settings inside the container (`/var/lib/silo`, `SILO_*`) keep
  their upstream names so existing deployments keep working.
