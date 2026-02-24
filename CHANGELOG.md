# Changelog

## [0.5.0](https://github.com/fredrikaverpil/claudeline/compare/v0.4.0...v0.5.0) (2026-02-24)


### Features

* rename -git-tag to -git-branch and add -git-branch-max-len flag ([#28](https://github.com/fredrikaverpil/claudeline/issues/28)) ([1886349](https://github.com/fredrikaverpil/claudeline/commit/188634941cc35dae8eda1a95f37410064536f70f))

## [0.4.0](https://github.com/fredrikaverpil/claudeline/compare/v0.3.0...v0.4.0) (2026-02-24)

> **Note:** The v0.4.0 release was revoked due to incorrect flag naming. Use
> v0.5.0 instead.


### Features

* display git branch and tag in status line ([#27](https://github.com/fredrikaverpil/claudeline/issues/27)) ([e60501a](https://github.com/fredrikaverpil/claudeline/commit/e60501a55d0fb1af8abc38294f88d716cc1ab785)), based on [#21](https://github.com/fredrikaverpil/claudeline/pull/21) by [@bpg-dev](https://github.com/bpg-dev)
* use pre-built binaries as primary install method ([#26](https://github.com/fredrikaverpil/claudeline/issues/26)) ([c9f1a7d](https://github.com/fredrikaverpil/claudeline/commit/c9f1a7d3a797a86b239e1441026598051f9b3205))


### Bug Fixes

* disable goreleaser changelog ([#24](https://github.com/fredrikaverpil/claudeline/issues/24)) ([ca65697](https://github.com/fredrikaverpil/claudeline/commit/ca65697c0dbe358cbebbd74aa1e2edd6483a1331))

## [0.3.0](https://github.com/fredrikaverpil/claudeline/compare/v0.2.4...v0.3.0) (2026-02-24)


### Features

* add goreleaser to release workflow ([#22](https://github.com/fredrikaverpil/claudeline/issues/22)) ([13928f8](https://github.com/fredrikaverpil/claudeline/commit/13928f8eea054d6be072c9e1b59c52828109fe1e))

## [0.2.4](https://github.com/fredrikaverpil/claudeline/compare/v0.2.3...v0.2.4) (2026-02-23)


### Bug Fixes

* prevent ANSI color artifacts in status line ([#19](https://github.com/fredrikaverpil/claudeline/issues/19)) ([96ac652](https://github.com/fredrikaverpil/claudeline/commit/96ac65230ae217b1896b473fc1ba1fc44f377769))

## [0.2.3](https://github.com/fredrikaverpil/claudeline/compare/v0.2.2...v0.2.3) (2026-02-22)


### Bug Fixes

* use math.Round for context and quota percentage conversions ([#17](https://github.com/fredrikaverpil/claudeline/issues/17)) ([c375091](https://github.com/fredrikaverpil/claudeline/commit/c3750912ccbfc5139b13211062aef2c37de1b96a))

## [0.2.2](https://github.com/fredrikaverpil/claudeline/compare/v0.2.1...v0.2.2) (2026-02-22)


### Bug Fixes

* guard macOS keychain lookup with runtime.GOOS check ([#14](https://github.com/fredrikaverpil/claudeline/issues/14)) ([288533e](https://github.com/fredrikaverpil/claudeline/commit/288533eb01912065de39627cdc695a4f803bb07b))

## [0.2.1](https://github.com/fredrikaverpil/claudeline/compare/v0.2.0...v0.2.1) (2026-02-22)


### Bug Fixes

* use os.TempDir() instead of hardcoded /tmp for cross-platform support ([#12](https://github.com/fredrikaverpil/claudeline/issues/12)) ([4f22f7c](https://github.com/fredrikaverpil/claudeline/commit/4f22f7c47224059d2fa84f2d72ad1eaaa5d1a5d5))
* use profile-specific cache file path when CLAUDE_CONFIG_DIR is set ([#10](https://github.com/fredrikaverpil/claudeline/issues/10)) ([476eade](https://github.com/fredrikaverpil/claudeline/commit/476eadecc2466179823604f8d7e4423ba07b3b0d))

## [0.2.0](https://github.com/fredrikaverpil/claudeline/compare/v0.1.1...v0.2.0) (2026-02-22)


### Features

* add -version flag ([#8](https://github.com/fredrikaverpil/claudeline/issues/8)) ([53a2b80](https://github.com/fredrikaverpil/claudeline/commit/53a2b802f0d5ce0ab14f4c9fcea3e5d1726f0451))

## [0.1.1](https://github.com/fredrikaverpil/claudeline/compare/v0.1.0...v0.1.1) (2026-02-22)


### Bug Fixes

* avoid os.Exit bypassing deferred log file close ([#3](https://github.com/fredrikaverpil/claudeline/issues/3)) ([2608886](https://github.com/fredrikaverpil/claudeline/commit/2608886d9b5b7a52f8650a735460803f0f853ae7))
* replace fmt.Println with fmt.Fprintln to satisfy forbidigo ([#6](https://github.com/fredrikaverpil/claudeline/issues/6)) ([2ca5b3b](https://github.com/fredrikaverpil/claudeline/commit/2ca5b3b25c9e4e57735a23f39f99c7b5e7df9727))
* use canonical HTTP header for Anthropic-Beta ([#2](https://github.com/fredrikaverpil/claudeline/issues/2)) ([28ecd45](https://github.com/fredrikaverpil/claudeline/commit/28ecd455c6f1c985935b5b15fc55496c72b79a5c))
* use errors.New for static error strings ([#4](https://github.com/fredrikaverpil/claudeline/issues/4)) ([c2d1e32](https://github.com/fredrikaverpil/claudeline/commit/c2d1e32d8505431fb76253c7fca700d2a6193870))
* use http.NewRequestWithContext for proper context propagation ([#5](https://github.com/fredrikaverpil/claudeline/issues/5)) ([c26887f](https://github.com/fredrikaverpil/claudeline/commit/c26887fe825a50849f3a954d23e37005b7d7f25f))

## 0.1.0 (2026-02-22)

### Features

- Initial release
