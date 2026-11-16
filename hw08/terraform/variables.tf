variable "aws_region" {
  type    = string
  default = "us-west-2"
}

variable "aws_profile" {
  type    = string
  default = "hw8"
}

variable "name" {
  type        = string
  default     = "hw8"
  description = "Prefix for every resource name"
}

variable "task_count" {
  type        = number
  default     = 2
  description = "Same as teammates: 2 tasks behind the ALB"
}

variable "max_idle_conns_per_host" {
  type        = number
  default     = 200
  description = "SDK HTTP keep-alive pool to DynamoDB (SDK default is 10)"
}

variable "log_retention_days" {
  type    = number
  default = 3
}

variable "database" {
  type        = string
  default     = "dynamodb"
  description = "Which store the cart service uses: dynamodb (STEP II) or mysql (STEP I)"
}

variable "mysql_pool_size" {
  type        = number
  default     = 10
  description = "database/sql MaxOpenConns per task; tasks x pool must stay under RDS max_connections"
}
