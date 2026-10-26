output "dns_name" {
  description = "Public DNS name of the ALB"
  value       = aws_lb.this.dns_name
}

output "security_group_id" {
  description = "ALB security group (allowed into the ECS tasks)"
  value       = aws_security_group.alb.id
}

output "target_group_arn" {
  description = "Target group the ECS service registers tasks into"
  value       = aws_lb_target_group.this.arn
}

output "arn_suffix" {
  description = "ALB ARN suffix, the LoadBalancer dimension in CloudWatch"
  value       = aws_lb.this.arn_suffix
}

output "target_group_arn_suffix" {
  description = "Target group ARN suffix, the TargetGroup dimension in CloudWatch"
  value       = aws_lb_target_group.this.arn_suffix
}
