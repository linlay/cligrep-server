# Admin API Keys

Admin API keys let scripts, CLI tools, and automation jobs call CLI Grep admin APIs without browser login.

## Create A Key

1. Sign in to `/admin` as a platform admin.
2. Open the **API Keys** section.
3. Enter a key name, such as `release bot`.
4. Click **Issue key**.
5. Copy the generated key immediately.

The full key is shown only once. After leaving the page, it cannot be viewed again.

Example key format:

```text
cg_admin_xxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

## Authenticate Requests

Send the API key as a Bearer token:

```http
Authorization: Bearer <api_key>
```

Production example:

```bash
curl -H "Authorization: Bearer cg_admin_xxxxxxxxx" \
  https://cligrep.com/api/v1/admin/me
```

Local development example:

```bash
curl -H "Authorization: Bearer cg_admin_xxxxxxxxx" \
  http://127.0.0.1:11802/api/v1/admin/me
```

## Example Calls

Check the current admin identity:

```bash
curl -H "Authorization: Bearer cg_admin_xxxxxxxxx" \
  https://cligrep.com/api/v1/admin/me
```

List managed CLI records:

```bash
curl -H "Authorization: Bearer cg_admin_xxxxxxxxx" \
  https://cligrep.com/api/v1/admin/clis
```

Add another admin:

```bash
curl -X POST \
  -H "Authorization: Bearer cg_admin_xxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{"identifier":"username_or_email"}' \
  https://cligrep.com/api/v1/admin/users
```

Revoke an API key:

```bash
curl -X DELETE \
  -H "Authorization: Bearer cg_admin_xxxxxxxxx" \
  https://cligrep.com/api/v1/admin/api-keys/123
```

## Security Notes

- Store API keys securely.
- Never commit API keys to git.
- The server stores only a hash of each key.
- The raw key is displayed only once when created.
- Revoked keys stop working immediately.
- If the user who created the key loses `platform_admin`, the key stops working.
