output "base_url" {
  value = "http://${aws_lb.main.dns_name}"
}
output "table_name" {
  value = aws_dynamodb_table.carts.name
}
output "cluster_name" {
  value = aws_ecs_cluster.main.name
}
output "service_name" {
  value = aws_ecs_service.cart.name
}
output "log_group" {
  value = aws_cloudwatch_log_group.cart.name
}
output "alb_arn_suffix" {
  value = aws_lb.main.arn_suffix
}
output "tg_arn_suffix" {
  value = aws_lb_target_group.cart.arn_suffix
}
