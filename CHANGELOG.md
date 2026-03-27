# Changelog

All notable changes to this project will be documented in this file.

## v0.0.3 - 2026-03-27

### Improvements
- **Documentation**: Updated README with latest API changes and usage examples
- **Documentation**: Added CHANGELOG.md for release tracking
- **Release**: Prepared for v0.0.3 official release

## v0.0.2 - 2026-03-27

### Breaking Changes
- **Rename**: `(*Request).EnableTrace()` has been renamed to `(*Request).EnableHTTPTrace()` to avoid confusion with OpenTelemetry tracing

### Features
- **OpenTelemetry**: OpenTelemetry is now a required dependency, no need to build with `otel` tag anymore
- **OpenTelemetry Control**: Added global OpenTelemetry tracing control functions:
  - `lian.EnableOtelTrace()` - Enable OTel tracing globally (default enabled)
  - `lian.DisableOtelTrace()` - Disable OTel tracing globally
- **OpenTelemetry Per-Request Control**: Added per-request OTel tracing methods:
  - `(*Request).DisableOtelTrace()` - Disable OTel tracing for specific request
  - `(*Request).EnableOtelTraceForRequest()` - Force enable OTel tracing for specific request (overrides global setting)
- **Independent Controls**: HTTP console trace and OpenTelemetry tracing now have completely independent switches

### Improvements
- **Documentation**: All Chinese comments have been translated to English for better internationalization
- **Documentation**: Updated README with new OpenTelemetry usage examples and API changes
- **Build**: Removed conditional compilation for OpenTelemetry, simplifying build process

### Bug Fixes
- Fixed potential nil pointer issues when OpenTelemetry is disabled
