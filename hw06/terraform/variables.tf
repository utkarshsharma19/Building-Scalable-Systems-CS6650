variable "aws_region" {
  type    = string
  default = "us-west-2"
}

variable "ecr_repository_name" {
  type    = string
  default = "product-search"
}

variable "service_name" {
  type    = string
  default = "product-search"
}

variable "container_port" {
  type    = number
  default = 8080
}

variable "ecs_count" {
  type        = number
  default     = 1
  description = "Task count when horizontal_scaling = false (Part II)"
}

variable "task_cpu" {
  type        = string
  default     = "256"
  description = "Task CPU units (256 = 0.25 vCPU). Try 512 for the vertical-scaling comparison."
}

variable "task_memory" {
  type        = string
  default     = "512"
  description = "Task memory (MiB)"
}

variable "log_retention_days" {
  type    = number
  default = 7
}

variable "horizontal_scaling" {
  type        = bool
  default     = false
  description = "false = Part II (1 task, public IP). true = Part III (ALB + target group + autoscaling)."
}

variable "min_capacity" {
  type    = number
  default = 2
}

variable "max_capacity" {
  type    = number
  default = 4
}

variable "target_cpu" {
  type        = number
  default     = 70
  description = "Average CPU % the target-tracking policy aims for"
}

variable "scale_out_cooldown" {
  type    = number
  default = 300
}

variable "scale_in_cooldown" {
  type    = number
  default = 300
}

variable "deregistration_delay" {
  type        = number
  default     = 30
  description = "Seconds a stopped/scaled-in task drains in the target group"
}
