variable "aws_region" {
  type    = string
  default = "us-west-2"
}

variable "name" {
  type        = string
  default     = "hw7"
  description = "Prefix for every resource name"
}

variable "processor_workers" {
  type        = number
  default     = 1
  description = "Concurrent payment goroutines in the Order Processor (Phase 5: 1, 5, 20, 100)"
}

variable "processor_count" {
  type        = number
  default     = 1
  description = "Order Processor tasks. Set 0 to pause processing (e.g. to fill the queue)."
}

variable "payment_capacity" {
  type        = number
  default     = 1
  description = "Orders the simulated payment processor verifies at once on /orders/sync"
}

variable "payment_delay" {
  type    = string
  default = "3s"
}

variable "deploy_lambda" {
  type        = bool
  default     = false
  description = "Part III: create the Lambda function. Run scripts/build-lambda.sh first."
}

variable "lambda_subscribed" {
  type        = bool
  default     = false
  description = "Part III: subscribe the Lambda to SNS. Keep FALSE for every Locust test."
}

variable "log_retention_days" {
  type    = number
  default = 7
}
