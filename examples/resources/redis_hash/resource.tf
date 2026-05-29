# Minimal — uses all provider-level connection settings.
resource "redis_hash" "example" {
  key = "user:42"
  fields = {
    name  = "Ozzy"
    email = "ozzy@example.com"
    role  = "admin"
  }
}

# With a per-resource connection override.
resource "redis_hash" "other_instance" {
  key = "user:42"
  fields = {
    name = "Ozzy"
  }

  redis_connection {
    addr = "other-redis:6380"
    tls  = true
  }
}
