output "alb_url" {
  value = "http://${aws_lb.main.dns_name}"
}

output "queue_url" {
  value = aws_sqs_queue.orders.url
}

output "topic_arn" {
  value = aws_sns_topic.orders.arn
}

output "cluster_name" {
  value = aws_ecs_cluster.main.name
}

output "processor_service" {
  value = aws_ecs_service.processor.name
}

output "lambda_log_group" {
  value = var.deploy_lambda ? aws_cloudwatch_log_group.lambda[0].name : null
}

output "sqs_metrics_console" {
  value = "https://${var.aws_region}.console.aws.amazon.com/cloudwatch/home?region=${var.aws_region}#metricsV2?graph=~(metrics~(~(~'AWS*2fSQS~'ApproximateNumberOfMessagesVisible~'QueueName~'order-processing-queue))~period~60~stat~'Maximum)"
}
