# URL Shortener Service

A simple URL shortener service written in Go using only the standard library.

## Building and Running

```bash
# Build
go build -o shortener .

# Run (ADMIN_TOKEN is required)
ADMIN_TOKEN=secret ./shortener -addr :8080 -data data.json

# With custom options
ADMIN_TOKEN=my-secret-token ./shortener -addr :3000 -data /var/lib/shortener/data.json
```

## API Endpoints

### Create a Link
```
POST /api/links
Content-Type: application/json

{
  "url": "https://example.com/page",
  "alias": "my-page"  # optional, 3-32 chars [A-Za-z0-9_-], not "api"
}

Response (201):
{
  "code": "my-page",
  "url": "https://example.com/page",
  "created_at": "2024-01-01T12:00:00Z",
  "visits": 0
}
```

If no alias is provided, a random 7-character code is generated.

### Follow a Link
```
GET /{code}
Response: 302 redirect to the stored URL
```

### List All Links (Admin)
```
GET /api/links
Authorization: Bearer <ADMIN_TOKEN>

Response (200):
[
  {
    "code": "my-page",
    "url": "https://example.com/page",
    "created_at": "2024-01-01T12:00:00Z",
    "visits": 42
  },
  ...
]
```

Links are returned oldest first.

### Delete a Link (Admin)
```
DELETE /api/links/{code}
Authorization: Bearer <ADMIN_TOKEN>

Response: 204 No Content
```

## Error Responses

All errors return JSON:
```json
{"error": "Error message"}
```

- **400 Bad Request**: Invalid input, malformed JSON, validation errors
- **401 Unauthorized**: Missing or invalid admin token
- **404 Not Found**: Link not found
- **409 Conflict**: Alias already taken
- **413 Request Entity Too Large**: Request body > 1 MiB
- **405 Method Not Allowed**: Unsupported HTTP method

## Data Storage

Links are stored in a JSON file specified by the `-data` flag. The file is updated atomically on every change (create, visit increment, delete) to ensure data integrity even if the service crashes.

## Concurrency

The service is thread-safe and handles concurrent requests correctly using proper locking mechanisms.

## Testing

```bash
# Run all tests
go test ./...

# Build and vet
go build .
go vet ./...
```

## Dependencies

Standard library only - no external dependencies.