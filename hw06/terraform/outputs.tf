output "ecs_cluster_name" {
  description = "Name of the created ECS cluster"
  value       = module.ecs.cluster_name
}

output "ecs_service_name" {
  description = "Name of the running ECS service"
  value       = module.ecs.service_name
}

output "alb_url" {
  description = "Load balancer URL to use as the Locust host (Part III)"
  value       = var.horizontal_scaling ? "http://${module.alb[0].dns_name}" : null
}

output "cloudwatch_dashboard_url" {
  description = "CloudWatch dashboard with CPU/memory, task count and ALB metrics"
  value       = "https://${var.aws_region}.console.aws.amazon.com/cloudwatch/home?region=${var.aws_region}#dashboards/dashboard/${module.dashboard.dashboard_name}"
}

output "target_group_arn" {
  description = "Target group ARN (used by scripts/scale-watch.sh and scripts/break-health.sh)"
  value       = var.horizontal_scaling ? module.alb[0].target_group_arn : null
}
