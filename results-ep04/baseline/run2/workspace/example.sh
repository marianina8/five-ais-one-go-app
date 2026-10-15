#!/bin/bash

# Example script demonstrating the URL shortener service

echo "Building the URL shortener..."
go build -o shortener .

echo -e "\nStarting the server in background..."
ADMIN_TOKEN=secret123 ./shortener -addr :18080 -data example-data.json &
SERVER_PID=$!

# Wait for server to start
sleep 2

echo -e "\n=== Testing API ===\n"

echo "1. Creating a link without alias:"
curl -X POST http://localhost:18080/api/links \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com"}'

echo -e "\n\n2. Creating a link with alias 'my-page':"
curl -X POST http://localhost:18080/api/links \
  -H "Content-Type: application/json" \
  -d '{"url": "https://google.com", "alias": "my-page"}'

echo -e "\n\n3. Trying to create duplicate alias (should fail):"
curl -X POST http://localhost:18080/api/links \
  -H "Content-Type: application/json" \
  -d '{"url": "https://duplicate.com", "alias": "my-page"}'

echo -e "\n\n4. Creating another link with alias 'test-123':"
curl -X POST http://localhost:18080/api/links \
  -H "Content-Type: application/json" \
  -d '{"url": "https://github.com", "alias": "test-123"}'

echo -e "\n\n5. Listing links without auth (should fail):"
curl http://localhost:18080/api/links

echo -e "\n\n6. Listing links with auth:"
curl -H "Authorization: Bearer secret123" http://localhost:18080/api/links

echo -e "\n\n7. Following a link (will show redirect headers):"
curl -v http://localhost:18080/my-page 2>&1 | grep -E "(GET|Location|HTTP)"

echo -e "\n\n8. Following another link:"
curl -v http://localhost:18080/test-123 2>&1 | grep -E "(GET|Location|HTTP)"

echo -e "\n\n9. Listing links again to see visit counts:"
curl -H "Authorization: Bearer secret123" http://localhost:18080/api/links

echo -e "\n\n10. Deleting a link without auth (should fail):"
curl -X DELETE http://localhost:18080/api/links/test-123

echo -e "\n\n11. Deleting a link with auth:"
curl -X DELETE -H "Authorization: Bearer secret123" http://localhost:18080/api/links/test-123

echo -e "\n\n12. Verifying link was deleted (should fail):"
curl -v http://localhost:18080/test-123 2>&1 | grep -E "(GET|HTTP/404)"

echo -e "\n\n13. Final list of links:"
curl -H "Authorization: Bearer secret123" http://localhost:18080/api/links

echo -e "\n\nCleaning up..."
kill $SERVER_PID
rm -f example-data.json

echo -e "\nDone!"