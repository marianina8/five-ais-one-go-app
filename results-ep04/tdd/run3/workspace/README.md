# URL Shortener Service

A simple, concurrent URL shortener service written in Go with JSON file persistence.

## Features

- Create short links with custom aliases or auto-generated 7-character codes
- Redirect to original URLs with visit tracking
- Admin functionality to list and delete links
- JSON file storage with atomic writes for data safety
- Thread-safe concurrent access
- RESTful API

## Building

```bash
go build -o shortener .
```

## Running

```bash
ADMIN_TOKEN=your-secret-token ./shortener -addr :8080 -data data.json
```

### Options
- `-addr`: HTTP address to listen on (default: `:8080`)
- `-data`: JSON file path for data storage (default: `data.json`)
- `ADMIN_TOKEN`: Environment variable containing the admin authentication token (required)

## API Reference

### Create a Link
```
POST /api/links
Content-Type: application/json

{
  "url": "https://example.com",
  "alias": "my-page"  # optional, 3-32 chars, alphanumeric + dash/underscore
}
```

**Response**: `201 Created`
```json
{
  "code": "my-page",
  "url": "https://example.com",
  "created_at": "2024-01-01T12:00:00Z",
  "visits": 0
}
```

**Error codes**:
- `400`: Invalid URL, invalid alias, or malformed JSON
- `409`: Alias already taken
- `413`: Request body too large (>1 MiB)

### Follow a Link
```
GET /{code}
```
**Response**: `302 Found` with `Location` header set to the stored URL

### List Links (Admin)
```
GET /api/links
Authorization: Bearer <ADMIN_TOKEN>
```
**Response**: `200 OK` with array of all links, oldest first

### Delete a Link (Admin)
```
DELETE /api/links/{code}
Authorization: Bearer <ADMIN_TOKEN>
```
**Response**: `204 No Content` on success

## Data Storage

Links are stored in a JSON file specified by the `-data` flag. The file uses atomic writes (write to temp file then rename) to prevent data corruption.

Example `data.json`:
```json
[
  {
    "code": "google",
    "url": "https://google.com",
    "created_at": "2024-01-01T12:00:00Z",
    "visits": 42
  }
]
```

## Validation Rules

- URLs must be absolute HTTP/HTTPS URLs with a host, max 2048 characters
- Aliases must match `^[A-Za-z0-9_-]{3,32}$` and cannot start or end with `-` or `_`
- Aliases cannot be "api" (case-insensitive)
- Random codes are 7 characters from `[A-Za-z0-9]` and are unique

## Concurrency

The service uses read-write locks to ensure thread safety. All modifications (create, increment visits, delete) are persisted to disk before responding.

## Tests

Run tests with:
```bash
go test ./...
```

Includes unit tests for store operations, validation, and integration tests for the HTTP API.