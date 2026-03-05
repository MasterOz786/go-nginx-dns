## Base URL

- Default: `http://<host>:8080`

---

## Health

### GET `/health`

- **Description:** Health check endpoint.
- **Request:** No body.
- **Responses:**
  - `200 OK`

    ```json
    {
      "status": "OK"
    }
    ```

---

## Certificates

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
      "error": "<certbot output>"
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

## Storage / Site Files

All storage endpoints require that a valid (non-expired, matching) certificate exists for the given `domain`. If not, a `400` error is returned with a message from the certificate verification.

### POST `/storage/store`

- **Description:** Upload site files (including zipped folders) and store them under:
  - `/var/www/html/sites/<domain>` if that directory exists, or
  - `/var/www/html` otherwise.

- **Request (multipart/form-data):**
  - Fields:
    - `domain` (text, required)
    - `files` (one or more file parts)
      - Normal files are written directly to the target directory.
      - `.zip` archives are extracted into the target directory; the archive itself is not kept.

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

- **Description:** Generate an HTTPS nginx config for the domain, write it to `/etc/nginx/sites-available`, create a symlink in `/etc/nginx/sites-enabled`, optionally store additional site files, and reload nginx.

- **Request (multipart/form-data):**
  - Fields:
    - `domain` (text, required)
    - `index` (text, optional) – desired index file name; defaults to `index.html`.
    - `files` (optional; same handling as `/storage/store`, supports `.zip`).

- **Successful responses:**
  - `200 OK`

    ```json
    {
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

## Sites API

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

