<p align="center">
  <img src="assets/spark-mark.svg" alt="Spark" width="112" height="112">
</p>

<h1 align="center">Spark</h1>

<p align="center">
  A customizable, self-hosted media server for films, series, audiobooks, ebooks, podcasts, and manga.
</p>

<p align="center">
  Spark is a fork of <a href="https://github.com/Silo-Server/silo-server">Silo</a>,
  focused on more customization and additional options.
</p>

<p align="center">
  <a href="https://github.com/Joloxx9/spark-server/releases"><img alt="Latest GitHub release" src="https://img.shields.io/github/v/release/Joloxx9/spark-server?include_prereleases&amp;sort=semver&amp;display_name=tag&amp;style=flat-square&amp;label=release"></a>
  <a href="https://github.com/users/Joloxx9/packages/container/spark-server"><img alt="Spark container image on GHCR" src="https://img.shields.io/badge/container-GHCR-2496ED?style=flat-square&amp;logo=docker&amp;logoColor=white"></a>
  <a href="https://github.com/Joloxx9/spark-server/actions/workflows/fork-image.yml"><img alt="Spark image build" src="https://img.shields.io/github/actions/workflow/status/Joloxx9/spark-server/fork-image.yml?branch=spark&amp;style=flat-square&amp;label=Spark%20image"></a>
  <img alt="Go 1.26" src="https://img.shields.io/badge/Go-1.26-00ADD8?style=flat-square&amp;logo=go&amp;logoColor=white">
  <img alt="React 19" src="https://img.shields.io/badge/React-19-149ECA?style=flat-square&amp;logo=react&amp;logoColor=white">
  <a href="LICENSE"><img alt="AGPL-3.0-or-later license" src="https://img.shields.io/badge/license-AGPL--3.0--or--later-555555?style=flat-square"></a>
</p>

<p align="center">
  <a href="#quick-start">Quick start</a>
  · <a href="DEVELOPMENT.md">Documentation</a>
  · <a href="https://github.com/Joloxx9/spark-plugins">Plugin catalog</a>
  · <a href="docs/release-versioning.md">Builds &amp; releases</a>
  · <a href="https://github.com/Joloxx9/spark-server/issues">Issues</a>
  · <a href="#supporting-spark">Supporting Spark</a>
  · <a href="CONTRIBUTING.md">Contributing</a>
</p>

---

> [!WARNING]
> Spark is pre-release. APIs, configuration, and database migrations may change
> before the first stable release. Back up your deployment before updating.

## Inherited feature scope

Spark is based on Silo and currently inherits its [1.0 milestone](https://siloserver.org/milestone/1.0/)
scope: movies and series. Audiobooks, ebooks, and Audiobookshelf compatibility
remain **beta** features. Spark-specific changes and options are tracked in this
fork.

## What Spark does

<table>
  <tr>
    <td width="33%" valign="top">
      <strong>Play</strong><br><br>
      Direct play when possible, remux when needed, transcode otherwise, with
      VA-API, Quick Sync, and NVENC hardware acceleration.
    </td>
    <td width="33%" valign="top">
      <strong>Organize</strong><br><br>
      One catalog for films, series, audiobooks, ebooks, podcasts, and manga,
      matched through metadata plugins such as TMDB and TVDB. Spark's catalog
      includes upstream plugins and Spark additions.
    </td>
    <td width="33%" valign="top">
      <strong>Connect</strong><br><br>
      Use the included web app, or the Jellyfin/Emby-compatible API with clients
      such as <a href="https://vidhub.okaapps.com/what-does-vidhub-do/">VidHub</a>,
      <a href="https://github.com/jarnedemeulemeester/findroid">Findroid</a>, and
      <a href="https://firecore.com/infuse">Infuse</a>. Client coverage varies.
    </td>
  </tr>
  <tr>
    <td width="33%" valign="top">
      <strong>Share</strong><br><br>
      Household profiles with their own watch state, library access, and
      parental controls.
    </td>
    <td width="33%" valign="top">
      <strong>Manage</strong><br><br>
      Libraries, users, providers, storage, search, and playback are configured
      in the admin interface, not in config files.
    </td>
    <td width="33%" valign="top">
      <strong>Scale</strong><br><br>
      Start with one integrated server; split proxy and transcode roles across
      shared PostgreSQL and Redis when you need to.
    </td>
  </tr>
</table>

## Quick start

Requires Docker Compose 2.24 or newer. The default Compose file runs the
Spark omnibus image with a bundled PostgreSQL (with pgvector) and Redis, plus
Meilisearch when you turn it on.

```sh
git clone --branch spark https://github.com/Joloxx9/spark-server.git
cd spark-server
cp .env.example .env
chmod 600 .env
key="$(openssl rand -base64 48)" && printf '\nSECRET_KEY=%s\n' "$key" >> .env
```

Set `MEDIA_ROOT` in `.env` to the absolute path of your media, then:

```sh
docker compose up -d
```

Open <http://localhost:8090> and complete onboarding.

Hardware transcoding (VA-API or NVIDIA) and access for proxy or transcode
nodes on other hosts are commented out in `docker-compose.yml`; uncomment the
lines you need.

### Updating an install from before the omnibus image

An install that runs Spark, PostgreSQL, and Redis in separate containers keeps
that layout when its `.env` sets `COMPOSE_FILE=docker-compose.advanced.yml`; set
it before updating. `docker-compose.yml` is an example of a new install and
does not look for an earlier one: started on that data folder, it replaces
the old `silo` container and comes up on a new, empty database with setup
open, while the earlier PostgreSQL and Redis keep running with their data
untouched. Moving into the single container is a separate step. The separate GPU and node Compose files are gone, so a `COMPOSE_FILE`
that names one fails until it names only the deployment file you use, with
the matching lines in it uncommented. See [Moving from the multi-container
stack](docs/architecture/omnibus-image.md#moving-from-the-multi-container-stack).

The [Silo user manual](https://siloserver.org/docs) describes upstream Silo.
Spark-specific setup notes and changes are tracked in this repository.

## Builds and releases

Until the first release, default-branch images carry an ordered `build-N` tag
and the full commit SHA alongside `latest`. Build numbers order published images;
they are not release versions. [Release versioning](docs/release-versioning.md)
defines each tag and the SemVer contract.

## Documentation

- [Silo user manual](https://siloserver.org/docs) — upstream operating guide; Spark may differ
- [Development guide](DEVELOPMENT.md) — source setup, builds, tests, migrations
- [Settings API](docs/settings-api.md), [Downloads API](docs/downloads-api.md), and [Apple Push Display Token](docs/notifications-push-api.md) — client contracts

## Community and contributions

Questions, bugs, and feature requests: [Spark GitHub issues](https://github.com/Joloxx9/spark-server/issues).

Native clients are developed in
[`silo-apple`](https://github.com/Silo-Server/silo-apple) and
[`silo-android`](https://github.com/Silo-Server/silo-android). These are upstream
Silo clients; client-visible
API, authentication, playback, or metadata changes should consider both.

Right now, the most helpful way to contribute is a clear, accurate issue. We
implement and review changes through our own workflow, and pull request volume
has outgrown what we can review carefully, so a good issue usually helps us
more than a pull request.

Pull requests are still welcome from contributors who've had one merged in a
Spark repository, or when a maintainer asks for one. We close other pull
requests without review. [CONTRIBUTING.md](CONTRIBUTING.md) explains what makes
an issue useful.

## Supporting Spark

Spark is free and open source. Bug reports, feature requests, and code
contributions are welcome through [GitHub issues](https://github.com/Joloxx9/spark-server/issues).

## License and trademarks

Spark's source code is licensed under the
**GNU Affero General Public License v3.0 or later** (`AGPL-3.0-or-later`), with
an additional permission to combine it with platform SDKs such as the Windows
App SDK. See [LICENSE](LICENSE) and [PLATFORM-EXCEPTION.md](PLATFORM-EXCEPTION.md).

Spark is an independent fork of Silo. The upstream Silo name, logo, and wordmark
are trademarks of Silo Media L.L.C. and are not covered by the AGPL. Spark does
not claim to be an official Silo project. See [TRADEMARK.md](TRADEMARK.md) for
the upstream trademark terms.
