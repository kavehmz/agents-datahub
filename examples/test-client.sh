#!/bin/bash

# Test client for DataHub exposer API
# This script demonstrates how to make API calls to the exposer

EXPOSER_URL="http://localhost:3000"
TOKEN="${TOKEN_CLIENT_TEST:-testtoken456}"

echo "=== DataHub API Test Client ==="
echo ""

# Test 1: Get user by ID
echo "Test 1: Get user by ID"
echo "Request: POST /data/user-data/getUserById"
curl -X POST "$EXPOSER_URL/data/user-data/getUserById" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "userId": "user-001",
    "includeMetadata": false
  }' | jq .
echo ""
echo ""

# Test 2: Get users by filter
echo "Test 2: Get users by filter (active users)"
echo "Request: POST /data/user-data/getUsersByFilter"
curl -X POST "$EXPOSER_URL/data/user-data/getUsersByFilter" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "filter": {
      "status": "active",
      "limit": 10,
      "offset": 0
    },
    "sort": {
      "field": "createdAt",
      "order": "desc"
    }
  }' | jq .
echo ""
echo ""

# Test 3: Test with invalid token (should fail)
echo "Test 3: Test with invalid token (should fail)"
echo "Request: POST /data/user-data/getUserById"
curl -X POST "$EXPOSER_URL/data/user-data/getUserById" \
  -H "Authorization: Bearer invalid-token" \
  -H "Content-Type: application/json" \
  -d '{
    "userId": "user-001"
  }' | jq .
echo ""
echo ""

# Test 4: Check health
echo "Test 4: Check exposer health"
echo "Request: GET /health"
curl -X GET "$EXPOSER_URL/health" | jq .
echo ""
echo ""

# Test 5: Check metrics
echo "Test 5: Check exposer metrics"
echo "Request: GET /metrics"
curl -X GET "$EXPOSER_URL/metrics"
echo ""

echo "=== Tests Complete ==="