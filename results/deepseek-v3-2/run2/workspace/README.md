# URL Shortener Service

A simple URL shortener service built in Go with the standard library only.

## Features

- Create short links with optional custom aliases
- Redirect short codes to original URLs
- Admin authentication for listing and deleting links
- Persistent storage in JSON file
- Thread-safe concurrent access
- Validates URLs and aliases

## Building

```bash
go build -o shortener .
```

## Running

```bash
ADMIN_TOKEN=secret ./shortener -addr :8080 -data data.json
```

Command line flags:
- `-addr`: Address to listen on (default: `:8080`)
- `-data`: Path to JSON data file (default: `data.json`)

Environment variable:
- `ADMIN_TOKEN`: Required admin token for protected endpoints

## API Endpoints

### Create a link
```
POST /api/links
Content-Type: application/json

{
  "url": "https://example.com",
  "alias": "optional-alias"
}
```

Response (201 Created):
```json
{
  "code": "abc1234",
  "url": "https://example.com",
  "created_at": "2024-01-01T00:00:00Z",
  "visits": 0
}
```

### Follow a link
```
GET /{code}
```
Redirects (302 Found) to the original URL and increments visit count.

### List all links (admin)
```
GET /api/links
Authorization: Bearer <ADMIN_TOKEN>
```
Returns array of all links.

### Delete a link (admin)
```
DELETE /api/links/{code}
Authorization: Bearer <ADMIN_TOKEN>
```
Returns 204 No Content on success.

## Validation Rules

### URL:
- Must be absolute HTTP or HTTPS URL
- Must have a host
- Maximum 2048 characters

### Alias (optional):
- Must match `^[A-Za-z0-9_-]{3,32}$`
- Cannot be "api"
- Must be unique

## Storage

- Links are stored in JSON format in the specified data file
- Changes are saved atomically using temporary files
- Supports crash recovery
- Thread-safe for concurrent access

## Testing

Run all tests:
```bash
go test ./...
```

Run with verbose output:
```bash
go test -v ./...
```