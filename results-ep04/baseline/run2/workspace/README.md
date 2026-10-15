# URL Shortener Service

A simple URL shortener service written in Go with JSON file storage.

## Building

```bash
go build -o shortener .
```

## Running

```bash
ADMIN_TOKEN=secret ./shortener -addr :8080 -data data.json
```

- `-addr`: Address to listen on (default: `:8080`)
- `-data`: JSON file for data storage (default: `data.json`)
- `ADMIN_TOKEN`: Environment variable containing the admin authentication token (required)

## API

### Create a link
```
POST /api/links
Content-Type: application/json

{
  "url": "https://example.com/page",
  "alias": "my-page"  # optional
}
```

Response (201 Created):
```json
{
  "code": "my-page",
  "url": "https://example.com/page",
  "created_at": "2024-01-01T00:00:00Z",
  "visits": 0
}
```

### Follow a link
```
GET /{code}
```

Redirects (302 Found) to the stored URL and increments the visit count.

### List all links (admin)
```
GET /api/links
Authorization: Bearer <ADMIN_TOKEN>
```

Response (200 OK):
```json
[
  {
    "code": "my-page",
    "url": "https://example.com/page",
    "created_at": "2024-01-01T00:00:00Z",
    "visits": 5
  }
]
```

### Delete a link (admin)
```
DELETE /api/links/{code}
Authorization: Bearer <ADMIN_TOKEN>
```

Response: 204 No Content

## Error Responses

All error responses are JSON with format:
```json
{"error": "error message"}
```

### Status Codes
- 400: Bad Request (invalid input)
- 401: Unauthorized (missing or invalid admin token)
- 404: Not Found (unknown code)
- 405: Method Not Allowed
- 409: Conflict (alias already taken)
- 413: Request Entity Too Large (> 1 MiB)
- 500: Internal Server Error

## Validation Rules

- URL must be absolute HTTP or HTTPS URL with a host
- URL maximum length: 2048 characters
- Alias (if provided) must match regex: `^[A-Za-z0-9_-]{3,32}$`
- Alias cannot be "api"
- Without alias, a random 7-character code is generated from [A-Za-z0-9]

## Storage

- Links are stored in the JSON file specified by `-data` flag
- Every change (create, visit, delete) is persisted before responding
- File writes are atomic (uses temporary file + rename)
- Thread-safe concurrent access

## Testing

```bash
go test ./...
```