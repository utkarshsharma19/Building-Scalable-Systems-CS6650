variable "cluster_name" {
  type        = string
  description = "ECS cluster name"
}

variable "service_name" {
  type        = string
  description = "ECS service name"
}

variable "min_capacity" {
  type        = number
  description = "Minimum task count"
}

variable "max_capacity" {
  type        = number
  description = "Maximum task count"
}

variable "target_cpu" {
  type        = number
  description = "Average CPU utilization (%) to track"
}

variable "scale_out_cooldown" {
  type        = number
  description = "Seconds after a scale-out before another scale-out"
}

variable "scale_in_cooldown" {
  type        = number
  description = "Seconds after a scale-in before another scale-in"
}
