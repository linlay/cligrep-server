# Admin CLI API

The Admin CLI API lets platform admins manage CLI catalog records, releases, and release artifacts through HTTP. It is intended for internal tools, scripts, and CLI automation.

## Authentication

Use an admin API key as a Bearer token:

```http
Authorization: Bearer <admin_api_key>
```

Example:

```bash
export CLIGREP_API_BASE="https://cligrep.com"
export CLIGREP_ADMIN_API_KEY="cg_admin_xxxxxxxxx"
```

```bash
curl -H "Authorization: Bearer $CLIGREP_ADMIN_API_KEY" \
  "$CLIGREP_API_BASE/api/v1/admin/me"
```

For local development:

```bash
export CLIGREP_API_BASE="http://127.0.0.1:11802"
```

See [admin-api-keys.md](admin-api-keys.md) for issuing and revoking admin API keys.

## CLI Record Fields

Create and update requests use this JSON shape:

```json
{
  "slug": "my-cli",
  "displayName": "My CLI",
  "summary": "Short catalog summary.",
  "helpText": "Longer usage or documentation text.",
  "tags": ["devtool", "automation"],
  "versionText": "1.0.0",
  "exampleLine": "my-cli --help",
  "author": "CLI Grep",
  "officialUrl": "https://example.com/my-cli",
  "giteeUrl": "",
  "license": "MIT",
  "originalCommand": "my-cli",
  "executionTemplate": "download-only"
}
```

Field notes:

- `slug` is required on create and must match `[a-z0-9][a-z0-9._-]{1,127}`.
- `slug` is immutable after creation; update the record by addressing `/api/v1/admin/clis/:slug`.
- `displayName` falls back to the slug when empty.
- `versionText` falls back to `N/A` when empty.
- `exampleLine` falls back to `<slug> --help` when empty.
- `author` falls back to the current admin's display name or username when empty.
- `tags` should be an array of strings.
- `executionTemplate` currently supports:
  - `download-only`: metadata/download-only CLI, not sandbox executable.
  - `busybox-applet`: executable inside the BusyBox sandbox.

New records are created as drafts. Publish them explicitly when ready.

## List CLIs

```http
GET /api/v1/admin/clis
```

```bash
curl -H "Authorization: Bearer $CLIGREP_ADMIN_API_KEY" \
  "$CLIGREP_API_BASE/api/v1/admin/clis"
```

Response:

```json
{
  "items": [
    {
      "slug": "my-cli",
      "displayName": "My CLI",
      "summary": "Short catalog summary.",
      "status": "draft"
    }
  ]
}
```

## Get One CLI

```http
GET /api/v1/admin/clis/:slug
```

```bash
curl -H "Authorization: Bearer $CLIGREP_ADMIN_API_KEY" \
  "$CLIGREP_API_BASE/api/v1/admin/clis/my-cli"
```

Response includes the CLI record, releases, and available execution templates:

```json
{
  "cli": {
    "slug": "my-cli",
    "displayName": "My CLI",
    "status": "draft"
  },
  "releases": [],
  "executionTemplates": [
    {
      "id": "download-only",
      "label": "Download only",
      "environmentKind": "TEXT",
      "executable": false
    }
  ]
}
```

## Create CLI

```http
POST /api/v1/admin/clis
```

```bash
curl -X POST \
  -H "Authorization: Bearer $CLIGREP_ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "slug": "my-cli",
    "displayName": "My CLI",
    "summary": "Short catalog summary.",
    "helpText": "Usage: my-cli [options]",
    "tags": ["devtool", "automation"],
    "versionText": "1.0.0",
    "exampleLine": "my-cli --help",
    "author": "CLI Grep",
    "officialUrl": "https://example.com/my-cli",
    "giteeUrl": "",
    "license": "MIT",
    "originalCommand": "my-cli",
    "executionTemplate": "download-only"
  }' \
  "$CLIGREP_API_BASE/api/v1/admin/clis"
```

Response:

```json
{
  "cli": {
    "slug": "my-cli",
    "displayName": "My CLI",
    "status": "draft"
  }
}
```

## Update CLI

```http
PATCH /api/v1/admin/clis/:slug
```

Send the full desired editable payload. Omitted string fields are treated as empty or fallback values depending on the field.

```bash
curl -X PATCH \
  -H "Authorization: Bearer $CLIGREP_ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "displayName": "My CLI",
    "summary": "Updated catalog summary.",
    "helpText": "Updated usage docs.",
    "tags": ["devtool", "automation", "release"],
    "versionText": "1.0.1",
    "exampleLine": "my-cli --version",
    "author": "CLI Grep",
    "officialUrl": "https://example.com/my-cli",
    "giteeUrl": "",
    "license": "MIT",
    "originalCommand": "my-cli",
    "executionTemplate": "download-only"
  }' \
  "$CLIGREP_API_BASE/api/v1/admin/clis/my-cli"
```

## Publish Or Unpublish CLI

Publish:

```http
POST /api/v1/admin/clis/:slug/publish
```

```bash
curl -X POST \
  -H "Authorization: Bearer $CLIGREP_ADMIN_API_KEY" \
  "$CLIGREP_API_BASE/api/v1/admin/clis/my-cli/publish"
```

Unpublish:

```http
POST /api/v1/admin/clis/:slug/unpublish
```

```bash
curl -X POST \
  -H "Authorization: Bearer $CLIGREP_ADMIN_API_KEY" \
  "$CLIGREP_API_BASE/api/v1/admin/clis/my-cli/unpublish"
```

Published records are visible in public catalog APIs. Draft records are only visible in admin APIs.

## Delete CLI

```http
DELETE /api/v1/admin/clis/:slug
```

```bash
curl -X DELETE \
  -H "Authorization: Bearer $CLIGREP_ADMIN_API_KEY" \
  "$CLIGREP_API_BASE/api/v1/admin/clis/my-cli"
```

Deleting a CLI also removes its database-linked release metadata and release asset files known to the server.

## Release Fields

Release create and update requests use this JSON shape:

```json
{
  "version": "v1.0.0",
  "publishedAt": "2026-06-01T00:00:00Z",
  "isCurrent": true,
  "sourceKind": "manual",
  "sourceUrl": "https://example.com/my-cli/releases/v1.0.0"
}
```

Field notes:

- `version` is required on create.
- `version` is immutable after creation.
- `publishedAt` is required and must be an RFC 3339 timestamp.
- `isCurrent: true` marks this release as current and clears current status from other releases for the same CLI.
- `sourceKind` and `sourceUrl` are free-form metadata fields.

## List Releases

```http
GET /api/v1/admin/clis/:slug/releases
```

```bash
curl -H "Authorization: Bearer $CLIGREP_ADMIN_API_KEY" \
  "$CLIGREP_API_BASE/api/v1/admin/clis/my-cli/releases"
```

## Create Release

```http
POST /api/v1/admin/clis/:slug/releases
```

```bash
curl -X POST \
  -H "Authorization: Bearer $CLIGREP_ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "version": "v1.0.0",
    "publishedAt": "2026-06-01T00:00:00Z",
    "isCurrent": true,
    "sourceKind": "manual",
    "sourceUrl": "https://example.com/my-cli/releases/v1.0.0"
  }' \
  "$CLIGREP_API_BASE/api/v1/admin/clis/my-cli/releases"
```

## Update Release

```http
PATCH /api/v1/admin/clis/:slug/releases/:version
```

```bash
curl -X PATCH \
  -H "Authorization: Bearer $CLIGREP_ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "publishedAt": "2026-06-01T00:00:00Z",
    "isCurrent": true,
    "sourceKind": "manual",
    "sourceUrl": "https://example.com/my-cli/releases/v1.0.0"
  }' \
  "$CLIGREP_API_BASE/api/v1/admin/clis/my-cli/releases/v1.0.0"
```

## Delete Release

```http
DELETE /api/v1/admin/clis/:slug/releases/:version
```

```bash
curl -X DELETE \
  -H "Authorization: Bearer $CLIGREP_ADMIN_API_KEY" \
  "$CLIGREP_API_BASE/api/v1/admin/clis/my-cli/releases/v1.0.0"
```

## Upload Release Asset

```http
POST /api/v1/admin/clis/:slug/releases/:version/assets
```

This endpoint accepts `multipart/form-data`.

Form fields:

- `file`: required binary file.
- `os`: optional target OS, such as `linux`, `darwin`, or `windows`.
- `arch`: optional target architecture, such as `amd64` or `arm64`.
- `packageKind`: optional package kind, such as `tar.gz`, `zip`, or `binary`.
- `checksumUrl`: optional checksum URL.

```bash
curl -X POST \
  -H "Authorization: Bearer $CLIGREP_ADMIN_API_KEY" \
  -F "file=@./dist/my-cli-linux-amd64.tar.gz" \
  -F "os=linux" \
  -F "arch=amd64" \
  -F "packageKind=tar.gz" \
  -F "checksumUrl=https://example.com/my-cli/checksums.txt" \
  "$CLIGREP_API_BASE/api/v1/admin/clis/my-cli/releases/v1.0.0/assets"
```

Response:

```json
{
  "asset": {
    "fileName": "my-cli-linux-amd64.tar.gz",
    "downloadUrl": "https://cligrep.com/cli-releases/my-cli/v1.0.0/my-cli-linux-amd64.tar.gz",
    "os": "linux",
    "arch": "amd64",
    "packageKind": "tar.gz",
    "checksumUrl": "https://example.com/my-cli/checksums.txt",
    "sizeBytes": 1234567
  }
}
```

The Go server accepts multipart uploads up to 64 MB. Any reverse proxy in front of the server must allow a large enough request body, for example `client_max_body_size 20m;` in Nginx for 20 MB uploads.

## Delete Release Asset

```http
DELETE /api/v1/admin/clis/:slug/releases/:version/assets/:assetId
```

```bash
curl -X DELETE \
  -H "Authorization: Bearer $CLIGREP_ADMIN_API_KEY" \
  "$CLIGREP_API_BASE/api/v1/admin/clis/my-cli/releases/v1.0.0/assets/123"
```

## Common Error Responses

| Status | Meaning |
| --- | --- |
| `400` | Invalid JSON, invalid slug, invalid release version, missing file, or invalid execution template. |
| `401` | Missing or invalid session/API key. |
| `403` | Authenticated user is not a platform admin, or the API key creator is no longer a platform admin. |
| `404` | CLI, release, or asset was not found. |
| `409` | CLI slug is already taken. |
| `413` | Upload rejected by reverse proxy because the request body is too large. |

Error responses use this shape:

```json
{
  "error": "forbidden"
}
```

## End-To-End Example

```bash
export CLIGREP_API_BASE="https://cligrep.com"
export CLIGREP_ADMIN_API_KEY="cg_admin_xxxxxxxxx"
```

Create a draft CLI:

```bash
curl -X POST \
  -H "Authorization: Bearer $CLIGREP_ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "slug": "demo-cli",
    "displayName": "Demo CLI",
    "summary": "Demo CLI managed through the admin API.",
    "helpText": "Usage: demo-cli --help",
    "tags": ["demo"],
    "versionText": "v0.1.0",
    "exampleLine": "demo-cli --help",
    "author": "CLI Grep",
    "officialUrl": "https://example.com/demo-cli",
    "license": "MIT",
    "originalCommand": "demo-cli",
    "executionTemplate": "download-only"
  }' \
  "$CLIGREP_API_BASE/api/v1/admin/clis"
```

Add a release:

```bash
curl -X POST \
  -H "Authorization: Bearer $CLIGREP_ADMIN_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "version": "v0.1.0",
    "publishedAt": "2026-06-01T00:00:00Z",
    "isCurrent": true,
    "sourceKind": "manual",
    "sourceUrl": "https://example.com/demo-cli/releases/v0.1.0"
  }' \
  "$CLIGREP_API_BASE/api/v1/admin/clis/demo-cli/releases"
```

Upload an artifact:

```bash
curl -X POST \
  -H "Authorization: Bearer $CLIGREP_ADMIN_API_KEY" \
  -F "file=@./demo-cli.tar.gz" \
  -F "os=linux" \
  -F "arch=amd64" \
  -F "packageKind=tar.gz" \
  "$CLIGREP_API_BASE/api/v1/admin/clis/demo-cli/releases/v0.1.0/assets"
```

Publish the CLI:

```bash
curl -X POST \
  -H "Authorization: Bearer $CLIGREP_ADMIN_API_KEY" \
  "$CLIGREP_API_BASE/api/v1/admin/clis/demo-cli/publish"
```
