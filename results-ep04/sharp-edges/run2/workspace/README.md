# URL Shortener Service

A simple, thread-safe URL shortener service written in Go with JSON file storage.

## Features

- Create short URLs with optional custom aliases
- Automatic random code generation (7 alphanumeric characters)
- Redirect tracking with visit counts
- Admin API for listing and deleting links
- Persistent storage to JSON file
- Thread-safe operations with proper locking
- Atomic file writes to prevent corruption

## Building

```bash
go build -o shortener .
```

## Running

```bash
ADMIN_TOKEN=secret ./shortener -addr :8080 -data data.json
```

### Arguments

- `-addr`: Address to listen on (default: `:8080`)
- `-data`: Path to JSON data file (default: `data.json`)

### Environment Variables

- `ADMIN_TOKEN`: Required admin token for admin operations

## API

### Create a Link

```bash
POST /api/links
Content-Type: application/json

{
  "url": "https://example.com/page",
  "alias": "my-page"  # optional
}
```

Response:
```json
{
  "code": "my-page",
  "url": "https://example.com/page",
  "created_at": "2024-01-01T00:00:00Z",
  "visits": 0
}
```

### Follow a Link

```bash
GET /{code}
```

Returns 302 redirect to the stored URL and increments visit count.

### List Links (Admin)

```bash
GET /api/links
Authorization: Bearer <ADMIN_TOKEN>
```

Returns array of all links, sorted by creation date (oldest first).

### Delete a Link (Admin)

```bash
DELETE /api/links/{code}
Authorization: Bearer <ADMIN_TOKEN>
```

Returns 204 on success.

## Error Responses

All errors return JSON with error message:

```json
{
  "error": "error message"
}
```

## Validation Rules

- URL must be absolute HTTP/HTTPS URL with host
- URL max length: 2048 characters
- Alias (if provided): must match `^[A-Za-z0-9_-]{3,32}$`
- Alias cannot be "api"
- Request body max size: 1 MiB

## Concurrency & Data Safety

- Thread-safe operations using read/write locks
- Atomic file writes (write to temp file, then rename)
- Rollback on save failures
- Background visit counting to not block redirects
- Crash-safe: service can always start from data file

## Testing

Run all tests:

```bash
go test ./...
```

## License

Standard Go project structure.