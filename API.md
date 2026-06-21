## 1. External Custom-Domain / Sites API (port 8080)

- **Base URL:** `http://<CUSTOM_DOMAIN_SERVER_IP>:8080`
- In your current environment: `http://89.167.105.57:8080`
- No auth is defined here; this service is intended to run on an internal/private network.

**Environment variables:**

| Variable | Default | Purpose |
|----------|---------|---------|
| `SITE_BASE_PATH` | `/var/www/html/sites` | Root directory for per-domain site files |
| `NGINX_CONF_DIR` | *(auto-detected)* | Optional override; skips auto-discovery and writes vhost configs directly to this directory |

**Nginx layout discovery**

`/storage/nginx` does not hardcode a config path. At request time the service:

1. Runs `nginx -V` to locate the main config file (for example `/etc/nginx/nginx.conf`).
2. Parses `include` directives in that file (and one level of nested non-glob includes).
3. Chooses a write strategy based on what nginx actually loads:

| Detected include pattern | `nginx_layout` | Config written to | Symlink |
|--------------------------|----------------|-------------------|---------|
| `conf.d/*.conf` | `conf.d` | `/etc/nginx/conf.d/<domain>.conf` | none |
| `sites-enabled/*` | `sites-available` | `/etc/nginx/sites-available/<domain>.conf` | → `sites-enabled/` |
| `NGINX_CONF_DIR` set | `override` | `$NGINX_CONF_DIR/<domain>.conf` | none |

4. Falls back to checking which of those directories exist if parsing fails.

Typical results by platform:

- **Amazon Linux / RHEL / EC2:** `conf.d` → `/etc/nginx/conf.d/`
- **Debian / Ubuntu:** `sites-available` → `/etc/nginx/sites-available/` + symlink in `/etc/nginx/sites-enabled/`

**Verify layout on a host**

Run on the target server before deploying:

```bash
# integration test (prints JSON report, checks dirs + nginx -t)
go test -v -run TestNginxLayoutEnvironment ./internal/handlers/

# or standalone JSON report
go run ./cmd/nginx-layout-check/
```

Set `NGINX_LAYOUT_ENV_STRICT=1` with the test to fail when `NGINX_CONF_DIR` override is active (forces verification of auto-discovery).

Example `go run ./cmd/nginx-layout-check/` output on EC2:

```json
{
  "main_conf": "/etc/nginx/nginx.conf",
  "nginx_root": "/etc/nginx",
  "includes": [
    "/etc/nginx/mime.types",
    "/etc/nginx/conf.d/*.conf"
  ],
  "write_dir": "/etc/nginx/conf.d",
  "layout": "conf.d",
  "source": "include conf.d/*.conf",
  "write_dir_exists": true,
  "write_dir_writable": true,
  "enable_dir_exists": false,
  "sample_config_path": "/etc/nginx/conf.d/example.com.conf",
  "nginx_test_ok": true,
  "nginx_test_output": "nginx: configuration file /etc/nginx/nginx.conf test is successful"
}
```

---

## 1.1 Health

### GET `/health`

- **Description:** Health check.
- **Request:** No body.
- **Responses:**
  - `200 OK`

    ```json
    {
      "status": "OK"
    }
    ```

---

## 1.2 Certificates

### POST `/cert/generate`

- **Description:** Create a new certificate via `certbot --nginx`.
- **Request body (JSON):**

  ```json
  {
    "domain": "example.com",
    "email": "admin@example.com"
  }
  ```

  - `domain` (string, required)
  - `email` (string, optional; defaults to `admin@<domain>` if omitted)

- **Responses:**
  - `200 OK`

    ```json
    {
      "status": "success",
      "message": "Certificate generated successfully",
      "output": "<raw certbot output>"
    }
    ```

  - `400 Bad Request` (validation error)

    ```json
    {
      "status": "error",
      "error": "domain is required"
    }
    ```

  - `500 Internal Server Error` (certbot failure)

    ```json
    {
      "status": "error",
      "error": "<certbot output or error>"
    }
    ```

---

### POST `/cert/renew?domain=<domain>`

- **Description:** Renew an existing certificate.
- **Query parameters:**
  - `domain` (string, required) – certificate name.
- **Request body:** None.
- **Responses:**
  - `200 OK`

    ```json
    {
      "status": "success",
      "message": "Certificate renewed successfully",
      "domain": "example.com",
      "output": "<certbot output>"
    }
    ```

  - `400 Bad Request`

    ```json
    {
      "status": "error",
      "error": "domain parameter required"
    }
    ```

  - `500 Internal Server Error`

    ```json
    {
      "status": "error",
      "error": "Renewal failed: <details>\nOutput: <certbot output>"
    }
    ```

---

### DELETE `/cert/delete?domain=<domain>`

- **Description:** Delete a certificate.
- **Query parameters:**
  - `domain` (string, required)
- **Request body:** None.
- **Responses:**
  - `200 OK`

    ```json
    {
      "status": "success",
      "message": "Certificate deleted successfully",
      "domain": "example.com"
    }
    ```

  - `400 Bad Request`

    ```json
    {
      "status": "error",
      "error": "domain parameter required"
    }
    ```

  - `500 Internal Server Error`

    ```json
    {
      "status": "error",
      "error": "Deletion failed: <details>\nOutput: <certbot output>"
    }
    ```

---

### GET `/cert/list`

- **Description:** List all certificates known to certbot.
- **Request:** None.
- **Responses:**
  - `200 OK`

    ```json
    {
      "status": "success",
      "certificates": [
        {
          "name": "example.com",
          "domains": ["example.com", "www.example.com"],
          "expiry": "Feb 20 12:34:56 2025 GMT",
          "cert_path": "/etc/letsencrypt/live/example.com/fullchain.pem",
          "key_path": "/etc/letsencrypt/live/example.com/privkey.pem"
        }
      ]
    }
    ```
  - `500 Internal Server Error`

    ```json
    {
      "status": "error",
      "error": "Failed to list certificates: <details>"
    }
    ```

---

## 1.3 Storage / Site Files

All storage endpoints require that a valid (non-expired, matching) certificate exists for the given `domain`. If not, a `400` error is returned with a message from the certificate verification.

### POST `/storage/store`

- **Description:** Upload site files (including zipped folders) and store them under:
  - `/var/www/html/sites/<domain>` if that directory exists, or
  - `/var/www/html` otherwise.

- **Request (multipart/form-data):**
  - Fields:
    - `domain` (text, required)
    - `mode` (text, optional) - `upsert` (default) or `replace_all`
      - `upsert`: overwrite matching uploaded paths and keep other existing files.
      - `replace_all`: clear all existing files under the domain directory before writing new uploads.
    - `files` (one or more file parts; this **must** be the field name)
      - Normal files are written directly to the target directory.
      - `.zip` archives are extracted into the target directory; the archive itself is not kept.
      - Paths inside the zip are preserved (for example, `dist/index.html` becomes `<storageDir>/dist/index.html`).
      - Each processed zip is reported in the response as `"yourfile.zip (unzipped)"`; individual unzipped files are not listed separately.

- **Example (curl):**

  ```bash
  curl -X POST http://<CUSTOM_DOMAIN_SERVER_IP>:8080/storage/store \
    -F "domain=example.com" \
    -F "files=@./site-bundle.zip"
  ```

- **Responses:**
  - `200 OK`

    ```json
    {
      "status": "success",
      "domain": "example.com",
      "path": "/var/www/html/sites/example.com",
      "mode": "upsert",
      "stored": [
        "index.html",
        "assets.zip (unzipped)"
      ],
      "summary": {
        "created": ["index.html"],
        "updated": ["assets/app.js"],
        "deleted": [],
        "unchanged": ["favicon.ico"]
      },
      "message": "Files stored successfully after certificate verification",
      "failed": [
        "badfile.js: <error message>"
      ]
    }
    ```

  - `400 Bad Request`

    ```json
    {
      "status": "error",
      "error": "<reason: e.g. domain parameter required | Certificate is not verified for domain: ... | no files provided | file-specific error>"
    }
    ```

---

### POST `/storage/nginx`

- **Description:** Generate an HTTPS nginx config for the domain, auto-detect the correct nginx vhost directory from the running nginx installation, write `<domain>.conf` there (and symlink into `sites-enabled` when that layout is used), ensure the per-site root directory exists at `/var/www/html/sites/<domain>`, optionally store additional site files into that directory, run `nginx -t`, and reload nginx.

- **Request (multipart/form-data):**
  - Fields:
    - `domain` (text, required)
    - `index` (text, optional) – desired index file name; defaults to `index.html`.
    - `mode` (text, optional) - `upsert` (default) or `replace_all`
      - `upsert`: overwrite matching uploaded paths and keep other existing files.
      - `replace_all`: clear all existing files under the domain directory before writing new uploads.
    - `files` (optional; same semantics as `/storage/store`, supports `.zip`):
      - Send one or more files under the `files` field.
      - `.zip` archives are extracted into `/var/www/html/sites/<domain-without-www>`; the archive itself is not kept.
      - Zipped subpaths are preserved, and the response will include `"yourfile.zip (unzipped)"` to indicate success.

- **Response fields (success):**

  | Field | Type | Description |
  |-------|------|-------------|
  | `nginx_conf` | string | Config filename (for example `example.com.conf`) |
  | `nginx_conf_path` | string | Absolute path where the config file was written |
  | `nginx_conf_dir` | string | Directory used for vhost configs (discovered or overridden) |
  | `nginx_layout` | string | `"conf.d"`, `"sites-available"`, or `"override"` |
  | `nginx_layout_src` | string | How the layout was determined (matched `include` directive or fallback reason) |
  | `nginx_enabled_path` | string | Present on Debian-style layouts; symlink path in `sites-enabled` |
  | `nginx_test` | string | Output of `nginx -t` |
  | `nginx_reload` | string | Reload result (`success (systemctl reload nginx)`, `success (nginx -s reload)`, or `failed (...)`) |

- **Example (curl):**

  ```bash
  curl -X POST http://<CUSTOM_DOMAIN_SERVER_IP>:8080/storage/nginx \
    -F "domain=example.com" \
    -F "index=index.html" \
    -F "files=@./site-bundle.zip"
  ```

- **Successful responses:**
  - `200 OK` — **conf.d layout** (Amazon Linux / RHEL / EC2)

    ```json
    {
      "status": "success",
      "domain": "example.com",
      "path": "/var/www/html/sites/example.com",
      "mode": "replace_all",
      "nginx_conf": "example.com.conf",
      "nginx_conf_path": "/etc/nginx/conf.d/example.com.conf",
      "nginx_conf_dir": "/etc/nginx/conf.d",
      "nginx_layout": "conf.d",
      "nginx_layout_src": "include /etc/nginx/conf.d/*.conf",
      "index_file": "index.html",
      "stored": [
        "example.com.conf",
        "index.html"
      ],
      "cert_path": "/etc/letsencrypt/live/example.com/fullchain.pem",
      "key_path": "/etc/letsencrypt/live/example.com/privkey.pem",
      "summary": {
        "created": ["index.html", "assets/app.js"],
        "updated": [],
        "deleted": ["old.bundle.js"],
        "unchanged": []
      },
      "message": "Nginx configuration generated and files stored successfully",
      "failed": [
        "badfile.js: <error message>"
      ],
      "nginx_test": "nginx: the configuration file /etc/nginx/nginx.conf test is successful",
      "nginx_reload": "success (systemctl reload nginx)"
    }
    ```

  - `200 OK` — **sites-available layout** (Debian / Ubuntu)

    ```json
    {
      "status": "success",
      "domain": "example.com",
      "path": "/var/www/html/sites/example.com",
      "mode": "upsert",
      "nginx_conf": "example.com.conf",
      "nginx_conf_path": "/etc/nginx/sites-available/example.com.conf",
      "nginx_conf_dir": "/etc/nginx/sites-available",
      "nginx_enabled_path": "/etc/nginx/sites-enabled/example.com.conf",
      "nginx_layout": "sites-available",
      "nginx_layout_src": "include /etc/nginx/sites-enabled/*",
      "index_file": "index.html",
      "stored": ["example.com.conf", "index.html"],
      "cert_path": "/etc/letsencrypt/live/example.com/fullchain.pem",
      "key_path": "/etc/letsencrypt/live/example.com/privkey.pem",
      "summary": {
        "created": ["index.html"],
        "updated": [],
        "deleted": [],
        "unchanged": []
      },
      "message": "Nginx configuration generated and files stored successfully",
      "nginx_test": "nginx: the configuration file /etc/nginx/nginx.conf test is successful",
      "nginx_reload": "success (systemctl reload nginx)"
    }
    ```

- **Error responses:**
  - `400 Bad Request` (examples)

    ```json
    {
      "status": "error",
      "error": "domain parameter required"
    }
    ```

    ```json
    {
      "status": "error",
      "error": "Certificate is not verified for domain: example.com"
    }
    ```

    ```json
    {
      "status": "error",
      "error": "invalid mode \"replace\", expected upsert or replace_all"
    }
    ```

  - `500 Internal Server Error` (for example, layout discovery failure or `nginx -t` failure)

    ```json
    {
      "status": "error",
      "error": "could not determine nginx config directory: ..."
    }
    ```

    ```json
    {
      "status": "error",
      "error": "nginx -t failed: <output>",
      "details": {
        "status": "success",
        "domain": "example.com",
        "path": "/var/www/html/sites/example.com",
        "nginx_conf": "example.com.conf",
        "nginx_conf_path": "/etc/nginx/conf.d/example.com.conf",
        "nginx_conf_dir": "/etc/nginx/conf.d",
        "nginx_layout": "conf.d",
        "nginx_layout_src": "include /etc/nginx/conf.d/*.conf",
        "index_file": "index.html",
        "stored": [
          "example.com.conf",
          "index.html"
        ],
        "cert_path": "/etc/letsencrypt/live/example.com/fullchain.pem",
        "key_path": "/etc/letsencrypt/live/example.com/privkey.pem",
        "message": "Nginx configuration generated and files stored successfully",
        "nginx_test": "nginx: [emerg] ...",
        "nginx_reload": "skipped"
      }
    }
    ```

---

## 1.4 Sites API

These endpoints work against the directory tree under `SITE_BASE_PATH` (default `/var/www/html/sites`).

### GET `/sites`

- **Description:** List top-level entries (directories and files) under `sitesBasePath`.
- **Request:** None.
- **Responses:**
  - `200 OK`

    ```json
    {
      "status": "success",
      "sites": [
        "example.com",
        "another-site",
        "shared-theme.json"
      ]
    }
    ```

  - `500 Internal Server Error`

    ```json
    {
      "status": "error",
      "error": "<details>"
    }
    ```

---

### GET `/sites/info/:site`

- **Description:** Discover resources associated with a site name (html/json/theme/manifest + directory listing).
- **Path parameters:**
  - `site` – logical site name (for example, `example.com`).

- **Responses:**
  - `200 OK`

    ```json
    {
      "status": "success",
      "site": "example.com",
      "info": {
        "html": "example.com.html",
        "json": { /* parsed contents of example.com.json */ },
        "default_theme": { /* parsed contents of example.com.default-theme.json */ },
        "manifest": { /* parsed contents of example.com-manifest.json */ },
        "directory": [
          "index.html",
          "assets",
          "example.com.json"
        ]
      }
    }
    ```

  - `404 Not Found`

    ```json
    {
      "status": "error",
      "error": "no resources found for site"
    }
    ```

---

### GET `/sites/:site/*filepath`

- **Description:** Serve site content or list a directory for a path within a site.
- **Path parameters:**
  - `site` – root site name.
  - `filepath` – optional path under that site (starts with `/`).

- **Behavior:**
  - If target is a file, the file is served directly.
  - If target is a directory:
    - If `index.html` exists, it is served.
    - Otherwise a directory listing JSON is returned.

- **Example responses:**
  - Directory listing `200 OK`

    ```json
    {
      "status": "success",
      "path": "example.com/assets",
      "entries": ["app.js", "logo.png"]
    }
    ```

  - Not found

    ```json
    {
      "status": "error",
      "error": "not found"
    }
    ```

  - Invalid path traversal

    ```json
    {
      "status": "error",
      "error": "invalid path"
    }
    ```

---

## 2. Skillbanto App API (Custom Domains & Deployment)

- **Base URLs (app):**
  - Local dev: usually `http://localhost:5001`
  - Production: `https://app.skillbanto.com`
- All `/api/creator/...` routes require an authenticated creator (cookie-based session).

These endpoints live in the Skillbanto app and orchestrate calls to the external custom-domain API above.

---

## 2.1 Creator custom-domain status & instructions

- **File:** `server/routes/creator/customDomainRoutes.ts`
- **Mount path:** `/api/creator/...`

### GET `/api/creator/site/custom-domain`

- **Description:** Get current custom-domain settings and DNS instructions for the logged-in creator.
- **Responses:**
  - `200 OK`

    ```json
    {
      "customDomain": "foziakashif.online",
      "status": "connected",
      "verifiedAt": "2025-02-12T10:23:45.000Z",
      "instructions": {
        "cnameTarget": "mysub.skillbanto.com",
        "apexTargets": ["89.167.105.57"],
        "verificationHost": "verify.foziakashif.online",
        "verificationCode": "sb-verify-abc123"
      },
      "guidance": null
    }
    ```

  - `4xx/5xx`:

    ```json
    {
      "error": "Failed to fetch custom domain",
      "details": "<message>"
    }
    ```

---

## 2.2 Connect a creator site to a custom domain

### POST `/api/creator/site/custom-domain/connect`

- **Description:** Connect and deploy a creator site to a custom domain. This endpoint:
  - Validates the domain.
  - Optionally verifies DNS.
  - Saves the domain to `users.customDomain`.
  - Calls the external API (`/cert/generate` and `/storage/nginx`) via `deploySiteToCustomDomain`.
  - Marks status as `"connected"`.

- **Request (JSON):**

  ```json
  {
    "domain": "foziakashif.online",
    "skipDnsCheck": false
  }
  ```

  - `domain` (string, required)
  - `skipDnsCheck` (boolean, optional) – if true, skips DNS verification.

- **Responses:**
  - `200 OK`

    ```json
    {
      "success": true,
      "domain": "foziakashif.online",
      "status": "connected"
    }
    ```

  - Possible `4xx/5xx`:

    ```json
    {
      "error": "domain is required"
    }
    ```

    ```json
    {
      "error": "Enter a valid domain like learn.mybrand.com"
    }
    ```

    ```json
    {
      "error": "DNS not configured correctly for this domain",
      "details": []
    }
    ```

    ```json
    {
      "error": "Failed to connect custom domain",
      "details": "<error message>"
    }
    ```

---

## 2.3 Disconnect a custom domain

### DELETE `/api/creator/site/custom-domain`

- **Description:** Clear the custom-domain mapping on the creator.
- **Responses:**
  - `200 OK`

    ```json
    {
      "success": true
    }
    ```

- Side effects:
  - `customDomain = null`
  - `customDomainStatus = "not_connected"`
  - `customDomainVerificationCode = null`
  - `customDomainVerifiedAt = null`

---

## 2.4 Creator profile (logo, favicon, subdomain, etc.)

- **File:** `server/routes/creator/gamificationRoutes.ts`

### GET `/api/creator/profile`

- **Description:** Fetch basic creator/site settings.
- **Response `200 OK` (simplified):**

  ```json
  {
    "subDomain": "dfiles",
    "logo": "https://doip65r0xfpnv.cloudfront.net/.../logo.png",
    "favicon": "https://doip65r0xfpnv.cloudfront.net/.../favicon.png",
    "colorPalette": ["#2C3E50"],
    "name": "Creator Name",
    "title": "Tagline",
    "profileImage": "...",
    "bio": "...",
    "customDomain": "rozeena.online",
    "customDomainStatus": "connected"
  }
  ```

### PUT `/api/creator/profile`

- **Description:** Update creator site/profile settings (subdomain, branding, etc.).
- **Request (partial JSON):**

  ```json
  {
    "subDomain": "dfiles",
    "logo": "https://doip65r0xfpnv.cloudfront.net/.../logo.png",
    "favicon": "https://doip65r0xfpnv.cloudfront.net/.../favicon.png",
    "colorPalette": ["#2C3E50"],
    "name": "Creator Name",
    "title": "Title",
    "profileImage": "...",
    "bio": "...",
    "skillToTeach": "...",
    "productType": "...",
    "description": "...",
    "benefit": "...",
    "ideaName": "...",
    "socialMessage": "..."
  }
  ```

- **Response `200 OK`:**

  ```json
  {
    "success": true,
    "message": "Profile updated successfully",
    "subDomain": "dfiles"
  }
  ```

---

## 2.5 Image uploads for logo / favicon / branding

- **Backend file:** `server/routes/creator/s3Upload.ts`

### POST `/api/creator/s3-upload/image`

- **Description:** Generate a presigned S3 URL and resulting CDN URL for image uploads (logo, favicon, branding).
- **Request (JSON):**

  ```json
  {
    "fileName": "logo.png",
    "contentType": "image/png",
    "prefix": "branding"
  }
  ```

- **Response `200 OK`:**

  ```json
  {
    "success": true,
    "key": "branding/file-<timestamp>-<rand>.png",
    "url": "<presigned-PUT-url>",
    "fileUrl": "https://doip65r0xfpnv.cloudfront.net/branding/file-...png",
    "message": "Image upload URL generated successfully"
  }
  ```

- **Typical flow:**
  1. Frontend calls `/api/creator/s3-upload/image` to get `url` and `fileUrl`.
  2. Frontend `PUT`s the file to `url`.
  3. Frontend stores `fileUrl` in `logo` / `favicon` fields and then calls `PUT /api/creator/profile`.
