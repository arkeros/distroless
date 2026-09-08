# A postgres family, and a wal-g channel of it

**Status:** In progress, 2026-09-08. Steps 1 and 2 are done and green; 3–7
remain. Step 2 measured out at **46.5 MB compressed** for the release image
(37.2 MB of it ICU), against ~150 MB for the official `postgres:18`.

`catan` builds its production database image itself — `deploy/postgres/apko.yaml`,
Wolfi's `postgresql-18` plus `wal-g-pg`, assembled by apko, scanned by a local
grype task and pushed to Artifact Registry. It is the one image in that repo
that is not built by `ko`, and the only reason it exists is that no distroless
postgres was available to build on.

This plan adds one: a **Family** `postgres`, on both **Distro**s, with a channel
that carries wal-g, and then retires catan's apko image onto it.

## What gets published

Family `postgres`, following the python family's shape — Hummingbird takes the
bare tags, Debian takes `-debian`-suffixed ones — and nginx's shape for
channels.

| Distro | Lines | Why |
|---|---|---|
| Hummingbird | 18.6, 17.11 | `postgresql`/`postgresql-server` (18.6) and `postgresql17`/`postgresql17-server` (17.11), both current in the pinned snapshot |
| Debian | 18.6 | sid carries `postgresql-18` 18.6-3 and **no 17** at the pinned snapshot |

**This is a deviation from "18 and 17" on both distros, and it is not a
shortcut.** Debian sid has no `postgresql-17` binary package — the archive
builds one line at a time and 18 is it. The python family already encodes
exactly this asymmetry ("Hummingbird rebuilds every line upstream still
patches, Debian sid only the ones it is currently building"), so
`POSTGRES_VERSIONS` is keyed by distro the way `PYTHON_VERSIONS` is, and the
Debian 17 line appears the day sid has one. The alternative — sourcing 17 from
`apt.postgresql.org` the way nginx is sourced from `nginx.org` — is a second
apt repo, a second lockfile, a second upstream identity and a second set of
gates, bought for one line of one distro. Not worth it; revisit if a consumer
asks for Debian 17.

Both lines put their binaries in `/usr/bin` on Hummingbird (verified:
`postgresql17` ships a real `/usr/bin/psql`, not an alternatives symlink), and
in `/usr/lib/postgresql/<line>/bin` on Debian. `PATH` is therefore a per-distro
value in `config.bzl`, not a per-line one.

### Tags

Generated from `(line, distro, channel, debug)`, with `latest` on the highest
Hummingbird line:

```
postgres:18  18.6  latest          postgres:17  17.11
postgres:18-debug  18.6-debug  debug
postgres:18-walg  18.6-walg  walg
postgres:18-debian  18.6-debian  debian   (+ -debug, -walg)
```

## The wal-g channel

wal-g is packaged by neither distro — it is a Wolfi package, and the reason
catan's image is a Wolfi image. It is pinned here instead, as a `tarballs`
lockfile entry (`images/postgres/walg.lock.json`, `source: "walg"`), which needs
a new `Source` implementation in `//bazel/tarballs` alongside `nodejs.go` and
`temurin.go`. `knife tarballs update` then moves it like every other pinned
upstream. Assets: `wal-g-pg-24.04-{amd64,aarch64}.tar.gz` from `wal-g/wal-g`,
v3.0.9 today.

### Why not compile it from source

Building wal-g here with `rules_go` was the obvious alternative and was tried
before this was written. It would have put the binary inside the pipeline the
**Platform provenance** describes, instead of importing one built by wal-g's
CI, and it would have let a vulnerable transitive dependency be fixed with a
`go.mod` bump rather than a **VEX statement**. Three findings sank it, and they
are recorded here so the question is not reopened from scratch:

- **wal-g has no valid Go module version at v3.** Its `go.mod` declares
  `module github.com/wal-g/wal-g` with no `/v3` suffix while the project tags
  `v3.0.9`, so semantic import versioning refuses it: `proxy.golang.org` 404s
  `v3.0.9` and answers `@latest` with `v0.2.22`, from 2021. The only way to
  name the release commit is the pseudo-version
  `v0.2.23-0.20260820091616-3e493188db28`, which reads as 0.2.x forever. The
  release binary hits the same wall from the other side — its own buildinfo
  says `v0.0.0-20260820091616-3e493188db28+dirty`, not `v3.0.9` — so the SBOM
  identity is a pseudo-version whichever route is taken, and this is not a
  reason to prefer either. It is a reason not to put that string in `go.mod`.
- **It does not build.** `@com_github_wal_g_wal_g//main/pg` fails in a
  transitive dependency, `yandex-cloud/go-sdk/v2/pkg/iamkey` (`undefined: Key`),
  which looks like the generated-protobuf problem the sigstore modules already
  carry a `gazelle_override` for. Fixable, probably, and the cost is adopting a
  Yandex Cloud backend nobody here uses.
- **It moves the shared module graph.** Adding wal-g alone pulled 49 new
  indirect modules into the root `go.mod` that `knife`, the **Registry** and
  the **Directory** all build from, and bumped `azcore` 1.21.1 → 1.23.0,
  `azidentity` 1.13.1 → 1.14.0, `microsoft-authentication-library-for-go`
  1.7.0 → 1.7.2 and `lz4` 4.1.26 → 4.1.28. The first two are on sigstore's KMS
  path, so a wal-g dependency bump would be able to move a version inside this
  repo's own signing and verification code. A separate `go.mod` does not help:
  gazelle's `go_deps` runs MVS across every `from_file` module, so it tidies
  the root file without isolating the versions.

What compiling would have bought, verified rather than assumed: `rules_go`
*does* emit full module buildinfo (checked against `knife` — 123 deps
enumerated), so routability is not the differentiator; and a `CGO_ENABLED=0`
build with no build tags drops brotli, libsodium and lzo, which catan does not
use — it sets only `WALG_GS_PREFIX`, and wal-g's default
`WALG_COMPRESSION_METHOD` is `lz4`, which is pure Go. Revisit if wal-g ever
publishes a `/v3` module path.

Three consequences of the download route worth naming before writing any of it:

**A wal-g image without a shell is useless, so the channel includes busybox.**
Postgres runs `archive_command` and `restore_command` through `system()`, which
is `/bin/sh`. An image with wal-g and no shell can still take a base backup and
cannot archive a WAL segment, which is the point of wal-g. So `-walg` carries
busybox and there is no `-walg-debug`; the channel is the debug rootfs plus a
binary. If you would rather have four orthogonal tags than three honest ones,
say so — it is a one-line change and I think it publishes a tag that does not
work. Building the family turned this from a prediction into a measurement —
see below, and note that `/busybox` on `PATH` is not enough.

**wal-g is 61 MB and drags a Go module graph into the scan.** syft's
go-module-binary cataloger reads the build info and emits 123 `pkg:golang/…`
components — aws-sdk-go, the Azure and GCP SDKs, grpc, and so on — plus wal-g
itself at the pseudo-version above. That is *good*: they are **Routable**
through OSV, so it is not a **Silent zero**. It also means the
`-walg` channel's **Gate** will fail on transitive Go CVEs that the postgres
channel never sees, on a schedule set by other people's dependency hygiene, and
the only fix available to us is bumping the pin or writing a **VEX statement**
per finding with a 90-day **Expiry**. Budget for that recurring cost; it is the
price of the answer chosen over "catan layers it".

**It is a channel, not a Variant.** `CONTEXT.md` defines a **Variant** as one
cell of Distro × User × debug × architecture, and says two variants "differ in
what is inside them, never in what they are for". wal-g is inside, so the
vocabulary holds, but it is close enough to the line that the glossary should
gain a sentence when this lands.

## Contents, and the locale problem

Per the decision: server, client tools, contrib, ICU, and an `en_US.utf8`
locale.

Hummingbird 18 layer, on top of `static` + `cc`:

| Package | Installed |
|---|---|
| `libicu` | 39.7 MB |
| `postgresql-server` | 33.7 MB |
| `postgresql` (client) | 8.8 MB |
| `glibc-langpack-en` | 6.0 MB |
| `util-linux` | 3.8 MB |
| `postgresql-contrib` | 3.7 MB |
| `systemd-libs`, `krb5-libs`, `cyrus-sasl-lib`, `pam`, `openldap`, `libxml2`, `libxslt`, `lz4-libs`, `readline`, `ncurses-libs`, `numactl-libs`, `postgresql-private-libs` | ~12 MB |

≈ 108 MB uncompressed before wal-g, ≈ 170 MB with it. Official `postgres:18` is
~450 MB. `libicu` alone is a third of it and cannot be trimmed — it is one rpm
and `postgresql-server` links `libicuuc`/`libicui18n` directly, so ICU is not
optional whatever the collation provider ends up being.

Compose from `:own` plus explicitly named libraries, the way `//images/nginx`
does, rather than from the resolved closure: `postgresql-server` declares
`user(postgres)`, `filesystem(unmerged-sbin-symlinks)`, `/bin/sh` and
`/usr/bin/bash` as requires, all of them scriptlet-shaped, and the closure
would ship bash into every postgres image to satisfy one `%post`. The pin tool
warns rather than fails on an unresolvable require (`closeDeps`, `pin/main.go`),
so the lockfile stays clean; the layer list is where the decision is made.

### Debian cannot have `en_US.utf8`, and should not pretend to

This is the one place the requested contents cannot be delivered on both
distros, so it is stated plainly rather than quietly dropped.

Hummingbird has `glibc-langpack-en`: a 6 MB rpm shipping a compiled
`en_US.utf8`. Debian has no equivalent.

- `locales` (15.7 MB) ships *sources* under `/usr/share/i18n` and a
  `locale-gen` that must be run at install time. There is no install time here,
  and no shell to run it in.
- `locales-all` ships a prebuilt archive and is **236 MB installed** — more
  than twice the whole rest of the image.
- Generating just `en_US.UTF-8` with `localedef` at build time means executing a
  target-architecture ELF in a Bazel action: it does not work for arm64 on an
  amd64 runner, and does not work at all on a Mac. Rejected on hermeticity.

So the Debian variants ship `C.UTF-8` (built into glibc since 2.35, no locale
files needed) and ICU collations, and the family README says so. PostgreSQL 18's
`--locale-provider=builtin` and `--locale-provider=icu` both work there;
`initdb --locale=en_US.utf8` does not. **catan's cluster was initdb'd
`en_US.utf8`, so catan takes the Hummingbird tags** — which are the bare ones
anyway.

## Postgres needs `/bin/sh` by that exact path

Found while building step 2, and the one thing here that no other family has
had to deal with. Postgres's tooling does not look up a shell on `PATH`; it
calls `/bin/sh` absolutely, through `popen()` and `system()`:

- `initdb` and `pg_ctl` locate their helper binaries with `popen()`. Without a
  shell `initdb` fails with `program "postgres" is needed by initdb but was not
  found in the same directory as "/usr/bin/initdb"` — which is `popen()`
  failing, not a missing file, and is thoroughly misleading. `/usr/bin/postgres
  --version` runs fine one exec earlier.
- The server runs `archive_command`, `restore_command` and `COPY … PROGRAM`
  through `system()`.

Every other family's debug image is content with busybox under `/busybox` and
that directory on `PATH`, because a human types the command. That is not enough
here: the **Debug variant** of this family also gets a `./usr/bin/sh ->
/usr/bin/busybox` symlink (`sh_symlink_layer`), `usr/bin` rather than `bin`
because the images are usrmerged. Two consequences:

- The **release** variants run a cluster somebody else initialised, and cannot
  `initdb`, `pg_ctl` or archive WAL. That is a real and narrow product, and the
  family README has to say so plainly rather than let a consumer discover it
  from that error message. It is asserted, not just written down: the release
  structure test requires `/bin/sh` to be *absent*, and the debug one requires
  it to be present.
- It confirms the wal-g channel belongs on the debug rootfs, for a second
  independent reason.

## Image configuration

```
entrypoint  ["postgres"]          # /usr/bin/postgres, on PATH
env         PGDATA=/var/lib/postgresql/data
            LANG=en_US.utf8       # C.UTF-8 on Debian
            PATH=…                # + /usr/lib/postgresql/<line>/bin on Debian
user        matrix default (root and nonroot indexes, 65532)
```

`/var/lib/postgresql/data` ships as a `0700` directory owned by 65532 — a
`tar` mtree layer, the same shape as `nginx_conf_layer`. Postgres refuses to
run as uid 0, so the `root` index of this family is a matrix artefact that
nobody should pull; the README should say the nonroot tags are the tags.

The nonroot user is named `nonroot`, not `postgres`. Postgres does not care
what the name is, only that `getpwuid()` resolves, and `static`'s passwd
resolves 65532. catan's `moor` records say `user = "postgres"` and will need
`user = "nonroot"` (or the uid).

## Tests

Red first, in this order, per family convention:

1. **`elf_needs`** on `postgres`, `psql`, `pg_dump`, `initdb` — every `NEEDED`
   soname present in the layer set. This is what catches a missing `libicu` or
   `numactl` before anything is built into an image.
2. **`container_structure_test`** — behavioural, not a file census, per the
   house rule. Borrow catan's proof, which is already the right test: `initdb`
   a throwaway cluster at the production locale, `pg_ctl start`, create a
   table, `pg_isready`, `pg_dump` it, stop. On the `-walg` channel, add
   `wal-g backup-push` to a local prefix and one `wal-g wal-push`, which is the
   only test that proves postgres, wal-g and the shell work *together*.
3. **Lock version tests** — `jq_test` holding `POSTGRES_VERSIONS` equal to what
   each lockfile pins, per line and distro, so a repin that moves a line turns
   a test red instead of publishing a tag that lies.
4. **The three gates**, from `distroless_matrix`: findings, stale suppressions,
   silent zero. `consumer_scan` stays **off** — every package here carries its
   distro's metadata, so the SBOM view and the image view agree by
   construction. It goes on only if wal-g's identity turns out to need proving
   the way nginx's did.

## Order of work

Each step is a commit, each ends green, and no step depends on a later one.

1. ~~`//bazel/tarballs`: a `walg` source + tests. No image yet.~~ **Done** —
   resolver, tests, and `images/postgres/walg.lock.json` generated from the
   real API and checksum-verified against an independent download.
2. ~~Hummingbird 18: manifest packages, repin, layers, `elf_needs`, matrix,
   structure test.~~ **Done** — the repin was purely additive (25 packages
   added, nothing existing moved), and all 30 generated tests pass, including
   a cluster that initialises at `en_US.utf8`, starts, round-trips a row,
   dumps, and loads `pg_stat_statements`.
3. Hummingbird 17.
4. Debian 18 (`C.UTF-8`), and the README paragraph about the locale.
5. The `-walg` channel and its wal-g structure test.
6. `mirror_push` for every tag, `docs/images.md`, family README.
7. catan: point `deploy/postgres` at the published image, drop `apko.yaml`,
   `apko.lock.json` and the lock/scan tasks, keep `container-structure-test.yaml`
   as the check that the *published* image still does what the origin needs.
   `user = "nonroot"` in the moor records.

Step 7 is a change in the other repository and lands after 6 is published, not
alongside it.

## Open

- **Does the `-walg` channel earn its keep, once its Go CVE surface is real
  rather than predicted?** If the first month is a VEX statement a week, the
  honest answer is that wal-g belongs in catan's own layer and this channel
  should be withdrawn. Worth revisiting at 30 days rather than defending.
- catan pins 18.4 today; both distros here are at 18.6. Moving is a postgres
  minor upgrade on a live cluster — in-place restart, no `pg_upgrade` — but it
  is still a database change, not an image change, and belongs in catan's
  runbook.
