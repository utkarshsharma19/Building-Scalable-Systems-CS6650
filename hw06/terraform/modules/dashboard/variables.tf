variable "region" {
  type        = string
  description = "AWS region the metrics live in"
}

variable "cluster_name" {
  type        = string
  description = "ECS cluster name"
}

variable "service_name" {
  type        = string
  description = "ECS service name (also the dashboard name)"
}

variable "task_cpu" {
  type        = number
  description = "Task CPU units, to turn CpuUtilized into a percentage"
}

variable "target_cpu" {
  type        = number
  description = "Autoscaling CPU target, drawn as a line on the CPU widget"
}

variable "alb_arn_suffix" {
  type        = string
  default     = null
  description = "ALB ARN suffix (null = no ALB widgets)"
}

variable "target_group_arn_suffix" {
  type        = string
  default     = null
  description = "Target group ARN suffix"
}
