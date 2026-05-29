# Minimal — uses all provider-level connection settings.
resource "redis_string" "example" {
  key   = "app:version"
  value = "1.2.3"
}

# With a per-resource connection override.
resource "redis_string" "other_instance" {
  key   = "app:version"
  value = "1.2.3"

  redis_connection {
    addr = "other-redis:6379"
    db   = 1
  }
}
