# Changelog

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
