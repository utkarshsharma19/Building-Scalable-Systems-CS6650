# Two ECR repos and images, one cluster, two Fargate services (256 CPU / 512 MB).
locals {
  src_hash = sha1(join("", [
    for f in sort(fileset("${path.module}/../src", "**")) :
    filesha1("${path.module}/../src/${f}") if !startswith(f, "cmd/lambda")
  ]))
  apps = ["receiver", "processor"]
}

# One image holds both binaries (/receiver, /processor).
resource "aws_ecr_repository" "app" {
  name         = "${var.name}-orders"
  force_delete = true
}

resource "docker_image" "app" {
  name = "${aws_ecr_repository.app.repository_url}:latest"
  build {
    context = "${path.module}/../src"
  }
  triggers = { src_hash = local.src_hash }
}

resource "docker_registry_image" "app" {
  name     = docker_image.app.name
  triggers = { image_id = docker_image.app.latest } # re-push whenever the image is rebuilt
}

resource "aws_cloudwatch_log_group" "app" {
  for_each          = toset(local.apps)
  name              = "/ecs/${var.name}-${each.key}"
  retention_in_days = var.log_retention_days
}

resource "aws_ecs_cluster" "main" {
  name = "${var.name}-cluster"
  setting {
    name  = "containerInsights"
    value = "enabled" # per-task CPU/memory for the "resource utilization" numbers
  }
}

locals {
  env = {
    receiver = {
      SNS_TOPIC_ARN    = aws_sns_topic.orders.arn
      PAYMENT_CAPACITY = tostring(var.payment_capacity)
      PAYMENT_DELAY    = var.payment_delay
    }
    processor = {
      SQS_QUEUE_URL = aws_sqs_queue.orders.url
      WORKERS       = tostring(var.processor_workers)
      PAYMENT_DELAY = var.payment_delay
    }
  }
}

resource "aws_ecs_task_definition" "app" {
  for_each                 = toset(local.apps)
  family                   = "${var.name}-${each.key}"
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = "256"
  memory                   = "512"
  execution_role_arn       = data.aws_iam_role.lab_role.arn
  task_role_arn            = data.aws_iam_role.lab_role.arn

  container_definitions = jsonencode([{
    name         = each.key
    image        = "${aws_ecr_repository.app.repository_url}@${docker_registry_image.app.sha256_digest}"
    entryPoint   = ["/${each.key}"]
    essential    = true
    portMappings = [{ containerPort = 8080 }]
    environment  = [for k, v in local.env[each.key] : { name = k, value = v }]
    healthCheck = {
      command     = ["CMD", "/${each.key}", "-healthcheck"] # distroless has no curl
      interval    = 10
      timeout     = 5
      retries     = 3
      startPeriod = 10
    }
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = aws_cloudwatch_log_group.app[each.key].name
        "awslogs-region"        = var.aws_region
        "awslogs-stream-prefix" = "ecs"
      }
    }
  }])
}

resource "aws_ecs_service" "receiver" {
  name            = "${var.name}-receiver"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.app["receiver"].arn
  desired_count   = 1
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = aws_subnet.private[*].id
    security_groups  = [aws_security_group.tasks.id]
    assign_public_ip = false
  }
  load_balancer {
    target_group_arn = aws_lb_target_group.receiver.arn
    container_name   = "receiver"
    container_port   = 8080
  }
  health_check_grace_period_seconds = 30
  depends_on                        = [aws_lb_listener.http, aws_nat_gateway.main]
}

resource "aws_ecs_service" "processor" {
  name            = "${var.name}-processor"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.app["processor"].arn
  desired_count   = var.processor_count
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = aws_subnet.private[*].id
    security_groups  = [aws_security_group.tasks.id]
    assign_public_ip = false
  }
  depends_on = [aws_nat_gateway.main, aws_route_table_association.private]
}
