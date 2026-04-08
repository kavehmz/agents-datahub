#!/bin/bash

# Test REST API source integration
# This demonstrates calling a REST API backend through DataHub

EXPOSER_URL="http://localhost:3000"
TOKEN="${TOKEN_CLIENT_TEST:-testtoken456}"

echo "=== DataHub REST API Source Test ==="
echo ""

echo "Test 1: Get User from PostgreSQL (existing)"
echo "Request: POST /data/user-data/getUserById"
curl -X POST "$EXPOSER_URL/data/user-data/getUserById" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "userId": "user-001"
  }' | jq .
echo ""
echo ""

echo "Test 2: Get User Payments from REST API (NEW!)"
echo "Request: POST /data/payment-data/getUserPayments"
curl -X POST "$EXPOSER_URL/data/payment-data/getUserPayments" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "userId": "user-001"
  }' | jq .
echo ""
echo ""

echo "=== Hybrid Source Demonstration Complete ==="
echo ""
echo "Summary:"
echo "  - getUserById       → PostgreSQL"
echo "  - getUsersByFilter  → PostgreSQL"
echo "  - getUserPayments   → External REST API"
echo ""
echo "One source service handling multiple backend types!"