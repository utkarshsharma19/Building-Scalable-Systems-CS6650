#!/usr/bin/env bash
# Functional checks for every endpoint and error path. Usage: scripts/smoke.sh http://host[:port]
set -uo pipefail
B=${1:?base url}
code() { curl -s -o /dev/null -w '%{http_code}' "$@"; }
ok=0; bad=0
check() { if [ "$2" == "$3" ]; then echo "PASS  $1 ($2)"; ok=$((ok+1)); else echo "FAIL  $1 (got $2, want $3)"; bad=$((bad+1)); fi; }

C=$(curl -s -XPOST "$B/shopping-carts" -H 'Content-Type: application/json' -d '{"customer_id":42}')
ID=$(echo "$C" | python3 -c 'import sys,json;print(json.load(sys.stdin)["shopping_cart_id"])')
echo "created cart $ID -> $C"
check "add new product"           "$(code -XPOST "$B/shopping-carts/$ID/items" -d '{"product_id":7,"quantity":2}')" 204
check "add same product again"    "$(code -XPOST "$B/shopping-carts/$ID/items" -d '{"product_id":7,"quantity":3}')" 204
check "add second product"        "$(code -XPOST "$B/shopping-carts/$ID/items" -d '{"product_id":9,"quantity":1}')" 204
G=$(curl -s "$B/shopping-carts/$ID?consistent=true"); echo "get -> $G"
check "quantities summed (7x5)"   "$(echo "$G" | python3 -c 'import sys,json;d=json.load(sys.stdin);print([i["quantity"] for i in d["items"] if i["product_id"]==7][0])')" 5
check "get missing cart"          "$(code "$B/shopping-carts/5")" 404
check "add to missing cart"       "$(code -XPOST "$B/shopping-carts/5/items" -d '{"product_id":1,"quantity":1}')" 404
check "bad customer_id"           "$(code -XPOST "$B/shopping-carts" -d '{"customer_id":-1}')" 400
check "missing customer_id"       "$(code -XPOST "$B/shopping-carts" -d '{}')" 400
check "non-numeric cart id"       "$(code "$B/shopping-carts/abc")" 400
check "injection-style input"     "$(code -XPOST "$B/shopping-carts" -d '{"customer_id":"1 OR 1=1"}')" 400
check "quantity 0"                "$(code -XPOST "$B/shopping-carts/$ID/items" -d '{"product_id":1,"quantity":0}')" 400
check "malformed JSON"            "$(code -XPOST "$B/shopping-carts" -d '{customer_id')" 400
for p in $(seq 100 147); do curl -s -o /dev/null -XPOST "$B/shopping-carts/$ID/items" -d "{\"product_id\":$p,\"quantity\":1}"; done
check "51st distinct product"     "$(code -XPOST "$B/shopping-carts/$ID/items" -d '{"product_id":999,"quantity":1}')" 409
check "existing product when full" "$(code -XPOST "$B/shopping-carts/$ID/items" -d '{"product_id":7,"quantity":1}')" 204
check "50-item cart item_count"   "$(curl -s "$B/shopping-carts/$ID?consistent=true" | python3 -c 'import sys,json;print(json.load(sys.stdin)["item_count"])')" 50
check "customer history"          "$(code "$B/customers/42/shopping-carts")" 200
echo "---- $ok passed, $bad failed"
[ $bad -eq 0 ]
