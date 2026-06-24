# Changelog

All notable changes to Engram are documented in this file.

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
