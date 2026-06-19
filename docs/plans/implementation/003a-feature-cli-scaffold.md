# Feature 03a — CLI Scaffold

_Delivers: the full `morsel` command tree exists and is navigable; every command group and flag is wired up via Cobra; all handlers are stubs that return "not yet implemented"._

**Direct dependencies:** [F01](001-feature-repository-foundation.md)

> Can be developed in parallel with F02 and F03. Needed by F04 (lint implementation) and any feature that implements a CLI command.

## Design

All CLI code lives in one flat `package cli` under `internal/cli/`. A single `cli` struct defined in `root.go` is the shared state carrier; every other file in the package adds methods to it. No package-level variables (cobra command trees are built inside `Execute()`, not at init time).

The `PersistentPreRunE` on the root command attempts to load the profile but silently ignores absence — commands that require auth call `requireProfile()`, which returns a descriptive error. Commands that create the profile (`operator login`, `service bootstrap`) never call `requireProfile()`.

See `docs/plans/implementation/lets-make-it-flat-snazzy-whale.md` in the claude plans directory for the full design rationale.

## Tasks

- [x] `internal/cli` package foundation — `cli` struct; `Execute()` entry point; `buildRoot()` wiring all four command groups; `--profile` persistent flag (default `"default"`); `loadProfilePreRun` on root (loads profile, silently tolerates absence); `requireProfile()` helper
- [x] `Profile` struct — GCPPlatform and LocalPlatform fields matching the profile JSON schema in [`components/cli.md`](../../specs/components/cli.md); `readProfile()` and `profilePath()` helpers (`~/.config/morsel/<name>.profile.json`); `writeProfile()`/`deleteProfile()` deferred to F03 where they are first called
- [x] `service` command group — `morsel service bootstrap --platform <gcp|local> [--kubeconfig <path>]` (`--platform` required), `morsel service status`, `morsel service delete --confirm` (guard: fails without flag), `morsel service upgrade retry`; all run implementations are stubs
- [x] `operator` command group — `morsel operator login`, `logout`, `principal add --principal/remove --principal/list`, `tier list/create/edit/set-default/delete` (shared `tierFlags` struct + `registerTierFlags()` for quota flags), `app exempt add --repo --app/remove --repo --app`, `repo exempt add <org/repo>/remove <org/repo>`; all run implementations are stubs
- [x] `app` command group — `morsel app deploy`; stub
- [x] `lint` command — `morsel lint`, `--staged`, `--fix` flags; stub
- [x] Wire `cmd/morsel/main.go` — call `cli.Execute()`; removed the old `flag`-based stub
