# Tool Patterns Reference

Use during `research` and `design`. Everything here is a starting hypothesis. Confirm with official documentation and the tool's own commands for the targeted versions before relying on it.

## Contents

- Classify every candidate path
- Key strategy by cache type
- Common ecosystem starting points
- Cross-cutting pitfalls
- Value check

## Classify every candidate path

A single tool often produces several kinds of cacheable data. Classify each path, because the type determines key inputs, portability, and risk.

| Type | Examples | Default decision | Main risks |
|------|----------|------------------|------------|
| **Download cache**: package tarballs, wheels, modules | npm `_cacache`, pip/uv/Poetry caches, Go module cache, Maven repository, Gradle `caches/modules-2`, NuGet global packages, Cargo registry cache, Composer cache, pub cache | Cache. Best ratio of safety to benefit | Credentials stored nearby; unbounded growth |
| **Installed tree**: dependencies materialized in the project | `node_modules`, Composer `vendor`, `Pods`, `vendor/bundle`, Mix `deps`, `.venv` | Investigate. Cache only if restore is a faithful reproduction | Absolute paths, native extensions, symlinks, post-install side effects |
| **Build output / compilation cache** | Gradle build cache, Bazel disk cache, ccache/sccache, Go `GOCACHE`, Turborepo/Nx caches, Cargo `target`, Xcode DerivedData, Mix `_build` | Separate opt-in decision | Toolchain/ABI coupling, mtime sensitivity, very large size |
| **Toolchain / distribution** | Gradle wrapper distributions, rustup toolchains, SDKs, Playwright/Cypress binaries | Separate decision; key on version file and OS/architecture | OS/architecture-specific, large |
| **Metadata / index** | Cargo registry index, CocoaPods specs | Cache only when refresh cost is significant | Fast-changing; stale data |
| **Never cache** | Credential files, daemon state, sockets, lock/journal files, logs, temp dirs | Exclude | Secret leakage, corruption, nondeterminism |

Credential locations to exclude: user-level `.npmrc`, `.yarnrc.yml` with tokens, Maven `settings.xml`, Gradle `gradle.properties` in `GRADLE_USER_HOME`, Cargo `credentials.toml`, Composer `auth.json`, `nuget.config` with credentials, `.netrc`, `.pypirc`, Docker `config.json`.

## Key strategy by cache type

- **Lockfile present**: hash the lockfile. Add the manifest only when the lockfile does not fully determine resolution.
- **Manifest only (no lockfile)**: hash every file that declares dependencies or resolution settings. For example, Gradle uses `*.gradle(.kts)`, `settings.gradle(.kts)`, `gradle/libs.versions.toml`, and `gradle-wrapper.properties`. The current engine hashes one primary file plus one `additionalHashGlob`, so multi-file hashing is an engine change (see `ENGINE.md`).
- **ABI-coupled content** (installed trees with native code, build outputs, toolchains): include the runtime/toolchain version and OS/architecture in the key, or restrict the design to download caches. The current engine has no runtime-version input; adding one is an engine change.
- **Content-addressed build caches**: the tool validates entries itself, so the key should not churn on every source edit. Prefer a stable key (toolchain plus lockfile) and accept that the cache grows; plan for pruning.

## Common ecosystem starting points

"—" means no reliable single command; derive the path from configuration precedence instead.

| Tool | Detection | Cache-path query | Env / native config | Usually cache | Exclude / pitfalls |
|------|-----------|------------------|---------------------|---------------|--------------------|
| npm | `package-lock.json`, `npm-shrinkwrap.json` | `npm config get cache` | `npm_config_cache`, `.npmrc` `cache=` | `<cache>/_cacache` | `npm ci` deletes `node_modules`; user `.npmrc` tokens |
| pnpm | `pnpm-lock.yaml` | `pnpm store path` | `store-dir` in `.npmrc`; `storeDir` in `pnpm-workspace.yaml` (pnpm 10+) | content store | `node_modules` is symlinks/hardlinks; store on another filesystem forces copies |
| Yarn 1 | `yarn.lock` without `.yarnrc.yml` | `yarn cache dir` | `YARN_CACHE_FOLDER`, `.yarnrc` `cache-folder` | cache folder | |
| Yarn 2+ | `yarn.lock` with `.yarnrc.yml` | `yarn config get cacheFolder` | `.yarnrc.yml` `cacheFolder`, `enableGlobalCache`, `YARN_CACHE_FOLDER` | `.yarn/cache` or global cache | Yarn 4 defaults to a global cache outside the repo; `.yarn/cache` may already be committed (zero-installs); `.pnp.cjs` is generated |
| Bun | `bun.lock`, `bun.lockb` | `bun pm cache` | `BUN_INSTALL_CACHE_DIR`, `bunfig.toml` `[install.cache]` | install cache | |
| pip | `requirements*.txt`, `constraints.txt`, `pyproject.toml` | `pip cache dir` | `PIP_CACHE_DIR`, `pip.conf` via `PIP_CONFIG_FILE` | HTTP and wheel cache | `.venv`; project `pip.conf` is not auto-discovered |
| uv | `uv.lock` | `uv cache dir` | `UV_CACHE_DIR`, `uv.toml` / `[tool.uv] cache-dir` | cache | `UV_LINK_MODE` across filesystems; `uv cache prune --ci` reduces size |
| Poetry | `poetry.lock` | `poetry config cache-dir` | `POETRY_CACHE_DIR`, `poetry.toml` | `artifacts`, `cache` | Virtualenvs default to `{cache-dir}/virtualenvs` |
| Pipenv | `Pipfile.lock` | — | `PIPENV_CACHE_DIR` | cache | Read at process start; later steps need the variable |
| Cargo | `Cargo.lock` | — (default `$CARGO_HOME`) | `CARGO_HOME` | `registry/index`, `registry/cache`, `git/db` | `registry/src` and `git/checkouts` are re-extractable; `target`, `bin`, credentials |
| Go | `go.sum` / `go.mod` | `go env GOMODCACHE`, `go env GOCACHE` | `GOMODCACHE`, `GOPATH`, `GOCACHE`, `GOFLAGS=-modcacherw` | module cache (`cache/download` is sufficient) | Module files are read-only, which breaks cleanup/overwrite; `GOCACHE` is a build cache |
| Maven | `pom.xml` | `mvn help:evaluate -Dexpression=settings.localRepository -q -DforceStdout` | `-Dmaven.repo.local` in `.mvn/maven.config`, `settings.xml` `localRepository` | local repository | `settings.xml` credentials; SNAPSHOT churn; `*.lastUpdated` files |
| Gradle | `build.gradle(.kts)`, `settings.gradle(.kts)`, `gradle.lockfile` | — | `GRADLE_USER_HOME`, `org.gradle.caching` | `caches/modules-2`, `wrapper/dists`, optional build cache | `gradle.properties` credentials, `*.lock` files, `daemon/` |
| sbt / Coursier | `build.sbt`, `project/build.properties` | — | `COURSIER_CACHE`, `~/.ivy2`, `~/.sbt` | Coursier cache | |
| NuGet / .NET | `packages.lock.json`, `*.csproj`, `*.fsproj`, `*.vbproj` | `dotnet nuget locals global-packages --list` | `NUGET_PACKAGES`, `nuget.config` `globalPackagesFolder` | global packages | `nuget.config` credentials |
| Bundler | `Gemfile.lock` | `bundle config get path` | `BUNDLE_PATH`, `.bundle/config`, `BUNDLE_APP_CONFIG` | `vendor/bundle` | Native extensions are Ruby-version and OS-specific |
| CocoaPods | `Podfile.lock` | — | `CP_HOME_DIR` | `Pods`, `~/Library/Caches/CocoaPods` | macOS only; specs repo is large |
| SwiftPM | `Package.resolved` | — | `--cache-path`, `--scratch-path` | `.build/repositories`, SwiftPM user cache | `.build` also contains build output |
| Composer | `composer.lock` | `composer config cache-dir` | `COMPOSER_CACHE_DIR`, `config.cache-dir` | cache, optionally `vendor` | `auth.json` |
| Dart / Flutter | `pubspec.lock` | — | `PUB_CACHE` | pub cache | |
| Elixir Mix | `mix.lock` | — | `MIX_HOME`, `HEX_HOME`, `MIX_DEPS_PATH` | `deps`, Hex cache | `_build` is ABI-coupled |
| Deno | `deno.lock` | `deno info` | `DENO_DIR` | `DENO_DIR` | |
| Bazel | `MODULE.bazel(.lock)`, `WORKSPACE` | — | `.bazelrc` `--disk_cache`, `--repository_cache` | repository cache, disk cache | `output_base` is not portable |
| ccache / sccache | compiler usage, no manifest | `ccache --show-config` | `CCACHE_DIR`, `SCCACHE_DIR`, `CCACHE_MAXSIZE` | cache dir | Key on compiler version; set a size limit |
| Turborepo / Nx | `turbo.json`, `nx.json` | — | `turbo.json` `cacheDir`, `NX_CACHE_DIRECTORY` | `.turbo/cache`, `.nx/cache` | Remote cache may be the better option |

## Cross-cutting pitfalls

- **Archive fidelity**: verify symlinks, hardlinks, executable bits, read-only files, and long or case-colliding paths survive save and restore. Tools that use mtimes for up-to-date checks may rebuild or skip work incorrectly after restore.
- **Embedded absolute paths**: virtualenv shebangs, compiled metadata, and some build caches record the checkout path. Harness checkout paths can differ between executions or infrastructure types.
- **Working-tree side effects**: generated configuration files appear in `git status`, can be committed accidentally, and can fail "clean tree" or lint checks. Prefer files that are conventionally gitignored, and document the rest.
- **Source scanning of `.cache/`**: an in-repo cache can be picked up by the build or tests (TypeScript, ESLint, Jest, pytest collection, Bazel). Bazel needs `.bazelignore` (see `prepare_bazel.go`); check the target tool too.
- **Integrity**: caches are shared across branches and pull requests. Prefer content the tool verifies against the lockfile (npm integrity, `go.sum`, Cargo checksums, uv/Poetry hashes). Flag tools that execute unverified cached content.
- **Platform defaults**: on Linux the default cache usually follows `XDG_CACHE_HOME` or `~/.cache`; on macOS `~/Library/Caches`; on Windows `%LOCALAPPDATA%`. The plugin ships Windows images, so default-location logic must use `filepath` and must not assume POSIX home layouts.
- **Concurrent writers**: parallel stages can save the same key, and the last writer wins. This is acceptable only if every writer produces equivalent content.

## Value check

Before committing to a path, estimate:

```text
Cold install time:
Warm install time with restored cache:
Archive size (compressed):
Restore + save overhead:
Growth over repeated runs:
```

If restore plus save overhead approaches the time saved, or the archive grows without bound and no pruning is available, recommend not caching that path, or caching a smaller subset. Report the numbers either way.
