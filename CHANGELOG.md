# Changelog

All notable changes to Engram are documented in this file.

## [Unreleased]

### Added

- **API**: `GET /api/folders` returns the immediate subfolders of a display-path
  prefix with recursive file counts. The response is complete rather than
  paginated — clients previously derived the folder tree from whichever page of
  files happened to be loaded, so folders were missing until scrolled into view
  and could appear empty.
- **API**: `GET /api/files` accepts `scope=folder` and `path`, restricting
  results to the direct children of a directory. Omitting both preserves the
  existing behavior and response shape exactly.
- **Migration 005**: `idx_files_owner_filename` on `(owner, filename
  text_pattern_ops)` to serve prefix matching. No row data is modified and the
  migration is fully reversible. The opclass is redundant under this
  deployment's C collation but keeps the index correct on a non-C database.

### Changed

- **API internals**: Filter construction shared by the file and folder queries
  moved into one builder, so folder counts cannot disagree with folder
  contents. Query assembly now has test coverage; previously the test double
  discarded SQL entirely.

## [v0.2.0] - 2026-06-24

### Added

- **Auth**: Shared-secret JWT validation for tokens issued by Reliquary.
  Set `JWT_SECRET` to accept Reliquary-issued bearer tokens.
- **Auth**: Mixed-mode authentication — when both `JWT_SECRET` and
  `OIDC_ISSUER_URL` are configured, Engram validates JWTs locally first and
  falls back to OIDC userinfo validation.
- **Auth tests**: Added unit tests for the combined JWT/OIDC authenticator.
- **API**: Real stats and activity endpoints backed by the database,
  replacing the previous hardcoded placeholder data.

### Changed

- **Auth architecture**: Engram no longer acts as an OAuth2/OIDC client.
  The `/api/auth/config`, `/api/auth/oidc/discovery`, and
  `/api/auth/oidc/token` endpoints have been removed. Engram now only
  validates bearer tokens supplied by callers.
- **Config**: Simplified environment variables.
  - Removed: `AUTH_MODE`, `OIDC_PUBLIC_ISSUER_URL`, `OIDC_CLIENT_ID`,
    `OIDC_REDIRECT_URI`.
  - Added: `JWT_SECRET`.
  - Retained: `OIDC_ISSUER_URL`, `OIDC_USERNAME_CLAIM`.
- **Docs**: Updated `README.md` and `CLAUDE.md` to describe the new
  token-validation model and mixed-mode env vars.

## [v0.1.0] - earlier

- Initial tracked release.
