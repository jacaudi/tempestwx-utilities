target "docker-metadata-action" {
  labels = {}
}

variable "BUILD_DATE" {
  default = ""
}

variable "REGISTRY_IMAGE" {
  default = ""
}

variable "SOURCE_COMMIT" {
  default = ""
}

variable "APP" {
  default = "tempestwx-utilities"
}

variable "VERSION" {
  default = "latest"
}

variable "SOURCE" {
  default = "https://github.com/jacaudi/tempestwx-utilities"
}

group "default" {
  targets = ["image-local"]
}

target "image" {
  inherits = ["docker-metadata-action"]
  args = {}
  labels = {
    "org.opencontainers.image.source" = "${SOURCE}"
    "org.opencontainers.image.created" = "${BUILD_DATE}"
    "org.opencontainers.image.version" = "${VERSION}"
    "org.opencontainers.image.title" = "${APP}" 
    "org.opencontainers.image.description" = "Multi-backend utilities for Tempest weather station data (Prometheus, PostgreSQL)"
    "org.opencontainers.image.licenses" = "MIT"
  }
}

target "image-local" {
  inherits = ["image"]
  output = ["type=docker"]
  tags = ["${APP}:${VERSION}"]
}

target "image-automated" {
  inherits = ["image"]
  platforms = [
    "linux/amd64",
    "linux/arm64"
  ]
  output = ["type=registry"]
  tags = ["${REGISTRY_IMAGE}:${VERSION}"]
  annotations = [
    "index:org.opencontainers.image.source=${SOURCE}",
    "index:org.opencontainers.image.created=${BUILD_DATE}",
    "index:org.opencontainers.image.version=${VERSION}",
    "index:org.opencontainers.image.title=${APP}",
    "index:org.opencontainers.image.description=Multi-backend utilities for Tempest weather station data (Prometheus, PostgreSQL)",
    "index:org.opencontainers.image.licenses=MIT"
  ]
}
