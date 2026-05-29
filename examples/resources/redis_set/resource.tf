# Minimal — uses all provider-level connection settings.
resource "redis_set" "example" {
  key     = "article:1:tags"
  members = ["terraform", "redis", "infrastructure"]
}

# With a per-resource connection override.
resource "redis_set" "other_instance" {
  key     = "article:1:tags"
  members = ["terraform"]

  redis_connection {
    addr     = "other-redis:6379"
    password = "s3cr3t"
  }
}
