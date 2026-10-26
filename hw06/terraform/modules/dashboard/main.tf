locals {
  ecs_dims = ["ClusterName", var.cluster_name, "ServiceName", var.service_name]
  has_alb  = var.alb_arn_suffix != null

  ecs_widgets = [
    {
      type = "metric", x = 0, y = 0, width = 12, height = 6
      properties = {
        title  = "ECS service CPU utilization (%)"
        region = var.region, view = "timeSeries", stat = "Average", period = 60
        yAxis  = { left = { min = 0, max = 100 } }
        metrics = [
          concat(["AWS/ECS", "CPUUtilization"], local.ecs_dims, [{ label = "average over tasks" }]),
          concat(["AWS/ECS", "CPUUtilization"], local.ecs_dims, [{ stat = "Maximum", label = "busiest task" }]),
        ]
        annotations = jsondecode(local.has_alb ? jsonencode({ horizontal = [{ value = var.target_cpu, label = "scaling target" }] }) : "{}")
      }
    },
    {
      type = "metric", x = 12, y = 0, width = 12, height = 6
      properties = {
        title  = "ECS service memory utilization (%)"
        region = var.region, view = "timeSeries", stat = "Average", period = 60
        yAxis  = { left = { min = 0, max = 100 } }
        metrics = [
          concat(["AWS/ECS", "MemoryUtilization"], local.ecs_dims, [{ label = "average over tasks" }]),
          concat(["AWS/ECS", "MemoryUtilization"], local.ecs_dims, [{ stat = "Maximum", label = "busiest task" }]),
        ]
      }
    },
    {
      type = "metric", x = 0, y = 6, width = 12, height = 6
      properties = {
        title  = "CPU per task (% of ${var.task_cpu} units, Container Insights)"
        region = var.region, view = "timeSeries", period = 60
        yAxis  = { left = { min = 0, max = 100 } }
        metrics = [
          [{ id = "raw", visible = false, expression = "SEARCH('{ECS/ContainerInsights,ClusterName,TaskDefinitionFamily,TaskId} MetricName=\"CpuUtilized\" ClusterName=\"${var.cluster_name}\"', 'Average', 60)" }],
          [{ id = "pct", expression = "raw * 100 / ${var.task_cpu}", label = "" }],
        ]
      }
    },
    {
      type = "metric", x = 12, y = 6, width = 12, height = 6
      properties = {
        title  = "Task count: desired vs running"
        region = var.region, view = "timeSeries", stat = "Average", period = 60
        yAxis  = { left = { min = 0 } }
        metrics = [
          concat(["ECS/ContainerInsights", "DesiredTaskCount"], local.ecs_dims, [{ label = "desired" }]),
          concat(["ECS/ContainerInsights", "RunningTaskCount"], local.ecs_dims, [{ label = "running" }]),
          concat(["ECS/ContainerInsights", "PendingTaskCount"], local.ecs_dims, [{ label = "pending" }]),
        ]
      }
    },
  ]

  alb_widgets = [
    {
      type = "metric", x = 0, y = 12, width = 12, height = 6
      properties = {
        title  = "ALB requests (per minute)"
        region = var.region, view = "timeSeries", stat = "Sum", period = 60
        metrics = [
          ["AWS/ApplicationELB", "RequestCount", "LoadBalancer", var.alb_arn_suffix, { label = "total" }],
          ["AWS/ApplicationELB", "RequestCountPerTarget", "TargetGroup", var.target_group_arn_suffix, "LoadBalancer", var.alb_arn_suffix, { label = "per target", yAxis = "right" }],
        ]
      }
    },
    {
      type = "metric", x = 12, y = 12, width = 12, height = 6
      properties = {
        title  = "ALB target response time (s)"
        region = var.region, view = "timeSeries", period = 60
        metrics = [
          ["AWS/ApplicationELB", "TargetResponseTime", "LoadBalancer", var.alb_arn_suffix, { stat = "Average", label = "avg" }],
          ["AWS/ApplicationELB", "TargetResponseTime", "LoadBalancer", var.alb_arn_suffix, { stat = "p95", label = "p95" }],
          ["AWS/ApplicationELB", "TargetResponseTime", "LoadBalancer", var.alb_arn_suffix, { stat = "p99", label = "p99" }],
        ]
      }
    },
    {
      type = "metric", x = 0, y = 18, width = 12, height = 6
      properties = {
        title  = "Target group: healthy vs unhealthy targets"
        region = var.region, view = "timeSeries", stat = "Minimum", period = 60
        yAxis  = { left = { min = 0 } }
        metrics = [
          ["AWS/ApplicationELB", "HealthyHostCount", "TargetGroup", var.target_group_arn_suffix, "LoadBalancer", var.alb_arn_suffix, { label = "healthy" }],
          ["AWS/ApplicationELB", "UnHealthyHostCount", "TargetGroup", var.target_group_arn_suffix, "LoadBalancer", var.alb_arn_suffix, { stat = "Maximum", label = "unhealthy" }],
        ]
      }
    },
    {
      type = "metric", x = 12, y = 18, width = 12, height = 6
      properties = {
        title  = "Errors seen by the ALB (per minute)"
        region = var.region, view = "timeSeries", stat = "Sum", period = 60
        metrics = [
          ["AWS/ApplicationELB", "HTTPCode_ELB_5XX_Count", "LoadBalancer", var.alb_arn_suffix, { label = "ELB 5xx (no healthy target, etc.)" }],
          ["AWS/ApplicationELB", "HTTPCode_Target_5XX_Count", "LoadBalancer", var.alb_arn_suffix, { label = "target 5xx" }],
          ["AWS/ApplicationELB", "HTTPCode_Target_4XX_Count", "LoadBalancer", var.alb_arn_suffix, { label = "target 4xx" }],
        ]
      }
    },
  ]
}

resource "aws_cloudwatch_dashboard" "this" {
  dashboard_name = var.service_name
  dashboard_body = jsonencode({ widgets = concat(local.ecs_widgets, slice(local.alb_widgets, 0, local.has_alb ? length(local.alb_widgets) : 0)) })
}
