# Build a URL shortener in Go

This is the only document you get. Build the service it describes in the current folder.
When you are done, say DONE and summarize what you built in a few sentences.

## Rules
- Go 1.24, **standard library only** (no third-party modules; the network is off).
- Module name `shortener`. The program is in the folder root: `go build -o shortener .` must work.
- Write your own tests (`go test ./...`).
- You have tools to list, read and write files and to run `go build`, `go test` and `go vet`. Nothing else.

## Running
```
ADMIN_TOKEN=secret ./shortener -addr :8080 -data data.json
```
- `-addr` default `:8080`. `-data` default `data.json`.
- If `ADMIN_TOKEN` is empty or unset, print an error and exit with a non-zero code.

## API
Every response body is JSON with `Content-Type: application/json`, except redirects and 204s.
Every error is `{"error": "<message>"}` with the status code listed below.

### Create a link — `POST /api/links` (no auth)
Request: `{"url": "https://example.com/page", "alias": "my-page"}` (`alias` is optional)

- `url` must be an absolute `http` or `https` URL with a host, at most 2048 characters. Otherwise **400**.
- `alias`, if given, must match `^[A-Za-z0-9_-]{3,32}$` and must not be `api`. Otherwise **400**.
- Alias already taken: **409**.
- Without an alias, generate a random code of 7 characters from `[A-Za-z0-9]`, unique among existing links.
- Malformed JSON or unknown fields: **400**. Request body over 1 MiB: **413**.
- Success: **201** with `{"code": "...", "url": "...", "created_at": "<RFC 3339>", "visits": 0}`

### Follow a link — `GET /{code}` (no auth)
- **302** with `Location` set to the stored URL, and the link's `visits` goes up by one.
- Unknown code: **404**.

### List links — `GET /api/links` (admin)
- **200** with a JSON array of every link (same fields as above), oldest first.

### Delete a link — `DELETE /api/links/{code}` (admin)
- **204** on success. Unknown code: **404**. Afterwards `GET /{code}` returns **404**.

### Admin auth
Admin requests send `Authorization: Bearer <ADMIN_TOKEN>`. Missing or wrong token: **401**.

Any other method on a known path: **405** (a HEAD request may be answered like the matching GET).

## Storage and concurrency
- Links (including visit counts) are stored in the `-data` JSON file and must survive a restart.
- Every change (a new link, a visit, a delete) is saved to the file before its response is sent.
- A crash or kill at any moment must never leave a file the service can't start from.
- The service must be correct when many requests arrive at the same time: no lost links,
  no lost visit counts, no duplicate codes, no data races.
