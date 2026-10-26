variable "service_name" {
  type        = string
  description = "Base name for ALB resources"
}

variable "vpc_id" {
  type        = string
  description = "VPC the ALB and target group live in"
}

variable "subnet_ids" {
  type        = list(string)
  description = "Public subnets for the ALB (at least 2 AZs)"
}

variable "container_port" {
  type        = number
  description = "Port the targets (ECS tasks) listen on"
}

variable "deregistration_delay" {
  type        = number
  default     = 30
  description = "Seconds a deregistering target keeps draining in-flight requests"
}
