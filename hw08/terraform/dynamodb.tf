# One table, partition key = cart_id (random 31-bit number -> uniform hash distribution).
# No sort key: every API call addresses exactly one cart. Items are embedded as a map
# (product_id -> quantity); 50 items x ~20 bytes is far below the 400 KB item limit.
resource "aws_dynamodb_table" "carts" {
  name         = "${var.name}-shopping-carts"
  billing_mode = "PAY_PER_REQUEST" # on-demand: no capacity planning, pay per request, nothing idle
  hash_key     = "cart_id"

  attribute {
    name = "cart_id"
    type = "N"
  }
  attribute {
    name = "customer_id"
    type = "N"
  }
  attribute {
    name = "created_at"
    type = "S"
  }

  # Customer purchase history: all carts for a customer, newest first.
  global_secondary_index {
    name            = "customer_id-index"
    hash_key        = "customer_id"
    range_key       = "created_at"
    projection_type = "ALL"
  }

  # Carts are session data: abandoned carts expire automatically (TTL deletes are free).
  ttl {
    attribute_name = "expires_at"
    enabled        = true
  }

  deletion_protection_enabled = false
}

# Shows the most-accessed partition keys -> evidence that load spreads evenly (no hot key).
resource "aws_dynamodb_contributor_insights" "carts" {
  table_name = aws_dynamodb_table.carts.name
}
