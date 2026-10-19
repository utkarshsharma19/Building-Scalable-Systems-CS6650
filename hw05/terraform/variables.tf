variable "aws_region" {
  type    = string
  default = "us-west-2"
}

variable "ecr_repository_name" {
  type    = string
  default = "product-service"
}

variable "service_name" {
  type    = string
  default = "product-service"
}

variable "container_port" {
  type    = number
  default = 8080
}

variable "ecs_count" {
  type    = number
  default = 1
}

variable "log_retention_days" {
  type    = number
  default = 7
}
