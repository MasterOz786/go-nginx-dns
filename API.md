## 1. External Custom-Domain / Sites API (port 8080)

- **Base URL:** `http://<CUSTOM_DOMAIN_SERVER_IP>:8080`
- In your current environment: `http://89.167.105.57:8080`
- No auth is defined here; this service is intended to run on an internal/private network.

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
ddd
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
      "stored": [
        "index.html",
        "assets.zip (unzipped)"
      ],
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

- **Description:** Generate an HTTPS nginx config for the domain, write it to `/etc/nginx/sites-available`, create a symlink in `/etc/nginx/sites-enabled`, ensure the per-site root directory exists at `/var/www/html/sites/<domain>`, optionally store additional site files into that directory, and reload nginx.

- **Request (multipart/form-data):**
  - Fields:
    - `domain` (text, required)
    - `index` (text, optional) – desired index file name; defaults to `index.html`.
    - `files` (optional; same semantics as `/storage/store`, supports `.zip`):
      - Send one or more files under the `files` field.
      - `.zip` archives are extracted into `/var/www/html/sites/<domain-without-www>`; the archive itself is not kept.
      - Zipped subpaths are preserved, and the response will include `"yourfile.zip (unzipped)"` to indicate success.

- **Example (curl):**

  ```bash
  curl -X POST http://<CUSTOM_DOMAIN_SERVER_IP>:8080/storage/nginx \
    -F "domain=example.com" \
    -F "index=index.html" \
    -F "files=@./site-bundle.zip"
  ```

- **Successful responses:**
  - `200 OK`

    ```json
    {
      "status": "success",
      "domain": "example.com",
      "path": "/var/www/html/sites/example.com",
      "nginx_conf": "example.com.conf",
      "sites_available": "/etc/nginx/sites-available/example.com.conf",
      "sites_enabled": "/etc/nginx/sites-enabled/example.com.conf",
      "index_file": "index.html",
      "stored": [
        "example.com.conf",
        "index.html"
      ],
      "cert_path": "/etc/letsencrypt/live/example.com/fullchain.pem",
      "key_path": "/etc/letsencrypt/live/example.com/privkey.pem",
      "message": "Nginx configuration generated and files stored successfully",
      "failed": [
        "badfile.js: <error message>"
      ],
      "nginx_test": "nginx: the configuration file ...",
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

  - `500 Internal Server Error` (for example, `nginx -t` failure)

    ```json
    {
      "status": "error",
      "error": "nginx -t failed: <output>",
      "details": {
        "status": "success",
        "domain": "example.com",
        "path": "/var/www/html",
        "nginx_conf": "example.com.conf",
        "sites_available": "/etc/nginx/sites-available/example.com.conf",
        "sites_enabled": "/etc/nginx/sites-enabled/example.com.conf",
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
